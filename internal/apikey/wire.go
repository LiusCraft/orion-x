package apikey

// RejectReason 是握手校验（verify）的业务拒绝原因（机器可读）。下划线风格与
// billing.RejectReason 对齐；取值只增不改，未知值由数据面原样透传（
// docs/wsserver-apikey-auth-design.md §3.1）。
type RejectReason string

const (
	RejectInvalidKey        RejectReason = "invalid_key"
	RejectKeyRevoked        RejectReason = "key_revoked"
	RejectKeyExpired        RejectReason = "key_expired"
	RejectInsufficientScope RejectReason = "insufficient_scope"
	RejectDeviceNotOwned    RejectReason = "device_not_owned"
	RejectRateLimited       RejectReason = "rate_limited"
)

// PathVerify 是数据面（wsserver）校验 WS 握手凭证的内部端点。路由挂在 manager 的
// /internal 组下并挂 InternalAuth，见 cmd/manager/server.go。
const PathVerify = "/internal/apikey/verify"

// VerifyRequest 是握手校验请求：key 明文 + 本次会话实际使用的 device_id。
// device_id 取自 hello 而不是 HTTP header，避免两者不一致绕过归属校验（§1 D3）。
type VerifyRequest struct {
	Key      string `json:"key"`
	DeviceID string `json:"device_id"`
}

// VerifyResponse 是校验结论。被拒时同样返回 HTTP 200，只是 allowed=false；
// reject_reason 是业务判断不是传输错误（对齐 billing.AuthorizeResponse 的约定）。
// allowed 恒在；insufficient_scope 额外带 required/granted 便于设备侧排障。
type VerifyResponse struct {
	Allowed      bool         `json:"allowed"`
	KeyID        string       `json:"key_id,omitempty"`
	UserID       string       `json:"user_id,omitempty"`
	Scopes       []string     `json:"scopes,omitempty"`
	RejectReason RejectReason `json:"reject_reason,omitempty"`
	Required     []string     `json:"required,omitempty"`
	Granted      []string     `json:"granted,omitempty"`
}
