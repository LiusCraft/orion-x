package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/store"
)

// newTestPaymentService 造一个没有仓储的 PaymentService。
// 仓储是具体类型（不是接口），所以这里只能覆盖**不碰库**的分支：参数校验与限额——
// 恰好就是钱最容易被写错的那一段。
func newTestPaymentService(t *testing.T) *PaymentService {
	t.Helper()
	cfg := DefaultPaymentConfig()
	cfg.Enabled = true
	cfg.Now = func() time.Time { return time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC) }
	return NewPaymentService(nil, nil, nil, cfg)
}

func TestDefaultPaymentConfigBounds(t *testing.T) {
	cfg := DefaultPaymentConfig()

	if cfg.MinAmountMicro <= 0 {
		t.Fatalf("MinAmountMicro = %d, want positive", cfg.MinAmountMicro)
	}
	if cfg.MaxAmountMicro <= cfg.MinAmountMicro {
		t.Fatalf("MaxAmountMicro = %d, want > %d", cfg.MaxAmountMicro, cfg.MinAmountMicro)
	}
	// 两个边界都必须是整分：不是整分的限额写进配置，等于把「下单必失败」当成默认值。
	if cfg.MinAmountMicro%billing.MicroPerCent != 0 || cfg.MaxAmountMicro%billing.MicroPerCent != 0 {
		t.Fatalf("bounds must be whole cents: min=%d max=%d", cfg.MinAmountMicro, cfg.MaxAmountMicro)
	}
	if len(cfg.Channels) != len(billing.PayChannels()) {
		t.Fatalf("default channels = %v, want all built-in channels", cfg.Channels)
	}
}

func TestNormalizePaymentConfig(t *testing.T) {
	// 零值配置要能直接用：只写了 enabled: true 的部署不该因为别的字段没填就跑不起来。
	got := normalizePaymentConfig(PaymentConfig{Enabled: true})
	def := DefaultPaymentConfig()

	if got.Currency != def.Currency || got.OrderTTL != def.OrderTTL || got.Now == nil {
		t.Fatalf("zero config not normalized: %+v", got)
	}
	if got.MinAmountMicro != def.MinAmountMicro || got.MaxAmountMicro != def.MaxAmountMicro {
		t.Fatalf("bounds not normalized: min=%d max=%d", got.MinAmountMicro, got.MaxAmountMicro)
	}

	// max < min 是配错了，回落到默认值，而不是让每一笔都撞限额。
	got = normalizePaymentConfig(PaymentConfig{Enabled: true, MinAmountMicro: 100 * billing.MicroPerUnit, MaxAmountMicro: 1})
	if got.MaxAmountMicro != def.MaxAmountMicro {
		t.Fatalf("MaxAmountMicro = %d, want fallback %d", got.MaxAmountMicro, def.MaxAmountMicro)
	}
}

func TestPaymentServiceSupports(t *testing.T) {
	svc := newTestPaymentService(t)

	if !svc.Supports(billing.PayChannelAlipay) {
		t.Fatal("alipay should be supported by default")
	}

	svc.cfg.Channels = []billing.PayChannel{billing.PayChannelWxpay}
	if svc.Supports(billing.PayChannelAlipay) {
		t.Fatal("alipay should no longer be supported")
	}
	if !svc.Supports(billing.PayChannelWxpay) {
		t.Fatal("wxpay should be supported")
	}
}

func TestCreateRechargeRejects(t *testing.T) {
	const subject = "user-1"

	tests := []struct {
		name        string
		mutate      func(svc *PaymentService)
		channel     billing.PayChannel
		amountMicro int64
		want        error
	}{
		{
			name:        "payment disabled",
			mutate:      func(svc *PaymentService) { svc.cfg.Enabled = false },
			channel:     billing.PayChannelAlipay,
			amountMicro: 10 * billing.MicroPerUnit,
			want:        ErrPaymentDisabled,
		},
		{
			name:        "channel not in allowlist",
			mutate:      func(svc *PaymentService) { svc.cfg.Channels = []billing.PayChannel{billing.PayChannelWxpay} },
			channel:     billing.PayChannelAlipay,
			amountMicro: 10 * billing.MicroPerUnit,
			want:        billing.ErrUnknownChannel,
		},
		{
			name:        "garbage channel",
			mutate:      func(svc *PaymentService) {},
			channel:     billing.PayChannel("epay:paypal"),
			amountMicro: 10 * billing.MicroPerUnit,
			want:        billing.ErrUnknownChannel,
		},
		{
			name:        "below minimum",
			mutate:      func(svc *PaymentService) {},
			channel:     billing.PayChannelAlipay,
			amountMicro: billing.MicroPerCent, // ¥0.01
			want:        ErrInvalidRequest,
		},
		{
			name:        "above maximum",
			mutate:      func(svc *PaymentService) {},
			channel:     billing.PayChannelAlipay,
			amountMicro: 100_000 * billing.MicroPerUnit,
			want:        ErrInvalidRequest,
		},
		{
			// 网关的 money 只有两位小数：微单位级别的零头必须在**下单前**就拒掉，
			// 不能等到出站时才报错，更不能被四舍五入成另一个金额。
			name:        "finer than a cent",
			mutate:      func(svc *PaymentService) {},
			channel:     billing.PayChannelAlipay,
			amountMicro: 10*billing.MicroPerUnit + 1,
			want:        ErrInvalidRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestPaymentService(t)
			tc.mutate(svc)

			_, _, err := svc.CreateRecharge(context.Background(), billing.SubjectTypeUser, subject, tc.channel, tc.amountMicro, "203.0.113.7")
			if !errors.Is(err, tc.want) {
				t.Fatalf("CreateRecharge error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCreateRechargeRejectsEmptySubject(t *testing.T) {
	svc := newTestPaymentService(t)
	for _, tc := range []struct{ subjectType, subjectID string }{
		{"", "user-1"},
		{billing.SubjectTypeUser, ""},
		{"  ", "user-1"},
	} {
		if _, _, err := svc.CreateRecharge(context.Background(), tc.subjectType, tc.subjectID,
			billing.PayChannelAlipay, 10*billing.MicroPerUnit, ""); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("CreateRecharge(%q, %q) error = %v, want ErrInvalidRequest", tc.subjectType, tc.subjectID, err)
		}
	}
}

func TestRefundNote(t *testing.T) {
	order := &store.PaymentOrder{
		OutTradeNo:  "p0123456789abcdef0123",
		Channel:     "epay:alipay",
		AmountMicro: 9_990_000,
	}

	// 操作者填的那句话必须留在账上：将来查「这笔钱为什么退了」时，
	// 只有系统自动生成的半句话是不够的。
	got := refundNote(order, "  用户申请（工单 #123）  ")
	if !strings.Contains(got, "9.99") || !strings.Contains(got, order.OutTradeNo) || !strings.Contains(got, "工单 #123") {
		t.Fatalf("refundNote = %q", got)
	}

	if got := refundNote(order, "   "); strings.Contains(got, "：") {
		t.Fatalf("blank note should not be appended: %q", got)
	}
}

func TestEncodeNotifyParams(t *testing.T) {
	// 原始事实要可读且稳定（键有序），排障时靠它对比两次通知差在哪。
	got := encodeNotifyParams(map[string]string{"trade_no": "t1", "money": "9.99", "out_trade_no": "p1"})
	if want := "money=9.99&out_trade_no=p1&trade_no=t1"; got != want {
		t.Fatalf("encodeNotifyParams = %q, want %q", got, want)
	}
	if got := encodeNotifyParams(nil); got != "" {
		t.Fatalf("encodeNotifyParams(nil) = %q, want empty", got)
	}
}
