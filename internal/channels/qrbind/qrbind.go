// Package qrbind 是「扫码开通」在配置面与平台实现之间的契约：manager 的通道
// handler 消费 Binder，各平台提供自己的实现（目前只有企业微信，见
// internal/channels/wecom/qrlogin）。会话是短时状态，内存持有、不落库。
package qrbind

import (
	"errors"
	"time"
)

// Status 是一次扫码会话的状态。
type Status string

const (
	// StatusPending 已生成二维码，等待扫码。
	StatusPending Status = "pending"
	// StatusScanned 已扫码，等待用户在同一台手机上确认。
	StatusScanned Status = "scanned"
	// StatusSuccess 已拿到平台凭证；Config 此时非空。
	StatusSuccess Status = "success"
	// StatusExpired 超过会话有效期仍未完成。
	StatusExpired Status = "expired"
	// StatusFailed 平台侧或本地判定的失败；Error 给出原因。
	StatusFailed Status = "failed"
)

// Session 是一次扫码会话的快照。
type Session struct {
	ID     string
	Status Status
	// QRContent 是二维码内容（企业微信的 auth_url），仅 pending / scanned 有值。
	QRContent string
	ExpiresAt time.Time
	// Error 是 failed 时的用户可读原因（平台 errmsg 或本地判断）。
	Error string
	// Config 是 success 后待写入的平台配置字段；写库前必须过平台 Schema 校验。
	Config map[string]string
}

// ErrSessionNotFound 表示会话不存在：已过期清理、已取消，或被新的会话替换。
var ErrSessionNotFound = errors.New("qr session not found")

// Binder 是各平台的扫码开通实现。
type Binder interface {
	// Start 生成二维码并启动后台轮询；同一 deviceID 重复调用会替换旧会话。
	// 返回错误表示现在拿不到二维码，不产生会话。
	Start(deviceID string) (Session, error)

	// Get 返回会话快照；会话不存在时返回 ErrSessionNotFound。
	Get(deviceID, sessionID string) (Session, error)

	// Cancel 结束并清理会话；幂等，未知会话返回 nil。
	Cancel(deviceID, sessionID string)
}
