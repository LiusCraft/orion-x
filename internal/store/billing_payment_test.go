package store

import (
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newDryRunPaymentStore 造一个只生成 SQL、不连库的 PaymentStore（复用 billing_test.go
// 里的 sqlRecorder 与同样的 DryRun 姿势）。
func newDryRunPaymentStore(t *testing.T) (*PaymentStore, *sqlRecorder) {
	t.Helper()

	rec := &sqlRecorder{Interface: logger.Discard}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DriverName: "pgx",
		DSN:        "postgres://billing:billing@127.0.0.1:5432/billing_test?sslmode=disable",
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 rec,
	})
	if err != nil {
		t.Fatalf("open dry-run db: %v", err)
	}
	return NewPaymentStore(db), rec
}

// TestPaymentStoreSQL 盯住状态机的 SQL 形状。
//
// 这里的每一条 UPDATE 都必须是**有条件**的：重复的回调、乱序的通知、并发的对账
// 都会重复执行到同一行上，条件就是那次更新该不该发生。少一个条件不会报错，只会
// 悄悄把已经入过账的订单改回去——所以用 SQL 断言把它们钉住。
func TestPaymentStoreSQL(t *testing.T) {
	at := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		run        func(s *PaymentStore) error
		wantSQL    []string
		notWantSQL []string
		wantErr    error // DryRun 下写操作的 RowsAffected 恒为 0
	}{
		{
			name: "mark paid: only from pending or closed",
			run: func(s *PaymentStore) error {
				return s.MarkOrderPaid(nil, "p1", "t1", at)
			},
			wantSQL: []string{
				`UPDATE "billing_payment_orders"`,
				`out_trade_no = 'p1' AND status IN ('order:pending','order:closed')`,
				`"status"='order:paid'`,
				`"gateway_trade_no"='t1'`,
				`"paid_at"='2026-09-20 10:00:00'`,
			},
			// 已经 paid / credited / refunded 的行不许被覆盖。
			notWantSQL: []string{"'order:credited'", "'order:refunded'"},
		},
		{
			name: "mark credited: only from paid",
			run: func(s *PaymentStore) error {
				return s.MarkOrderCredited("p1", at)
			},
			wantSQL: []string{
				`out_trade_no = 'p1' AND status = 'order:paid'`,
				`"status"='order:credited'`,
				`"credited_at"='2026-09-20 10:00:00'`,
			},
			wantErr: ErrNotFound,
		},
		{
			name: "close: only from pending",
			run: func(s *PaymentStore) error {
				return s.CloseOrder("p1")
			},
			wantSQL: []string{
				`out_trade_no = 'p1' AND status = 'order:pending'`,
				`"status"='order:closed'`,
			},
		},
		{
			name: "lock order takes a row lock",
			run: func(s *PaymentStore) error {
				_, err := s.LockOrder(nil, "p1")
				return err
			},
			// 这条断言的是 SELECT ... FOR UPDATE 这个形状（回调并发全靠它串行化）；
			// DryRun 不会真的查库，所以拿不到 ErrRecordNotFound，err 是 nil。
			wantSQL: []string{`FROM "billing_payment_orders"`, `out_trade_no = 'p1'`, `FOR UPDATE`},
		},
		{
			name: "paid but uncredited is the sweep's source of truth",
			run: func(s *PaymentStore) error {
				_, err := s.ListPaidUncreditedOrders(100)
				return err
			},
			wantSQL: []string{
				`FROM "billing_payment_orders"`,
				`status = 'order:paid'`,
				`ORDER BY paid_at ASC`,
			},
		},
		{
			name: "expired orders are pending and past their deadline",
			run: func(s *PaymentStore) error {
				_, err := s.ListExpiredOrders(at, 100)
				return err
			},
			wantSQL: []string{
				`status = 'order:pending' AND expires_at < '2026-09-20 10:00:00'`,
			},
		},
		{
			name: "mark refunded: only from credited",
			run: func(s *PaymentStore) error {
				return s.MarkOrderRefunded("p1", at)
			},
			wantSQL: []string{
				`UPDATE "billing_payment_orders"`,
				`out_trade_no = 'p1' AND status = 'order:credited'`,
				`"status"='order:refunded'`,
				`"refunded_at"='2026-09-20 10:00:00'`,
			},
			// 没入过账的单子没有可退的钱：pending / paid / closed 都不在条件里。
			notWantSQL: []string{"'order:pending'", "'order:paid'", "'order:closed'"},
			wantErr:    ErrNotFound,
		},
		{
			name: "order list filters by subject, status and channel",
			run: func(s *PaymentStore) error {
				_, err := s.ListOrders(PaymentOrderQuery{
					SubjectType: "user",
					SubjectID:   "user-1",
					Channel:     "epay:alipay",
					Status:      "order:credited",
					Keyword:     "p0123",
					Limit:       20,
				})
				return err
			},
			wantSQL: []string{
				`FROM "billing_payment_orders"`,
				`subject_type = 'user' AND subject_id = 'user-1'`,
				`channel = 'epay:alipay'`,
				`status = 'order:credited'`,
				`out_trade_no ILIKE '%p0123%' OR gateway_trade_no ILIKE '%p0123%'`,
				`ORDER BY created_at DESC,id DESC`,
			},
		},
		{
			name: "order list without filters is the admin view",
			run: func(s *PaymentStore) error {
				_, err := s.ListOrders(PaymentOrderQuery{Limit: 20})
				return err
			},
			wantSQL:    []string{`FROM "billing_payment_orders"`},
			notWantSQL: []string{"WHERE"},
		},
		{
			name: "notifications are append-only facts",
			run: func(s *PaymentStore) error {
				return s.InsertNotification(nil, &PaymentNotification{
					OutTradeNo:     "p1",
					GatewayTradeNo: "t1",
					AmountRaw:      "9.99",
					Status:         "TRADE_SUCCESS",
					Payload:        "money=9.99&out_trade_no=p1",
				})
			},
			wantSQL: []string{
				`INSERT INTO "billing_payment_notifications"`,
				`'p1'`,
				`'TRADE_SUCCESS'`,
			},
			// 通知表没有唯一键：网关重发是正常现象，每一次投递都是一条要留的事实。
			notWantSQL: []string{"ON CONFLICT"},
		},
		{
			name: "create order rejects a duplicate out_trade_no",
			run: func(s *PaymentStore) error {
				return s.CreateOrder(nil, &PaymentOrder{
					OutTradeNo:  "p1",
					SubjectType: "user",
					SubjectID:   "user-1",
					Channel:     "epay:alipay",
					AmountMicro: 9_990_000,
					Currency:    "CNY",
					Title:       "余额充值 9.99",
					Status:      "order:pending",
					ExpiresAt:   at,
				})
			},
			wantSQL: []string{`INSERT INTO "billing_payment_orders"`, `'p1'`, `9990000`},
			// 撞单号必须是错误，不能静默改成 upsert：那等于把别人的订单覆盖掉。
			notWantSQL: []string{"ON CONFLICT"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, rec := newDryRunPaymentStore(t)
			err := tc.run(s)

			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else if err != tc.wantErr {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}

			sql := rec.all()
			for _, want := range tc.wantSQL {
				if !strings.Contains(sql, want) {
					t.Errorf("SQL missing %q\n--- got ---\n%s", want, sql)
				}
			}
			for _, notWant := range tc.notWantSQL {
				if strings.Contains(sql, notWant) {
					t.Errorf("SQL should not contain %q\n--- got ---\n%s", notWant, sql)
				}
			}
		})
	}
}
