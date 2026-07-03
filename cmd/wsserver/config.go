package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultWsserverConfigPath = "data/wsserver.yaml"

type WsserverConfig struct {
	Server  WsServerConfig  `yaml:"server"`
	Manager WsManagerConfig `yaml:"manager"`
	Logging WsLoggingConfig `yaml:"logging"`
}

type WsServerConfig struct {
	Addr   string `yaml:"addr"`
	WsPath string `yaml:"ws_path"`
}

type WsManagerConfig struct {
	URL string `yaml:"url"`
}

type WsLoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func defaultWsserverConfig() *WsserverConfig {
	return &WsserverConfig{
		Server:  WsServerConfig{Addr: ":8080", WsPath: "/ws"},
		Manager: WsManagerConfig{},
		Logging: WsLoggingConfig{Level: "info", Format: "console"},
	}
}

func loadWsserverConfig(path string) (*WsserverConfig, error) {
	cfg := defaultWsserverConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			applyWsserverEnv(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	applyWsserverEnv(cfg)
	return cfg, nil
}

func applyWsserverEnv(cfg *WsserverConfig) {
	if v := strings.TrimSpace(os.Getenv("MANAGER_URL")); v != "" {
		cfg.Manager.URL = v
	}
	if v := strings.TrimSpace(os.Getenv("LOG_LEVEL")); v != "" {
		cfg.Logging.Level = v
	}
}
