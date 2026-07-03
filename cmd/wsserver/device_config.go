package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/liuscraft/orion-x/internal/config"
)

// DeviceConfigLoader resolves a per-device AppConfig from an external store.
// Returns nil, nil when the device has no dedicated config (caller falls back
// to the global appConfig).
type DeviceConfigLoader interface {
	LoadConfig(deviceID string) (*config.AppConfig, error)
}

// httpDeviceConfigLoader fetches device config from the manager service via
// its internal HTTP API. No auth is required — the endpoint is intended for
// internal network use only.
type httpDeviceConfigLoader struct {
	managerURL string
	client     *http.Client
}

func newHTTPDeviceConfigLoader(managerURL string) *httpDeviceConfigLoader {
	return &httpDeviceConfigLoader{
		managerURL: managerURL,
		client:     &http.Client{Timeout: 3 * time.Second},
	}
}

func (l *httpDeviceConfigLoader) LoadConfig(deviceID string) (*config.AppConfig, error) {
	u := l.managerURL + "/internal/device-config?device_id=" + url.QueryEscape(deviceID)
	resp, err := l.client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("device config loader: GET %s: %w", u, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // device not registered → caller uses fallback
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device config loader: unexpected status %d", resp.StatusCode)
	}

	cfg := config.DefaultConfig()
	if err := json.NewDecoder(resp.Body).Decode(cfg); err != nil {
		return nil, fmt.Errorf("device config loader: decode config: %w", err)
	}
	cfg.NormalizeProviders()
	cfg.ApplyEnv()
	return cfg, nil
}
