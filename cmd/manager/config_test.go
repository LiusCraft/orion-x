package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/billing/service"
)

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

// TestManagerConfigAuthYAML 钉住 auth / smtp 段的 yaml 键名：这两段直接复用模块
// 配置类型（handler.AuthConfig / mailer.Config），键名写错不会有编译错误。
func TestManagerConfigAuthYAML(t *testing.T) {
	var cfg ManagerConfig
	err := yaml.Unmarshal([]byte(`
auth:
  email_verify: true
  verify_url_base: https://console.example.com
smtp:
  host: smtp.example.com
  port: 465
  tls: implicit
  username: no-reply@example.com
  from: no-reply@example.com
`), &cfg)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !cfg.Auth.EmailVerify {
		t.Error("auth.email_verify = false, want true")
	}
	if cfg.Auth.VerifyURLBase != "https://console.example.com" {
		t.Errorf("auth.verify_url_base = %q, want https://console.example.com", cfg.Auth.VerifyURLBase)
	}
	if cfg.SMTP.Host != "smtp.example.com" || cfg.SMTP.Port != 465 || cfg.SMTP.TLS != "implicit" {
		t.Errorf("smtp = %+v, want host/port/tls from yaml", cfg.SMTP)
	}
	if cfg.SMTP.From != "no-reply@example.com" {
		t.Errorf("smtp.from = %q, want no-reply@example.com", cfg.SMTP.From)
	}
}

// TestLoadManagerConfigIgnoresEnv 钉住「配置文件是唯一来源」：移除 applyManagerEnv
// 后，环境变量不能再覆盖任何字段（回归：曾经 15 个 env 会静默覆盖文件值）。
func TestLoadManagerConfigIgnoresEnv(t *testing.T) {
	envs := map[string]string{
		"DB_DSN":               "postgres://env/override",
		"JWT_SECRET":           "env-secret",
		"LOG_LEVEL":            "debug",
		"ADMIN_USERNAME":       "env-admin",
		"ADMIN_PASSWORD":       "env-password",
		"GITHUB_CLIENT_ID":     "env-client-id",
		"GITHUB_CLIENT_SECRET": "env-client-secret",
		"GITHUB_REDIRECT_URL":  "https://env.example.com/callback",
		"INTERNAL_TOKEN":       "env-internal-token",
		"EPAY_KEY":             "env-epay-key",
		"STORAGE_ENDPOINT":     "https://env.example.com",
		"STORAGE_REGION":       "env-region",
		"STORAGE_BUCKET":       "env-bucket",
		"STORAGE_ACCESS_KEY":   "env-access-key",
		"STORAGE_SECRET_KEY":   "env-secret-key",
	}
	for k, v := range envs {
		t.Setenv(k, v)
	}

	path := filepath.Join(t.TempDir(), "manager.yaml")
	if err := os.WriteFile(path, []byte("database:\n  dsn: postgres://file/db\njwt:\n  secret: file-secret\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadManagerConfig(path)
	if err != nil {
		t.Fatalf("loadManagerConfig() error = %v", err)
	}
	if cfg.Database.DSN != "postgres://file/db" {
		t.Errorf("Database.DSN = %q, want the file value", cfg.Database.DSN)
	}
	if cfg.JWT.Secret != "file-secret" {
		t.Errorf("JWT.Secret = %q, want the file value", cfg.JWT.Secret)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("Logging.Level = %q, want default info (env ignored)", cfg.Logging.Level)
	}
	if cfg.Admin.Username != "admin" {
		t.Errorf("Admin.Username = %q, want default admin (env ignored)", cfg.Admin.Username)
	}
	if cfg.Admin.Password != "" || cfg.GithubOAuth.ClientID != "" || cfg.GithubOAuth.ClientSecret != "" ||
		cfg.Internal.Token != "" || cfg.Payment.Key != "" || cfg.Storage.AccessKey != "" || cfg.Storage.SecretKey != "" {
		t.Errorf("config picked up env values: %+v", cfg)
	}
}

// TestLoadManagerConfigMissingFileFails 钉住 fail closed：配置文件缺失是启动
// 错误，不能静默退回默认值（挂着空 DSN 起来再报错会掩盖真正的漏挂载）。
func TestLoadManagerConfigMissingFileFails(t *testing.T) {
	if _, err := loadManagerConfig(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("loadManagerConfig() error = nil, want an error for a missing config file")
	}
}
