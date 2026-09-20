package channels

import (
	"context"
	"sync"

	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/provider"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/task"
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
	// APIKeyAuth 校验客户端带来的访问密钥（xiaozhi 握手的接入鉴权）。
	// nil = 无法校验：带了密钥的连接会被拒，不带密钥的照旧（见 AuthConfig）。
	APIKeyAuth APIKeyAuthorizer
	// Sessions and Tasks are process-wide so a task can be mounted from any
	// channel into any currently active session.
	Sessions  *session.Manager
	Tasks     *task.Registry
	Providers *provider.Pool
	// Billing 为每个设备会话创建一个计费句柄；nil = 计费关闭（所有调用 no-op）。
	// P1 只接 xiaozhi WS 通道：TG 那边的会话边界本来就模糊（一个聊天窗口可以活
	// 好几天），硬套“一次连接 = 一次会话”会引出一堆没人答得上来的问题（§15.5）。
	Billing BillingFactory
}

// APIKeyAuthorizer 把一次接入请求交给控制面判定。响应里的 Allowed=false 是正常
// 结果（含机器可读的 RejectReason）；error 只表示“没走通”（控制面不可达、token
// 不对、响应不合法），调用方必须按“验证不了就不放行”处理。
type APIKeyAuthorizer interface {
	Authorize(ctx context.Context, key, deviceID string) (apikey.AuthorizeResponse, error)
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
