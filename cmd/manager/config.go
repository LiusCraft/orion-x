package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type ManagerConfig struct {
	Server      ServerConfig      `yaml:"server"`
	Database    DatabaseConfig    `yaml:"database"`
	JWT         JWTConfig         `yaml:"jwt"`
	Admin       AdminConfig       `yaml:"admin"`
	GithubOAuth GithubOAuthConfig `yaml:"github_oauth"`
	Logging     LoggingConfig     `yaml:"logging"`
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
}
