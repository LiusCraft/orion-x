package main

import (
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/billing"
)

// TestPaymentConfigDisabled 钉住「默认关闭」这条决定。
//
// 它和 BillingConfig 刚好相反（计费是默认启用）：支付默认开着而商户密钥没配，
// 用户点「充值」那一刻才会失败——那种失败比启动时就说「没接支付」难查得多。
func TestPaymentConfigDisabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  PaymentConfig
		want bool
	}{
		{"missing section means disabled", PaymentConfig{}, true},
		{"explicit false", PaymentConfig{Enabled: boolPtr(false)}, true},
		{"explicit true", PaymentConfig{Enabled: boolPtr(true)}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Disabled(); got != tc.want {
				t.Fatalf("Disabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPaymentConfigChannelsAllowlist(t *testing.T) {
	// 不写 channels 就是全开。
	got, err := PaymentConfig{}.ChannelsAllowlist()
	if err != nil {
		t.Fatalf("default channels: %v", err)
	}
	if len(got) != len(billing.PayChannels()) {
		t.Fatalf("default channels = %v, want all built-in channels", got)
	}

	got, err = PaymentConfig{Channels: []string{"epay:alipay", " epay:wxpay "}}.ChannelsAllowlist()
	if err != nil {
		t.Fatalf("explicit channels: %v", err)
	}
	if len(got) != 2 || got[0] != billing.PayChannelAlipay || got[1] != billing.PayChannelWxpay {
		t.Fatalf("explicit channels = %v", got)
	}

	// 渠道名写错一个字母要在启动时炸，不要拖到用户下单。
	if _, err := (PaymentConfig{Channels: []string{"alipay"}}).ChannelsAllowlist(); err == nil {
		t.Fatal("unknown channel should fail validation")
	}
}

func TestPaymentConfigOrderTTL(t *testing.T) {
	if got := (PaymentConfig{OrderTTL: "45m"}).OrderTTLDuration(); got != 45*time.Minute {
		t.Fatalf("OrderTTLDuration = %s, want 45m", got)
	}
	// 空值 / 写错的值回落到 service 的默认值（0 表示「用默认」），不能让一只写错的
	// 订单有效期变成「立刻过期」。
	for _, bad := range []string{"", "   ", "半小时", "-5m"} {
		if got := (PaymentConfig{OrderTTL: bad}).OrderTTLDuration(); got != 0 {
			t.Errorf("OrderTTLDuration(%q) = %s, want 0", bad, got)
		}
	}
}

func TestPaymentConfigServiceConfig(t *testing.T) {
	cfg := PaymentConfig{
		MinAmountMicro: 2 * billing.MicroPerUnit,
		MaxAmountMicro: 500 * billing.MicroPerUnit,
		OrderTTL:       "10m",
		SubjectPrefix:  "钱包充值 ",
	}
	channels := []billing.PayChannel{billing.PayChannelWxpay}

	got := cfg.PaymentServiceConfig(channels)
	if got.MinAmountMicro != cfg.MinAmountMicro || got.MaxAmountMicro != cfg.MaxAmountMicro {
		t.Fatalf("bounds = [%d, %d], want [%d, %d]",
			got.MinAmountMicro, got.MaxAmountMicro, cfg.MinAmountMicro, cfg.MaxAmountMicro)
	}
	if got.OrderTTL != 10*time.Minute {
		t.Fatalf("OrderTTL = %s, want 10m", got.OrderTTL)
	}
	if got.SubjectPrefix != cfg.SubjectPrefix {
		t.Fatalf("SubjectPrefix = %q", got.SubjectPrefix)
	}
	if len(got.Channels) != 1 || got.Channels[0] != billing.PayChannelWxpay {
		t.Fatalf("Channels = %v", got.Channels)
	}
}
