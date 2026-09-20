package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/liuscraft/orion-x/internal/billing/service"
	"github.com/liuscraft/orion-x/internal/storage"
)

type ManagerConfig struct {
	Server      ServerConfig      `yaml:"server"`
	Database    DatabaseConfig    `yaml:"database"`
	JWT         JWTConfig         `yaml:"jwt"`
	Admin       AdminConfig       `yaml:"admin"`
	GithubOAuth GithubOAuthConfig `yaml:"github_oauth"`
	Logging     LoggingConfig     `yaml:"logging"`
	Storage     storage.Config    `yaml:"storage"`
	Internal    InternalConfig    `yaml:"internal"`
	Billing     BillingConfig     `yaml:"billing"`
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
}

type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

type JWTConfig struct {
	Secret string `yaml:"secret"`
}

type AdminConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type GithubOAuthConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURL  string `yaml:"redirect_url"` // GitHub OAuth 回调地址
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// InternalConfig 是服务间调用的共享凭据。
//
// token 没配时 /internal/billing/* **拒绝一切请求**（不是“不鉴权”）：忘了配如果等于
// 放行，那迟早会发生在生产上（docs/billing-design.md §14.1）。
type InternalConfig struct {
	Token string `yaml:"token"`
}

// BillingConfig 是计费控制面的运行参数（docs/billing-design.md §6 / §14）。
// 零值可用：ServiceConfig 会拿 service.DefaultConfig() 兜底。
type BillingConfig struct {
	Enabled          *bool  `yaml:"enabled"`            // nil = 启用；显式 false 关闭
	Currency         string `yaml:"currency"`           //
	ReserveSeconds   int    `yaml:"reserve_seconds"`    // 预冻结覆盖的会话时长
	ReserveRateMicro int64  `yaml:"reserve_rate_micro"` // 每秒预留额（微元），0 = 按单价估算
	SettlementMode   string `yaml:"settlement_mode"`    // sync | async
	OverdraftPolicy  string `yaml:"overdraft_policy"`   // deny | allow
	PeriodTimezone   string `yaml:"period_timezone"`    // 如 Asia/Shanghai，默认 UTC
	SignupGrantMicro int64  `yaml:"signup_grant_micro"` // 新账户注册赠款总额
	// FallbackPriceMicro 是“会话计费项还没有任何生效价格”时插入的 item 级兜底单价
	// （微元/token、微元/秒、微元/字符）。为 0 时不插：价格表为空的话每个会话都会
	// 被 price_missing 拒掉，这是有意的 fail closed（docs/billing-design.md §3.4）。
	FallbackPriceMicro int64 `yaml:"fallback_price_micro"`
}

// Disabled 判断计费是否被显式关掉。只有写了 `enabled: false` 才算关闭——没配这一
// 段时按启用处理，跟计费模块“默认就位”的其它默认值一致。
func (c BillingConfig) Disabled() bool {
	return c.Enabled != nil && !*c.Enabled
}

// ServiceConfig 把 yaml 里的配置翻译成 service.Config：先取默认值，再用配置里
// 非零的字段覆盖；账期时区用 time.LoadLocation 解析，解析不了就报错——把
// “账期按哪个时区切”弄错，等于每个月初都会算错一批钱。
func (c BillingConfig) ServiceConfig(now func() time.Time) (service.Config, error) {
	cfg := service.DefaultConfig()
	if v := strings.TrimSpace(c.Currency); v != "" {
		cfg.Currency = v
	}
	if c.ReserveSeconds > 0 {
		cfg.ReserveSeconds = c.ReserveSeconds
	}
	if c.ReserveRateMicro > 0 {
		cfg.ReserveRateMicro = c.ReserveRateMicro
	}
	if v := strings.TrimSpace(c.SettlementMode); v != "" {
		cfg.SettlementMode = v
	}
	if v := strings.TrimSpace(c.OverdraftPolicy); v != "" {
		cfg.OverdraftPolicy = v
	}
	if c.SignupGrantMicro > 0 {
		cfg.SignupGrantMicro = c.SignupGrantMicro
	}
	if now != nil {
		cfg.Now = now
	}
	if tz := strings.TrimSpace(c.PeriodTimezone); tz != "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return service.Config{}, fmt.Errorf("billing.period_timezone %q: %w", tz, err)
		}
		cfg.PeriodLocation = loc
	}
	return cfg, nil
}

func defaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		Server:   ServerConfig{Addr: ":9090"},
		Database: DatabaseConfig{},
		JWT:      JWTConfig{},
		Admin:    AdminConfig{Username: "admin"},
		Logging:  LoggingConfig{Level: "info", Format: "console"},
	}
}

func loadManagerConfig(path string) (*ManagerConfig, error) {
	cfg := defaultManagerConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	applyManagerEnv(cfg)
	return cfg, nil
}

func applyManagerEnv(cfg *ManagerConfig) {
	if v := strings.TrimSpace(os.Getenv("DB_DSN")); v != "" {
		cfg.Database.DSN = v
	}
	if v := strings.TrimSpace(os.Getenv("JWT_SECRET")); v != "" {
		cfg.JWT.Secret = v
	}
	if v := strings.TrimSpace(os.Getenv("LOG_LEVEL")); v != "" {
		cfg.Logging.Level = v
	}
	if v := strings.TrimSpace(os.Getenv("ADMIN_USERNAME")); v != "" {
		cfg.Admin.Username = v
	}
	if v := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD")); v != "" {
		cfg.Admin.Password = v
	}
	if v := strings.TrimSpace(os.Getenv("GITHUB_CLIENT_ID")); v != "" {
		cfg.GithubOAuth.ClientID = v
	}
	if v := strings.TrimSpace(os.Getenv("GITHUB_CLIENT_SECRET")); v != "" {
		cfg.GithubOAuth.ClientSecret = v
	}
	if v := strings.TrimSpace(os.Getenv("GITHUB_REDIRECT_URL")); v != "" {
		cfg.GithubOAuth.RedirectURL = v
	}
	// 服务间凭据建议只走环境变量，不落配置文件
	if v := strings.TrimSpace(os.Getenv("INTERNAL_TOKEN")); v != "" {
		cfg.Internal.Token = v
	}
	// 对象存储（AK/SK 建议只走环境变量，不落配置文件）
	if v := strings.TrimSpace(os.Getenv("STORAGE_ENDPOINT")); v != "" {
		cfg.Storage.Endpoint = v
	}
	if v := strings.TrimSpace(os.Getenv("STORAGE_REGION")); v != "" {
		cfg.Storage.Region = v
	}
	if v := strings.TrimSpace(os.Getenv("STORAGE_BUCKET")); v != "" {
		cfg.Storage.Bucket = v
	}
	if v := strings.TrimSpace(os.Getenv("STORAGE_ACCESS_KEY")); v != "" {
		cfg.Storage.AccessKey = v
	}
	if v := strings.TrimSpace(os.Getenv("STORAGE_SECRET_KEY")); v != "" {
		cfg.Storage.SecretKey = v
	}
}
