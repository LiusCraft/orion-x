package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const DefaultManagerPath = "data/manager.json"

type ManagerAppConfig struct {
	Logging   LoggingConfig          `json:"logging"`
	Server    ManagerServerConfig    `json:"server"`
	Database  ManagerDatabaseConfig  `json:"database"`
	Security  ManagerSecurityConfig  `json:"security"`
	Auth      ManagerAuthConfig      `json:"auth"`
	Migration ManagerMigrationConfig `json:"migration"`
}

type ManagerServerConfig struct {
	Address        string `json:"address"`
	ReadTimeoutMs  int    `json:"read_timeout_ms"`
	WriteTimeoutMs int    `json:"write_timeout_ms"`
	HealthPath     string `json:"health_path"`
}

type ManagerDatabaseConfig struct {
	DSN               string `json:"dsn"`
	MaxOpenConns      int    `json:"max_open_conns"`
	MaxIdleConns      int    `json:"max_idle_conns"`
	ConnMaxLifetimeMs int    `json:"conn_max_lifetime_ms"`
	ConnMaxIdleTimeMs int    `json:"conn_max_idle_time_ms"`
	PingTimeoutMs     int    `json:"ping_timeout_ms"`
}

type ManagerMigrationConfig struct {
	AutoMigrateOnStartup bool `json:"auto_migrate_on_startup"`
}

type ManagerSecurityConfig struct {
	AccessKeyCipherSecret string `json:"access_key_cipher_secret"`
}

type ManagerAuthConfig struct {
	JWTSecret              string `json:"jwt_secret"`
	Issuer                 string `json:"issuer"`
	AccessTokenTTLSeconds  int    `json:"access_token_ttl_seconds"`
	RefreshTokenTTLSeconds int    `json:"refresh_token_ttl_seconds"`
}

func DefaultManagerConfig() *ManagerAppConfig {
	return &ManagerAppConfig{
		Logging: LoggingConfig{},
		Server: ManagerServerConfig{
			Address:        "127.0.0.1:8081",
			ReadTimeoutMs:  5000,
			WriteTimeoutMs: 5000,
			HealthPath:     "/healthz",
		},
		Database: ManagerDatabaseConfig{
			DSN:               "host=127.0.0.1 user=postgres password=postgres dbname=orion_manager port=5432 sslmode=disable TimeZone=UTC",
			MaxOpenConns:      20,
			MaxIdleConns:      10,
			ConnMaxLifetimeMs: 300000,
			ConnMaxIdleTimeMs: 120000,
			PingTimeoutMs:     2000,
		},
		Security: ManagerSecurityConfig{
			AccessKeyCipherSecret: "manager-dev-access-key-cipher-secret",
		},
		Auth: ManagerAuthConfig{
			JWTSecret:              "manager-dev-jwt-secret",
			Issuer:                 "orion-x-manager",
			AccessTokenTTLSeconds:  900,
			RefreshTokenTTLSeconds: 1209600,
		},
		Migration: ManagerMigrationConfig{
			AutoMigrateOnStartup: true,
		},
	}
}

func LoadManager(path string) (*ManagerAppConfig, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = DefaultManagerPath
	}

	cfg := DefaultManagerConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg.ApplyEnv()
			return cfg, cfg.Validate()
		}
		return nil, fmt.Errorf("read manager config %s: %w", path, err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse manager config %s: %w", path, err)
	}

	cfg.ApplyEnv()
	return cfg, cfg.Validate()
}

func (c *ManagerAppConfig) ApplyEnv() {
	if level := strings.TrimSpace(os.Getenv("LOG_LEVEL")); level != "" {
		c.Logging.Level = level
	}
	if format := strings.TrimSpace(os.Getenv("LOG_FORMAT")); format != "" {
		c.Logging.Format = format
	}

	if addr := strings.TrimSpace(os.Getenv("MANAGER_SERVER_ADDRESS")); addr != "" {
		c.Server.Address = addr
	}
	if path := strings.TrimSpace(os.Getenv("MANAGER_HEALTH_PATH")); path != "" {
		c.Server.HealthPath = path
	}

	if dsn := strings.TrimSpace(os.Getenv("MANAGER_DB_DSN")); dsn != "" {
		c.Database.DSN = dsn
	}
	if value := strings.TrimSpace(os.Getenv("MANAGER_DB_MAX_OPEN_CONNS")); value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			c.Database.MaxOpenConns = n
		}
	}
	if value := strings.TrimSpace(os.Getenv("MANAGER_DB_MAX_IDLE_CONNS")); value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			c.Database.MaxIdleConns = n
		}
	}
	if value := strings.TrimSpace(os.Getenv("MANAGER_DB_CONN_MAX_LIFETIME_MS")); value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			c.Database.ConnMaxLifetimeMs = n
		}
	}
	if value := strings.TrimSpace(os.Getenv("MANAGER_DB_CONN_MAX_IDLE_TIME_MS")); value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			c.Database.ConnMaxIdleTimeMs = n
		}
	}
	if value := strings.TrimSpace(os.Getenv("MANAGER_DB_PING_TIMEOUT_MS")); value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			c.Database.PingTimeoutMs = n
		}
	}

	if secret := strings.TrimSpace(os.Getenv("MANAGER_ACCESS_KEY_CIPHER_SECRET")); secret != "" {
		c.Security.AccessKeyCipherSecret = secret
	}

	if secret := strings.TrimSpace(os.Getenv("MANAGER_AUTH_JWT_SECRET")); secret != "" {
		c.Auth.JWTSecret = secret
	}
	if issuer := strings.TrimSpace(os.Getenv("MANAGER_AUTH_ISSUER")); issuer != "" {
		c.Auth.Issuer = issuer
	}
	if value := strings.TrimSpace(os.Getenv("MANAGER_AUTH_ACCESS_TOKEN_TTL_SECONDS")); value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			c.Auth.AccessTokenTTLSeconds = n
		}
	}
	if value := strings.TrimSpace(os.Getenv("MANAGER_AUTH_REFRESH_TOKEN_TTL_SECONDS")); value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			c.Auth.RefreshTokenTTLSeconds = n
		}
	}
}

func (c *ManagerAppConfig) Validate() error {
	if strings.TrimSpace(c.Server.Address) == "" {
		return errors.New("server.address must not be empty")
	}
	if _, _, err := net.SplitHostPort(strings.TrimSpace(c.Server.Address)); err != nil {
		return errors.New("server.address must be host:port")
	}
	if strings.TrimSpace(c.Server.HealthPath) == "" {
		return errors.New("server.health_path must not be empty")
	}
	if !strings.HasPrefix(c.Server.HealthPath, "/") {
		return errors.New("server.health_path must start with /")
	}
	if c.Server.ReadTimeoutMs < 0 {
		return errors.New("server.read_timeout_ms must be >= 0")
	}
	if c.Server.WriteTimeoutMs < 0 {
		return errors.New("server.write_timeout_ms must be >= 0")
	}

	if strings.TrimSpace(c.Database.DSN) == "" {
		return errors.New("database.dsn must not be empty")
	}
	if c.Database.MaxOpenConns <= 0 {
		return errors.New("database.max_open_conns must be > 0")
	}
	if c.Database.MaxIdleConns < 0 {
		return errors.New("database.max_idle_conns must be >= 0")
	}
	if c.Database.ConnMaxLifetimeMs < 0 {
		return errors.New("database.conn_max_lifetime_ms must be >= 0")
	}
	if c.Database.ConnMaxIdleTimeMs < 0 {
		return errors.New("database.conn_max_idle_time_ms must be >= 0")
	}
	if c.Database.PingTimeoutMs <= 0 {
		return errors.New("database.ping_timeout_ms must be > 0")
	}

	if strings.TrimSpace(c.Security.AccessKeyCipherSecret) == "" {
		return errors.New("security.access_key_cipher_secret must not be empty")
	}

	if strings.TrimSpace(c.Auth.JWTSecret) == "" {
		return errors.New("auth.jwt_secret must not be empty")
	}
	if strings.TrimSpace(c.Auth.Issuer) == "" {
		return errors.New("auth.issuer must not be empty")
	}
	if c.Auth.AccessTokenTTLSeconds <= 0 {
		return errors.New("auth.access_token_ttl_seconds must be > 0")
	}
	if c.Auth.RefreshTokenTTLSeconds <= 0 {
		return errors.New("auth.refresh_token_ttl_seconds must be > 0")
	}

	return nil
}
