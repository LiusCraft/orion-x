package channels

import (
	"context"
	"sync"

	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/logging"
)

// DeviceConfigLoader 从 manager 服务加载设备配置。
type DeviceConfigLoader interface {
	// LoadConfig 根据 device_id 获取 AppConfig。返回 nil 表示设备未注册。
	LoadConfig(deviceID string) (*config.AppConfig, error)

	// ListDevicesWithTGBot 返回所有配置了 tg_bot_token 的设备列表。
	ListDevicesWithTGBot() ([]DeviceTGBotInfo, error)

	// ManagerURL 返回 manager 服务的 base URL。
	ManagerURL() string
}

// DeviceTGBotInfo 是设备的 TG Bot 信息，由 ListDevicesWithTGBot 返回。
type DeviceTGBotInfo struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	TgBotToken string `json:"tg_bot_token"`
	VoicebotID string `json:"voicebot_id"`
}

// Dependencies 是 Manager 注入给各 Channel 的共享依赖。
type Dependencies struct {
	// DeviceCfgLoader 用于按设备 ID 加载配置（含 LLM/ASR/TTS/MCP 等）。
	DeviceCfgLoader DeviceConfigLoader
}

// Manager 管理多个 Channel 的生命周期。进程级单例。
type Manager struct {
	deps     *Dependencies
	channels map[string]Channel

	rootCtx    context.Context
	rootCancel context.CancelFunc
}

// NewManager 创建 Manager。
func NewManager(deps *Dependencies) *Manager {
	return &Manager{
		deps:     deps,
		channels: make(map[string]Channel),
	}
}

// Register 注册一个 Channel。已存在同名 Channel 时会 panic。
func (m *Manager) Register(c Channel) {
	name := c.Name()
	if _, exists := m.channels[name]; exists {
		logging.Fatalf("channel %q already registered", name)
	}
	m.channels[name] = c
	logging.Infof("channel: registered %q (%s)", name, c.Info().DisplayName)
}

// Start 启动所有已注册的 Channel。按注册顺序依次启动。
func (m *Manager) Start(ctx context.Context) error {
	m.rootCtx, m.rootCancel = context.WithCancel(ctx)

	for name, c := range m.channels {
		info := c.Info()
		logging.Infof("channel: starting %q (%s, %s)", name, info.DisplayName, info.Type)
		if err := c.Start(m.rootCtx); err != nil {
			return err
		}
	}
	return nil
}

// Stop 优雅停止所有 Channel。先并行触发 Stop，再等待所有 goroutine 退出。
func (m *Manager) Stop() {
	if m.rootCancel != nil {
		m.rootCancel()
	}

	var wg sync.WaitGroup
	for name, c := range m.channels {
		wg.Add(1)
		go func(name string, c Channel) {
			defer wg.Done()
			logging.Infof("channel: stopping %q", name)
			if err := c.Stop(m.rootCtx); err != nil {
				logging.Warnf("channel: stop %q error: %v", name, err)
			}
		}(name, c)
	}
	wg.Wait()
}
