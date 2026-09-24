package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/billing"
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
	Payment     PaymentConfig     `yaml:"payment"`
	APIKey      APIKeyConfig      `yaml:"apikey"`
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

// PaymentConfig 是充值通道（支付渠道 → 余额）的接入参数。
//
// 整段可以不写：不写就是没接支付渠道，/api/billing/recharge 回 503，其余能力不受
// 影响。商户密钥建议只走环境变量 EPAY_KEY，不落配置文件。
//
// 一期只接易支付（EPay）一家，所以字段是平的；将来接第二家渠道时再改成按渠道分组。
type PaymentConfig struct {
	Enabled    *bool  `yaml:"enabled"`      // nil = 未接入；显式 true 才启用
	APIBaseURL string `yaml:"api_base_url"` // 网关地址，如 https://pay.example.com
	PID        int    `yaml:"pid"`          // 商户 ID
	Key        string `yaml:"key"`          // 商户密钥（建议只走 EPAY_KEY 环境变量）
	NotifyURL  string `yaml:"notify_url"`   // 异步通知地址，必须公网可达
	ReturnURL  string `yaml:"return_url"`   // 同步跳转地址
	// Channels 是开放的支付渠道（epay:alipay / epay:wxpay / epay:qqpay），
	// 留空 = 全部开放。
	Channels []string `yaml:"channels"`
	// MinAmountMicro / MaxAmountMicro 是单笔充值上下限（微元，1000000 = ￥1）。
	MinAmountMicro int64 `yaml:"min_amount_micro"`
	MaxAmountMicro int64 `yaml:"max_amount_micro"`
	// OrderTTL 是订单有效期，如 "30m"。
	OrderTTL string `yaml:"order_ttl"`
	// SubjectPrefix 是订单标题前缀（收银台上显示的那句话）。
	SubjectPrefix string `yaml:"subject_prefix"`
	// Timeout 是请求网关的超时秒数，0 = SDK 默认 30s。
	Timeout int `yaml:"timeout"`
	// Debug 打开 SDK 的请求日志（会打印含签名的完整 URL），生产别开。
	Debug bool `yaml:"debug"`
}

// Disabled 判断充值通道是否关闭。和 BillingConfig 相反：支付是**默认关闭**的
// ——没配商户号密钥就启用，只会在用户点「充值」那一刻失败。
func (c PaymentConfig) Disabled() bool {
	return c.Enabled == nil || !*c.Enabled
}

// ChannelsAllowlist 把配置里的字符串渠道解析成领域枚举，顺带校验拼写：
// 渠道名写错一个字母，用户下单时才会发现，不如启动时报错。
func (c PaymentConfig) ChannelsAllowlist() ([]billing.PayChannel, error) {
	if len(c.Channels) == 0 {
		return billing.PayChannels(), nil
	}
	out := make([]billing.PayChannel, 0, len(c.Channels))
	for _, raw := range c.Channels {
		ch, err := billing.ParsePayChannel(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("payment.channels: %w", err)
		}
		out = append(out, ch)
	}
	return out, nil
}

// OrderTTLDuration 解析订单有效期；空值、写错的值、非正值都回落到 0
// （0 表示「用 service 的默认值」）。负的 TTL 最危险：它会让 ExpiresAt 落在过去，
// 每一笔订单都出生即过期，然后被 Sweep 立刻关掉。
func (c PaymentConfig) OrderTTLDuration() time.Duration {
	if strings.TrimSpace(c.OrderTTL) == "" {
		return 0
	}
	d, err := time.ParseDuration(strings.TrimSpace(c.OrderTTL))
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// PaymentServiceConfig 把 yaml 里的配置翻译成 service.PaymentConfig。
func (c PaymentConfig) PaymentServiceConfig(channels []billing.PayChannel) service.PaymentConfig {
	cfg := service.DefaultPaymentConfig()
	if c.MinAmountMicro > 0 {
		cfg.MinAmountMicro = c.MinAmountMicro
	}
	if c.MaxAmountMicro > 0 {
		cfg.MaxAmountMicro = c.MaxAmountMicro
	}
	if ttl := c.OrderTTLDuration(); ttl > 0 {
		cfg.OrderTTL = ttl
	}
	if len(channels) > 0 {
		cfg.Channels = channels
	}
	if v := c.SubjectPrefix; strings.TrimSpace(v) != "" {
		// 原样保留（包括尾部空格）：这是收银台上给用户看的文案，
		// 「余额充值 9.99」和「余额充值9.99」是两个不同的东西。
		cfg.SubjectPrefix = v
	}
	return cfg
}

// APIKeyConfig 是 API Key（凭证）功能的接入参数（docs/api-key-design.md §7.3）。
//
// 与 payment 一样是**默认关闭**：不写这一段的部署行为与以前完全一样——带
// ox_sk_ 前缀的请求 401，管理面 503。
type APIKeyConfig struct {
	Enabled *bool `yaml:"enabled"` // nil = 关闭；显式 true 才启用

	RateLimit APIKeyRateLimitConfig `yaml:"rate_limit"`
	// CounterFlush 是用量计数（last_used_at / call_count）的刷库窗口，如 "30s"。
	// 计数是非精确口径，不得用于计费或对账。
	CounterFlush string `yaml:"counter_flush"`
	// MaxPerAccount 是单账号的 key 上限（§3.2 的容量假设）。
	MaxPerAccount int `yaml:"max_per_account"`
	// AdminOnly 是灰度期的收口：true 时只有管理员能在控制台创建新 Key（§7.3）。
	AdminOnly bool `yaml:"admin_only"`
}

// APIKeyRateLimitConfig 是每 key 的令牌桶参数（§1 D8）。
type APIKeyRateLimitConfig struct {
	RPS   float64 `yaml:"rps"`   // 每秒补充的令牌数
	Burst int     `yaml:"burst"` // 桶容量（瞬时突发）
}

// Disabled 判断凭证功能是否关闭。没写这一段就是关闭：给用户签发长期凭证这件事
// 应该是一次显式的决定，而不是升级镜像时顺手打开的。
func (c APIKeyConfig) Disabled() bool {
	return c.Enabled == nil || !*c.Enabled
}

// CounterFlushDuration 解析刷库窗口；空值、写错、非正值都回落到 0（= 用 domain 默认值）。
// 负的窗口最危险：它会让每次 touch 都触发一次刷库。
func (c APIKeyConfig) CounterFlushDuration() time.Duration {
	if strings.TrimSpace(c.CounterFlush) == "" {
		return 0
	}
	d, err := time.ParseDuration(strings.TrimSpace(c.CounterFlush))
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// ServiceConfig 把 yaml 里的配置翻译成 apikey.Config：0 / 空 = 用默认值。
func (c APIKeyConfig) ServiceConfig() apikey.Config {
	cfg := apikey.DefaultConfig()
	if c.RateLimit.RPS > 0 {
		cfg.RateLimitRPS = c.RateLimit.RPS
	}
	if c.RateLimit.Burst > 0 {
		cfg.RateLimitBurst = c.RateLimit.Burst
	}
	if d := c.CounterFlushDuration(); d > 0 {
		cfg.CounterFlush = d
	}
	if c.MaxPerAccount > 0 {
		cfg.MaxKeysPerAccount = c.MaxPerAccount
	}
	return cfg
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
	// 支付网关的商户密钥同理：只走环境变量，不落配置文件
	if v := strings.TrimSpace(os.Getenv("EPAY_KEY")); v != "" {
		cfg.Payment.Key = v
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
