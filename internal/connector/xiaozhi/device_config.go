package xiaozhi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/connector"
)

// DeviceConfigLoader resolves a per-device AppConfig from the manager service.
// Returns nil, nil when the device has no dedicated config.
type DeviceConfigLoader interface {
	LoadConfig(deviceID string) (*config.AppConfig, error)
	ManagerURL() string
}

// HTTPDeviceConfigLoader fetches device config from the manager service via
// its internal HTTP API. No auth required — the endpoint is intended for
// internal network use only.
type HTTPDeviceConfigLoader struct {
	managerURL string
	client     *http.Client
}

// NewHTTPDeviceConfigLoader creates a new HTTPDeviceConfigLoader.
func NewHTTPDeviceConfigLoader(managerURL string) *HTTPDeviceConfigLoader {
	return &HTTPDeviceConfigLoader{
		managerURL: managerURL,
		client:     &http.Client{Timeout: 3 * time.Second},
	}
}

// ManagerURL returns the base URL of the manager service.
func (l *HTTPDeviceConfigLoader) ManagerURL() string {
	return l.managerURL
}

// LoadConfig fetches the device configuration from the manager service.
func (l *HTTPDeviceConfigLoader) LoadConfig(deviceID string) (*config.AppConfig, error) {
	u := l.managerURL + "/internal/device-config?device_id=" + url.QueryEscape(deviceID)
	resp, err := l.client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("device config loader: GET %s: %w", u, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // device not registered
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

// ListDevicesWithTGBot returns all devices that have a tg_bot_token set.
func (l *HTTPDeviceConfigLoader) ListDevicesWithTGBot() ([]connector.DeviceTGBotInfo, error) {
	u := l.managerURL + "/internal/devices/tg-bots"
	resp, err := l.client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("list tg bots: GET %s: %w", u, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list tg bots: unexpected status %d", resp.StatusCode)
	}

	var list []connector.DeviceTGBotInfo
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("list tg bots: decode: %w", err)
	}
	return list, nil
}
