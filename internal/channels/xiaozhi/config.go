package xiaozhi

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds Xiaozhi WebSocket connector server-level settings
// (not device-level — those come from the manager service).
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Health  HealthConfig  `yaml:"health"`
	Manager ManagerConfig `yaml:"manager"`
	Auth    AuthConfig    `yaml:"auth"`
	Billing BillingConfig `yaml:"billing"`
	Logging LoggingConfig `yaml:"logging"`
}

// ServerConfig holds the HTTP server listen address and WebSocket path.
type ServerConfig struct {
	Addr   string `yaml:"addr"`
	WsPath string `yaml:"ws_path"`
}

// HealthConfig holds the process-level health endpoint settings.
type HealthConfig struct {
	Addr string `yaml:"addr"`
}

// ManagerConfig holds the manager service URL and the internal service token.
type ManagerConfig struct {
	URL string `yaml:"url"`
	// Token 是访问 /internal/* 的 Bearer token（计费与接入鉴权都要），必须与
	// manager 的 internal.token 一致。没配的话控制面会拒掉所有计费请求与密钥校验
	// （§14.1），后者意味着带密钥的接入会被拒。
	Token string `yaml:"token"`
}

// AuthConfig 是握手时的接入鉴权开关。
//
// 默认（RequireAPIKey=false）只校验“客户端主动带来的密钥”：带了就要对，不带就
// 照旧按 device_id 接入——存量设备不带凭据，一刀切会全部连不上。打开开关后
// 没有密钥的连接一律拒，用于把设备接入锁死。
type AuthConfig struct {
	// RequireAPIKey 为 true 时，只有带有效 API Key（apikey:scope:voice）的连接
	// 才被接受。
	RequireAPIKey bool `yaml:"require_api_key"`
}

// BillingConfig 是数据面计费的本地开关。
type BillingConfig struct {
	// Enabled 为 false 时整个计费链路关闭（等价于给通道传 nil sink）。
	Enabled *bool `yaml:"enabled"`
	// SuspensionTTL 是“已被停服”标记的本地 TTL：控制面不可达时，上次明确
	// 拒绝过 account_suspended 的设备仍然拒（§14.3）。
	SuspensionTTL time.Duration `yaml:"suspension_ttl"`
}

// BillingEnabled 返回计费是否开启（默认开启）。
func (c BillingConfig) BillingEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// LoggingConfig holds logging level/format.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Server:  ServerConfig{Addr: ":8080", WsPath: "/ws"},
		Health:  HealthConfig{Addr: ":8081"},
		Manager: ManagerConfig{},
		Logging: LoggingConfig{Level: "info", Format: "console"},
	}
}

// LoadConfig reads the YAML config file and applies environment overrides.
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			applyEnv(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	applyEnv(cfg)
	return cfg, nil
}

func applyEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("MANAGER_URL")); v != "" {
		cfg.Manager.URL = v
	}
	if v := strings.TrimSpace(os.Getenv("MANAGER_TOKEN")); v != "" {
		cfg.Manager.Token = v
	}
	if v := strings.TrimSpace(os.Getenv("LOG_LEVEL")); v != "" {
		cfg.Logging.Level = v
	}
	if v := strings.TrimSpace(os.Getenv("HEALTH_ADDR")); v != "" {
		cfg.Health.Addr = v
	}
}

// ValidateManagerURL ensures channels can reach the manager before startup.
func ValidateManagerURL(rawURL string) error {
	managerURL := strings.TrimSpace(rawURL)
	if managerURL == "" {
		return errors.New("manager.url is required (set it in the config file or MANAGER_URL)")
	}
	u, err := url.Parse(managerURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("manager.url must be an absolute http(s) URL: %q", managerURL)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("manager.url must use http or https: %q", managerURL)
	}
	return nil
}
