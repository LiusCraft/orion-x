// Package connector defines the platform connector framework — an abstraction
// that lets Orion-X agents communicate over different platforms
// (WebSocket voice sessions, Telegram, Discord, etc.) through a uniform
// lifecycle and resource-sharing interface.
package connector

import "context"

// ConnectorType 连接器连接方式类型。
type ConnectorType string

const (
	// ConnectorServer 监听端口，被动接受连接（如 Xiaozhi WS）。
	ConnectorServer ConnectorType = "server"
	// ConnectorPolling 主动轮询/长连接拉取消息（如 TG Bot）。
	ConnectorPolling ConnectorType = "polling"
	// ConnectorClient 以客户端身份主动连接到第三方平台（预留）。
	ConnectorClient ConnectorType = "client"
)

// Capability 是连接器支持的能力标识。
type Capability string

const (
	CapText        Capability = "text"         // 文本消息
	CapVoiceFile   Capability = "voice_file"   // 语音文件（离线 ASR）
	CapAudioStream Capability = "audio_stream" // 实时音频流
)

// Connector 是平台连接器的通用接口。每个平台（TG/Discord/Xiaozhi WS 等）
// 实现一个 Connector，ConnectorManager 统一管理其生命周期。
type Connector interface {
	// Name 返回全局唯一的连接器标识，如 "tg" / "discord" / "xiaozhi"。
	Name() string

	// Start 启动连接器，建立与平台的连接/监听。
	Start(ctx context.Context) error

	// Stop 断开平台连接，释放所有资源。
	Stop(ctx context.Context) error

	// Info 返回连接器元信息。
	Info() ConnectorInfo
}

// ConnectorInfo 连接器元信息。
type ConnectorInfo struct {
	Name         string         // 标识符
	DisplayName  string         // 展示名
	Type         ConnectorType  // 连接方式
	Capabilities []Capability   // 能力列表
}

// NewConnectorInfo 构建连接器元信息。
func NewConnectorInfo(name, displayName string, ctype ConnectorType, caps []Capability) ConnectorInfo {
	if caps == nil {
		caps = []Capability{}
	}
	return ConnectorInfo{
		Name:         name,
		DisplayName:  displayName,
		Type:         ctype,
		Capabilities: caps,
	}
}
