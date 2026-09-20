package apikey

// 数据面（wsserver）与控制面（manager）之间的接入鉴权契约。
//
// 拆分与计费一致：wire 类型与判定逻辑留在零依赖的领域层，HTTP 客户端住在
// internal/apikey/client，控制面实现住在 cmd/manager/handler。握手里带上
// 明文密钥的是客户端，所以数据面不解析密钥内容——它只是把事实转给控制面。

// PathAuthorize 是数据面校验接入凭据的内部接口路径（走内部 token 鉴权）。
const PathAuthorize = "/internal/apikey/authorize"

// RejectReason 是 authorize 拒绝一次接入的原因（机器可读）。它同时作为
// WebSocket Close 帧的 reason 交给客户端，所以要短且稳定。
type RejectReason string

const (
	// RejectNone 表示放行。
	RejectNone RejectReason = ""
	// RejectKeyNotFound 密钥不存在（含已被删除）。
	RejectKeyNotFound RejectReason = "key:not_found"
	// RejectScopeMissing 密钥合法但没有 ScopeVoice。
	RejectScopeMissing RejectReason = "key:scope_missing"
	// RejectKeyMissing 是数据面的本地判定：要求必须带密钥，而请求里没有。
	RejectKeyMissing RejectReason = "key:missing"
	// RejectKeyUnavailable 是数据面的本地判定：验证不了（控制面不可达 / 没配内部
	// token / 数据面没注入校验器）。取不到结论时选择拒绝，而不是放行。
	RejectKeyUnavailable RejectReason = "key:unavailable"
	// RejectDeviceNotKnown 设备没有注册（或没绑到任何智能体）。
	RejectDeviceNotKnown RejectReason = "device:not_found"
	// RejectOwnerMismatch 设备属于别人：密钥只能接入自己名下的设备。
	RejectOwnerMismatch RejectReason = "device:owner_mismatch"
)

// AuthorizeRequest 是接入校验请求。数据面只带事实：设备是谁、凭据是什么，
// 其余（密钥属主、设备属主、权限范围）都由控制面查。
type AuthorizeRequest struct {
	Key      string `json:"key"`
	DeviceID string `json:"device_id"`
}

// AuthorizeResponse 是校验结果。被拒也是 HTTP 200 + allowed=false（与计费准入
// 一致）：拒绝是正常业务结果，不该混进“请求失败”的错误通道。
type AuthorizeResponse struct {
	Allowed      bool         `json:"allowed"`
	OwnerID      string       `json:"owner_id,omitempty"`
	KeyID        string       `json:"key_id,omitempty"`
	KeyName      string       `json:"key_name,omitempty"`
	RejectReason RejectReason `json:"reject_reason,omitempty"`
}

// Facts 是控制面查库之后交给判定函数的全部事实。
type Facts struct {
	KeyFound      bool
	KeyOwnerID    string
	KeyScopes     []Scope
	DeviceFound   bool
	DeviceOwnerID string
}

// Decide 判定一次语音接入。顺序是有意的：先确认密钥本身（不存在 → scope 不符），
// 再看设备，最后比属主——每步的拒绝原因都不同，排查时一眼能看出卡在哪。
func Decide(f Facts) RejectReason {
	switch {
	case !f.KeyFound:
		return RejectKeyNotFound
	case !AllowsScope(f.KeyScopes, ScopeVoice):
		return RejectScopeMissing
	case !f.DeviceFound:
		return RejectDeviceNotKnown
	case f.DeviceOwnerID != f.KeyOwnerID:
		return RejectOwnerMismatch
	default:
		return RejectNone
	}
}
