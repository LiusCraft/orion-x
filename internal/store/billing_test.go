package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestNormalizeLimit(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "zero uses default", in: 0, want: defaultBillingQueryLimit},
		{name: "negative uses default", in: -1, want: defaultBillingQueryLimit},
		{name: "explicit limit is kept", in: 25, want: 25},
		{name: "over max is clamped", in: maxBillingQueryLimit + 1, want: maxBillingQueryLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeLimit(tt.in); got != tt.want {
				t.Errorf("normalizeLimit(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

// sqlRecorder 把 GORM 编译出来的语句收下来（DryRun 模式，不连数据库）。
// 内联 mock，不引 mock 生成器。
type sqlRecorder struct {
	logger.Interface
	mu   sync.Mutex
	sqls []string
}

func (r *sqlRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sqls = append(r.sqls, sql)
}

// all 返回到目前为止捕获到的所有语句（含插值后的变量）。
func (r *sqlRecorder) all() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.sqls, "\n")
}

// newDryRunBillingStore 造一个只生成 SQL、不连库的 BillingStore。
// SkipDefaultTransaction 是必须的：GORM 在 DryRun 下仍然会为写操作 Begin()，那一步会真去连库。
func newDryRunBillingStore(t *testing.T) (*BillingStore, *sqlRecorder) {
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
	return NewBillingStore(db), rec
}

// TestBillingStoreSQL 盯住几条容易写错、又只能靠 SQL 才能验证的语句形状：
// 价格匹配的优先级 ORDER BY、资源维度的 IN 组合、SKIP LOCKED、ON CONFLICT 的目标列。
// DryRun 下写操作不会真的执行，RowsAffected 为 0，所以断言里要容忍 ErrNotFound。
func TestBillingStoreSQL(t *testing.T) {
	at := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		run        func(s *BillingStore) error
		wantSQL    []string
		notWantSQL []string
		wantErr    error // DryRun 下写操作的 RowsAffected 恒为 0，聚合查询也拿不到行
	}{
		{
			name: "price candidates: account agreement beats resource rank beats recency",
			run: func(s *BillingStore) error {
				_, err := s.FindPriceCandidates("llm:tokens:input", "acct-1",
					[][2]string{{"provider", "p1"}, {"model", "m1"}}, at, 10)
				return err
			},
			wantSQL: []string{
				`FROM "billing_prices"`,
				`item_code = 'llm:tokens:input'`,
				`account_id IN ('','acct-1')`,
				`(resource_type = 'item' OR (resource_type, resource_id) IN (('provider','p1'),('model','m1')))`,
				`effective_to IS NULL OR effective_to > '2026-09-20 10:00:00'`,
				`ORDER BY (account_id <> '') DESC,` +
					`CASE resource_type WHEN 'voice' THEN 3 WHEN 'model' THEN 2 WHEN 'provider' THEN 1 ELSE 0 END DESC,` +
					`effective_from DESC,id DESC`,
				`LIMIT 10`,
			},
			notWantSQL: []string{"FOR UPDATE"},
		},
		{
			name: "price candidates: no account and no refs only matches item level",
			run: func(s *BillingStore) error {
				_, err := s.FindPriceCandidates("tts:characters", "", nil, at, 0)
				return err
			},
			wantSQL: []string{
				`account_id = ''`,
				`resource_type = 'item'`,
				`LIMIT 100`, // limit <= 0 用默认值，别变成全表扫
			},
			notWantSQL: []string{"(resource_type, resource_id) IN"},
		},
		{
			name: "claim pending events locks with skip locked",
			run: func(s *BillingStore) error {
				_, err := s.ClaimPendingEvents(nil, 200)
				return err
			},
			wantSQL: []string{
				`FROM "billing_usage_events"`,
				`status = 'pending'`,
				`ORDER BY account_id ASC,received_at ASC,id ASC`,
				`LIMIT 200 FOR UPDATE SKIP LOCKED`,
			},
		},
		{
			name: "lock period state creates the row first, then locks it",
			run: func(s *BillingStore) error {
				_, err := s.LockPeriodState(nil, "acct-1", "llm:tokens:input", "price-1", at)
				return err
			},
			wantSQL: []string{
				`INSERT INTO "billing_period_states"`,
				`ON CONFLICT DO NOTHING`,
				`FROM "billing_period_states" WHERE account_id = 'acct-1' AND item_code = 'llm:tokens:input' ` +
					`AND price_id = 'price-1' AND period_start = '2026-09-20 10:00:00' LIMIT 1 FOR UPDATE`,
			},
		},
		{
			name: "lock account takes the row lock by id",
			run: func(s *BillingStore) error {
				_, err := s.LockAccount(nil, "acct-1")
				return err
			},
			wantSQL: []string{
				`SELECT * FROM "billing_accounts" WHERE id = 'acct-1' LIMIT 1 FOR UPDATE`,
			},
		},
		{
			name: "insert usage events is idempotent on the event id",
			run: func(s *BillingStore) error {
				_, err := s.InsertUsageEvents([]BillingUsageEvent{{
					ID: "e1", AccountID: "acct-1", ItemCode: "tts:characters",
					Quantity: 3, OccurredAt: at, ReceivedAt: at,
				}})
				return err
			},
			wantSQL: []string{
				`INSERT INTO "billing_usage_events"`,
				`"aimodel_id"`,
				`ON CONFLICT ("id") DO NOTHING`,
			},
			notWantSQL: []string{`"ai_model_id"`},
		},
		{
			name: "upsert daily stat accumulates on conflict",
			run: func(s *BillingStore) error {
				return s.UpsertDailyStat(nil, BillingDailyStat{
					AccountID: "acct-1", Date: "2026-09-20", ItemCode: "tts:characters",
					Quantity: 12, AmountMicro: 34, EventCount: 1,
				})
			},
			wantSQL: []string{
				`INSERT INTO "billing_daily_stats"`,
				`ON CONFLICT ("account_id","date","item_code","aimodel_id","voice_id") DO UPDATE SET`,
				`"quantity"=billing_daily_stats.quantity + EXCLUDED.quantity`,
				`"amount_micro"=billing_daily_stats.amount_micro + EXCLUDED.amount_micro`,
				`"event_count"=billing_daily_stats.event_count + EXCLUDED.event_count`,
			},
			notWantSQL: []string{`"ai_model_id"`},
		},
		{
			name: "expire reservations returns the rows it expired",
			run: func(s *BillingStore) error {
				_, err := s.ExpireReservations(nil, at)
				return err
			},
			wantSQL: []string{
				`UPDATE "billing_reservations" SET "status"='expired'`,
				`WHERE status = 'open' AND expires_at < '2026-09-20 10:00:00'`,
				`RETURNING *`,
			},
		},
		{
			name: "touch reservation only extends open rows",
			run: func(s *BillingStore) error {
				return s.TouchReservation(nil, "s1", at)
			},
			wantSQL: []string{
				`UPDATE "billing_reservations" SET "expires_at"='2026-09-20 10:00:00'`,
				`WHERE session_id = 's1' AND status = 'open'`,
			},
		},
		{
			name: "list grants only returns unexpired rows with headroom, soonest expiry first",
			run: func(s *BillingStore) error {
				_, err := s.ListGrants(nil, "acct-1", at)
				return err
			},
			wantSQL: []string{
				`FROM "billing_grants" WHERE account_id = 'acct-1' AND expires_at > '2026-09-20 10:00:00' AND used_micro < granted_micro`,
				`ORDER BY expires_at ASC,id ASC`,
			},
		},
		{
			name: "use grant is conditional on the remaining amount",
			run: func(s *BillingStore) error {
				return s.UseGrant(nil, "grant-1", 100)
			},
			wantSQL: []string{
				`UPDATE "billing_grants" SET "used_micro"=used_micro + 100`,
				`WHERE id = 'grant-1' AND used_micro + 100 <= granted_micro`,
			},
			wantErr: ErrNotFound, // DryRun 下没有行被更新，按额度被并发用光处理
		},
		{
			name: "insert ledger keeps the idempotency key as the guard",
			run: func(s *BillingStore) error {
				return s.InsertLedger(nil, &BillingLedger{
					AccountID: "acct-1", Direction: "debit", AmountMicro: 12,
					Kind: "charge", ItemCode: "tts:characters", IdempotencyKey: "settle:e1", OccurredAt: at,
				})
			},
			wantSQL: []string{
				`INSERT INTO "billing_ledger"`,
				`'settle:e1'`,
			},
		},
		{
			name: "mark event settled writes back the pricing snapshot",
			run: func(s *BillingStore) error {
				return s.MarkEventSettled(nil, "e1", "charged", "price-1", 12, "")
			},
			wantSQL: []string{
				`UPDATE "billing_usage_events" SET "amount_micro"=12,"last_error"='',"price_id"='price-1',"status"='charged'`,
				`WHERE id = 'e1'`,
			},
			wantErr: ErrNotFound,
		},
		{
			name: "update account balance writes absolute values under the caller's lock",
			run: func(s *BillingStore) error {
				return s.UpdateAccountBalance(nil, "acct-1", 100, 50)
			},
			wantSQL: []string{
				`UPDATE "billing_accounts" SET "balance_micro"=100,"frozen_micro"=50`,
				`WHERE id = 'acct-1'`,
			},
			wantErr: ErrNotFound,
		},
		{
			name: "upsert items never re-enables an item",
			run: func(s *BillingStore) error {
				return s.UpsertItems([]BillingItem{{
					Code: "tts:audio:seconds", Name: "TTS 音频时长",
					ChargeMode: "duration", Unit: "second", MeterSource: "tts",
				}})
			},
			wantSQL: []string{
				`INSERT INTO "billing_items"`,
				`ON CONFLICT ("code") DO UPDATE SET "name"="excluded"."name"`,
			},
			notWantSQL: []string{`"enabled"="excluded"."enabled"`},
		},
		{
			name: "sum usage by item only counts charged events",
			run: func(s *BillingStore) error {
				_, err := s.SumUsageByItem("acct-1", at, at.Add(time.Hour), 10)
				return err
			},
			wantSQL: []string{
				`SELECT item_code, SUM(quantity) AS quantity, SUM(amount_micro) AS amount_micro`,
				`status = 'charged'`,
				`occurred_at >= '2026-09-20 10:00:00' AND occurred_at < '2026-09-20 11:00:00'`,
				`GROUP BY "item_code"`,
			},
			wantErr: gorm.ErrDryRunModeUnsupported, // Scan 在 DryRun 下没有行可扫
		},
		{
			name: "list accounts escapes the keyword and groups the OR",
			run: func(s *BillingStore) error {
				_, err := s.ListAccounts(AccountQuery{Status: "active", Keyword: "50%"})
				return err
			},
			wantSQL: []string{
				`status = 'active' AND (subject_id ILIKE '%50\%%' OR id ILIKE '%50\%%')`,
			},
		},
		{
			name: "list prices can filter to the versions active at a moment",
			run: func(s *BillingStore) error {
				_, err := s.ListPrices(PriceQuery{ItemCode: "tts:characters", PlatformOnly: true, ActiveAt: &at})
				return err
			},
			wantSQL: []string{
				`item_code = 'tts:characters'`,
				`account_id = ''`,
				`effective_from <= '2026-09-20 10:00:00' AND (effective_to IS NULL OR effective_to > '2026-09-20 10:00:00')`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, rec := newDryRunBillingStore(t)
			err := tt.run(store)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("run: err = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("run: %v", err)
			}

			sql := rec.all()
			for _, want := range tt.wantSQL {
				if !strings.Contains(sql, want) {
					t.Errorf("SQL is missing %q\n--- SQL ---\n%s", want, sql)
				}
			}
			for _, notWant := range tt.notWantSQL {
				if strings.Contains(sql, notWant) {
					t.Errorf("SQL must not contain %q\n--- SQL ---\n%s", notWant, sql)
				}
			}
		})
	}
}
