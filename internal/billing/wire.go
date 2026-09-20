package billing

import "time"

// RejectReason 是 authorize 拒绝会话的原因（机器可读）。
type RejectReason string

const (
	RejectInsufficientBalance RejectReason = "insufficient_balance"
	RejectAccountSuspended    RejectReason = "account_suspended"
	RejectNoAccount           RejectReason = "no_account"
	RejectPriceMissing        RejectReason = "price_missing"
	// RejectSessionClosed 表示这个 session_id 的预冻结已经结算或回收过了。正常链路
	// 不会碰到：session_id 由数据面每个连接新生成，重复利用属于协议违规。
	RejectSessionClosed RejectReason = "session_closed"
)

// 数据面访问控制面的三个内部接口路径。
const (
	PathAuthorize   = "/internal/billing/authorize"
	PathUsageEvents = "/internal/billing/usage-events"
	PathSettle      = "/internal/billing/settle"
)

// 单批上报上限。超限整批拒绝——半批成功会让重试语义变复杂。
const (
	MaxUsageEventsPerBatch = 500
	MaxUsageBatchBytes     = 256 << 10
)

// ClockSkewTolerance 是 occurred_at 与 received_at 之间可接受的偏差，超过就
// 记维度并告警（§14.4）。
const ClockSkewTolerance = 5 * time.Minute

// AuthorizeRequest 是会话建立时的准入请求。数据面只带事实，不带账户。
type AuthorizeRequest struct {
	DeviceID  string `json:"device_id"`
	SessionID string `json:"session_id"`
	Channel   string `json:"channel,omitempty"`
}

// AuthorizeResponse 是准入决策 + 本地熔断需要的参数。被拒时同样返回 HTTP 200，
// 只是 allowed=false。
type AuthorizeResponse struct {
	Allowed           bool            `json:"allowed"`
	AccountID         string          `json:"account_id,omitempty"`
	ReservationID     string          `json:"reservation_id,omitempty"`
	ReservedMicro     int64           `json:"reserved_micro,omitempty"`
	MaxSessionSeconds int             `json:"max_session_seconds,omitempty"`
	BalanceMicro      int64           `json:"balance_micro,omitempty"`
	RequiredMicro     int64           `json:"required_micro,omitempty"`
	ExpiresAt         *time.Time      `json:"expires_at,omitempty"`
	RejectReason      RejectReason    `json:"reject_reason,omitempty"`
	PriceSnapshot     []PriceSnapshot `json:"price_snapshot,omitempty"`
}

// SnapshotMap 把快照数组转成按 item_code 索引的映射，方便数据面估算。
func (r AuthorizeResponse) SnapshotMap() map[string]PriceSnapshot {
	out := make(map[string]PriceSnapshot, len(r.PriceSnapshot))
	for _, s := range r.PriceSnapshot {
		out[s.ItemCode] = s
	}
	return out
}

// UsageEventReport 是一条用量事实。event_id 由数据面生成，服务端主键 →
// 重试天然去重。
type UsageEventReport struct {
	EventID    string         `json:"event_id"`
	ItemCode   string         `json:"item_code"`
	Quantity   int64          `json:"quantity"`
	TurnIndex  int64          `json:"turn_index,omitempty"`
	OccurredAt time.Time      `json:"occurred_at"`
	ModelID    string         `json:"model_id,omitempty"`
	ProviderID string         `json:"provider_id,omitempty"`
	VoiceID    string         `json:"voice_id,omitempty"`
	Dimensions map[string]any `json:"dimensions,omitempty"`
}

// UsageBatchRequest 是批量上报体。每条事件都必须带 session_id；device_id 只在
// 控制面查不到 reservation 时用来补一条 degraded 行（§14.2）。
type UsageBatchRequest struct {
	SessionID string             `json:"session_id"`
	DeviceID  string             `json:"device_id,omitempty"`
	Events    []UsageEventReport `json:"events"`
}

// RejectedEvent 是逐条拒绝的原因。
type RejectedEvent struct {
	EventID string `json:"event_id"`
	Reason  string `json:"reason"`
}

// UsageBatchResponse 逐条给出结果。duplicated 是正常现象（重试撞上了）。
type UsageBatchResponse struct {
	Accepted   int             `json:"accepted"`
	Duplicated int             `json:"duplicated"`
	Rejected   []RejectedEvent `json:"rejected,omitempty"`
}

// SettleRequest 是会话结束时交出账目的请求。
type SettleRequest struct {
	SessionID string    `json:"session_id"`
	Reason    string    `json:"reason,omitempty"`
	EndedAt   time.Time `json:"ended_at"`
}

// SettleResponse 返回这次会话最终的收支。
type SettleResponse struct {
	ChargedMicro  int64 `json:"charged_micro"`
	ReleasedMicro int64 `json:"released_micro"`
	BalanceMicro  int64 `json:"balance_micro"`
	Settled       bool  `json:"settled"`
}
