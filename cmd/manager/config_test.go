package main

import (
	"strings"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/billing/service"
)

func boolPtr(v bool) *bool { return &v }

func TestBillingConfigDisabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  BillingConfig
		want bool
	}{
		{name: "missing section means enabled", cfg: BillingConfig{}, want: false},
		{name: "enabled true", cfg: BillingConfig{Enabled: boolPtr(true)}, want: false},
		{name: "enabled false", cfg: BillingConfig{Enabled: boolPtr(false)}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Disabled(); got != tc.want {
				t.Fatalf("Disabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBillingConfigServiceConfig(t *testing.T) {
	fixed := time.Date(2026, 9, 20, 10, 30, 0, 0, time.UTC)
	now := func() time.Time { return fixed }

	cases := []struct {
		name    string
		cfg     BillingConfig
		wantErr string
		want    func(t *testing.T, got service.Config)
	}{
		{
			name: "zero config keeps every default",
			cfg:  BillingConfig{},
			want: func(t *testing.T, got service.Config) {
				def := service.DefaultConfig()
				if got.Currency != def.Currency {
					t.Errorf("Currency = %q, want %q", got.Currency, def.Currency)
				}
				if got.ReserveSeconds != def.ReserveSeconds || got.ReserveTTL != def.ReserveTTL {
					t.Errorf("reserve = (%d, %v), want (%d, %v)", got.ReserveSeconds, got.ReserveTTL, def.ReserveSeconds, def.ReserveTTL)
				}
				if got.SettlementMode != billing.SettlementAsync || got.OverdraftPolicy != billing.OverdraftDeny {
					t.Errorf("modes = (%q, %q), want (%q, %q)",
						got.SettlementMode, got.OverdraftPolicy, billing.SettlementAsync, billing.OverdraftDeny)
				}
				if got.SignupGrantMicro != def.SignupGrantMicro {
					t.Errorf("SignupGrantMicro = %d, want %d", got.SignupGrantMicro, def.SignupGrantMicro)
				}
				if got.PeriodLocation.String() != "UTC" {
					t.Errorf("PeriodLocation = %v, want UTC", got.PeriodLocation)
				}
			},
		},
		{
			name: "non-zero fields override defaults",
			cfg: BillingConfig{
				Currency:         "USD",
				ReserveSeconds:   120,
				ReserveRateMicro: 1500,
				SettlementMode:   billing.SettlementSync,
				OverdraftPolicy:  billing.OverdraftAllow,
				PeriodTimezone:   "Asia/Shanghai",
				SignupGrantMicro: 1_234_000,
			},
			want: func(t *testing.T, got service.Config) {
				if got.Currency != "USD" {
					t.Errorf("Currency = %q, want USD", got.Currency)
				}
				if got.ReserveSeconds != 120 || got.ReserveRateMicro != 1500 {
					t.Errorf("reserve = (%d, %d), want (120, 1500)", got.ReserveSeconds, got.ReserveRateMicro)
				}
				if got.SettlementMode != billing.SettlementSync || got.OverdraftPolicy != billing.OverdraftAllow {
					t.Errorf("modes = (%q, %q), want (%q, %q)",
						got.SettlementMode, got.OverdraftPolicy, billing.SettlementSync, billing.OverdraftAllow)
				}
				if got.SignupGrantMicro != 1_234_000 {
					t.Errorf("SignupGrantMicro = %d, want 1234000", got.SignupGrantMicro)
				}
				if got.PeriodLocation.String() != "Asia/Shanghai" {
					t.Errorf("PeriodLocation = %v, want Asia/Shanghai", got.PeriodLocation)
				}
			},
		},
		{
			name:    "unknown timezone is rejected",
			cfg:     BillingConfig{PeriodTimezone: "Mars/Olympus"},
			wantErr: "billing.period_timezone",
		},
		{
			name:    "blank timezone falls back to UTC",
			cfg:     BillingConfig{PeriodTimezone: "   "},
			wantErr: "",
			want: func(t *testing.T, got service.Config) {
				if got.PeriodLocation != time.UTC {
					t.Errorf("PeriodLocation = %v, want UTC", got.PeriodLocation)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.cfg.ServiceConfig(now)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("ServiceConfig() error = nil, want an error containing %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ServiceConfig() error = %v, want it to mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ServiceConfig() error = %v", err)
			}
			if got.Now == nil || !got.Now().Equal(fixed) {
				t.Errorf("Now() = %v, want the injected clock %v", got.Now(), fixed)
			}
			if tc.want != nil {
				tc.want(t, got)
			}
		})
	}
}

func TestAPIKeyConfigDisabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  APIKeyConfig
		want bool
	}{
		// 与 payment 相反、与 billing 也不同：凭证默认关闭，写这一段才是显式决定。
		{name: "missing section means disabled", cfg: APIKeyConfig{}, want: true},
		{name: "enabled true", cfg: APIKeyConfig{Enabled: boolPtr(true)}, want: false},
		{name: "enabled false", cfg: APIKeyConfig{Enabled: boolPtr(false)}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Disabled(); got != tc.want {
				t.Fatalf("Disabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAPIKeyConfigServiceConfig(t *testing.T) {
	def := apikey.DefaultConfig()

	cases := []struct {
		name string
		cfg  APIKeyConfig
		want apikey.Config
	}{
		{name: "empty section keeps defaults", cfg: APIKeyConfig{}, want: def},
		{
			name: "explicit values win",
			cfg: APIKeyConfig{
				RateLimit:     APIKeyRateLimitConfig{RPS: 5, Burst: 10},
				CounterFlush:  "45s",
				MaxPerAccount: 7,
			},
			want: apikey.Config{RateLimitRPS: 5, RateLimitBurst: 10, CounterFlush: 45 * time.Second, MaxKeysPerAccount: 7},
		},
		{
			name: "zeros and blanks fall back to defaults",
			cfg: APIKeyConfig{
				RateLimit:     APIKeyRateLimitConfig{RPS: 0, Burst: 0},
				CounterFlush:  "0s",
				MaxPerAccount: 0,
			},
			want: def,
		},
		{
			// 写错的窗口最危险：负值会让每次 touch 都刷一次库，解析不了就回默认。
			name: "broken durations fall back to defaults",
			cfg:  APIKeyConfig{CounterFlush: "-30s"},
			want: def,
		},
		{
			name: "unknown duration falls back to defaults",
			cfg:  APIKeyConfig{CounterFlush: "soon"},
			want: def,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.cfg.ServiceConfig()
			if got.RateLimitRPS != tc.want.RateLimitRPS ||
				got.RateLimitBurst != tc.want.RateLimitBurst ||
				got.CounterFlush != tc.want.CounterFlush ||
				got.MaxKeysPerAccount != tc.want.MaxKeysPerAccount {
				t.Fatalf("ServiceConfig() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
