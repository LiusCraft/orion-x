package xiaozhi

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds Xiaozhi WebSocket connector server-level settings
// (not device-level — those come from the manager service).
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Health  HealthConfig  `yaml:"health"`
	Manager ManagerConfig `yaml:"manager"`
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

// ManagerConfig holds the manager service URL.
type ManagerConfig struct {
	URL string `yaml:"url"`
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
