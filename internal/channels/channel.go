// Package channels defines the platform channel framework — an abstraction
// that lets Orion-X agents communicate over different platforms
// (WebSocket voice sessions, Telegram, Discord, etc.) through a uniform
// lifecycle and resource-sharing interface.
package channels

import "context"

// ChannelType 通道连接方式类型。
type ChannelType string

const (
	// ChannelServer 监听端口，被动接受连接（如 Xiaozhi WS）。
	ChannelServer ChannelType = "server"
	// ChannelPolling 主动轮询/长连接拉取消息（如 TG Bot）。
	ChannelPolling ChannelType = "polling"
	// ChannelClient 以客户端身份主动连接到第三方平台（预留）。
	ChannelClient ChannelType = "client"
)

// Capability 是通道支持的能力标识。
type Capability string

const (
	CapText        Capability = "text"         // 文本消息
	CapVoiceFile   Capability = "voice_file"   // 语音文件（离线 ASR）
	CapAudioStream Capability = "audio_stream" // 实时音频流
)

// Channel 是平台通道的通用接口。每个平台（TG/Discord/Xiaozhi WS 等）
// 实现一个 Channel，Manager 统一管理其生命周期。
type Channel interface {
	// Name 返回全局唯一的通道标识，如 "tg" / "discord" / "xiaozhi"。
	Name() string

	// Start 启动通道，建立与平台的连接/监听。
	Start(ctx context.Context) error

	// Stop 断开平台连接，释放所有资源。
	Stop(ctx context.Context) error

	// Info 返回通道元信息。
	Info() ChannelInfo
}

// ChannelInfo 通道元信息。
type ChannelInfo struct {
	Name         string         // 标识符
	DisplayName  string         // 展示名
	Type         ChannelType    // 连接方式
	Capabilities []Capability   // 能力列表
}

// NewChannelInfo 构建通道元信息。
func NewChannelInfo(name, displayName string, ctype ChannelType, caps []Capability) ChannelInfo {
	if caps == nil {
		caps = []Capability{}
	}
	return ChannelInfo{
		Name:         name,
		DisplayName:  displayName,
		Type:         ctype,
		Capabilities: caps,
	}
}
