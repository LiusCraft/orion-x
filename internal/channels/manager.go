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

// KeyVerifier 校验一次 WebSocket 握手的凭证，只回事实、不做任何本地判定。
// 实现住在 internal/apikey/client（HTTP 调 manager 的 /internal/apikey/verify）。
type KeyVerifier interface {
	// Verify 返回校验结论；allowed=false 是业务拒绝（err == nil），err 只在
	// 传输/协议失败时非 nil。
	Verify(ctx context.Context, rawKey, deviceID string) (apikey.VerifyResponse, error)
}

// DeviceConfigLoader 从 manager 服务加载设备配置。
type DeviceConfigLoader interface {
	// LoadConfig 根据 device_id 获取 AppConfig。返回 nil 表示设备未注册。
	LoadConfig(deviceID string) (*config.AppConfig, error)

	// ListDeviceChannels 返回指定平台上所有已配置的设备及其原始配置（含敏感值）。
	// platform 取 internal/channels/platform 的平台标识。
	ListDeviceChannels(platform string) ([]DeviceChannelInfo, error)

	// ManagerURL 返回 manager 服务的 base URL。
	ManagerURL() string
}

// DeviceChannelInfo 是设备在一个通道平台上的配置，由 ListDeviceChannels 返回。
// Config 的键集合由平台声明（internal/channels/platform）定义，值为明文凭证。
type DeviceChannelInfo struct {
	DeviceID   string            `json:"device_id"`
	DeviceName string            `json:"device_name"`
	VoicebotID string            `json:"voicebot_id"`
	Config     map[string]string `json:"config"`
}

// Dependencies 是 Manager 注入给各 Channel 的共享依赖。
type Dependencies struct {
	// DeviceCfgLoader 用于按设备 ID 加载配置（含 LLM/ASR/TTS/MCP 等）。
	DeviceCfgLoader DeviceConfigLoader
	// Sessions and Tasks are process-wide so a task can be mounted from any
	// channel into any currently active session.
	Sessions  *session.Manager
	Tasks     *task.Registry
	Providers *provider.Pool
	// Billing 为每个设备会话创建一个计费句柄；nil = 计费关闭（所有调用 no-op）。
	// P1 只接 xiaozhi WS 通道：TG 那边的会话边界本来就模糊（一个聊天窗口可以活
	// 好几天），硬套“一次连接 = 一次会话”会引出一堆没人答得上来的问题（§15.5）。
	Billing BillingFactory
	// KeyVerifier 校验 WS 握手凭证；nil = auth.enabled: false（握手鉴权关闭，
	// 通道按现状放行；docs/wsserver-apikey-auth-design.md §1 D6）。
	KeyVerifier KeyVerifier
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
