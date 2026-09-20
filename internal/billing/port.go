package billing

import (
	"context"
	"time"
)

// Sink 是数据面记录用量的 port。实现它的是数据面的接线层
// （internal/billing/client），不是 audio / agent 本身。
type Sink interface {
	// Record 非阻塞：写入会话内存缓冲，按 unit 累加。它跑在音频和 LLM 的
	// 热路径上，任何网络调用或阻塞写入都会变成用户听到的卡顿。
	Record(itemCode string, quantity int64, dims map[string]any)
	// Flush 在 turn / 会话边界批量上报（含补报重试）。
	Flush(ctx context.Context) error
}

// NopSink 是计费关闭时的 Sink。所有调用 no-op。
type NopSink struct{}

// Record 实现 Sink。
func (NopSink) Record(string, int64, map[string]any) {}

// Flush 实现 Sink。
func (NopSink) Flush(context.Context) error { return nil }

// Meter 是控制面业务模块唯一需要认识的计费 port：业务模块说“我干了什么、多大
// 量”，不说钱。金额、有没有免费额度、要不要按账期阶梯算，全是计费的活。
//
// 同一个 port 不给数据面用：数据面是异步、可合并、可丢（少收方向），控制面这
// 条是同步、不能丢（业务动作已经发生了）。
type Meter interface {
	// Charge 用于动作已经完成、立即扣费的场景（MCP 调用、按次收费的入库）。
	Charge(ctx context.Context, req ChargeRequest) (ChargeResult, error)
	// Reserve 用于动作还没做的场景：先占住钱，做完再 Settle，失败就 Release。
	Reserve(ctx context.Context, req ReserveRequest) (Reservation, error)
	// Release 释放预冻结（业务动作失败）。
	Release(ctx context.Context, reservationID string) error
}

// ChargeRequest 是一次控制面扣费请求。
type ChargeRequest struct {
	SubjectType string // user | org
	SubjectID   string
	ItemCode    string // 用 billing 导出的常量，别写字面量
	Quantity    int64
	RefType     string // 外部引用类型，如 voice:clone / mcp:call
	RefID       string // 外部实体 ID；幂等键由 RefType + RefID 拼
	Dims        map[string]any
}

// ReserveRequest 是一次控制面预冻结请求，字段与 ChargeRequest 相同。
type ReserveRequest struct {
	SubjectType string
	SubjectID   string
	ItemCode    string
	Quantity    int64
	RefType     string
	RefID       string
	Dims        map[string]any
}

// Reservation 是一次预冻结的结果。
type Reservation struct {
	ID          string
	AccountID   string
	AmountMicro int64
	ExpiresAt   time.Time
}

// ChargeIdempotencyKey 由外部引用拼出幂等键，不由调用方随手生成。这样重试、
// 重放都安全，也让“这笔账对应哪次业务操作”永远查得到。
func ChargeIdempotencyKey(refType, refID string) string {
	return "charge:" + refType + ":" + refID
}

// RechargeIdempotencyKey 是一笔充值入账的幂等键。
//
// 它必须与 Service.Credit 内部拼的键（"credit:" + RefType + ":" + RefID，RefType
// 取 billing.RefOrder）逐字一致：充值走的是 Credit，重复的支付回调、对账补单、
// worker 重试都靠这个键变成一次入账。支付模块用它在上报前先查账（LedgerExists），
// 免得把“已经入过账”当成失败来重试。
func RechargeIdempotencyKey(outTradeNo string) string {
	return "credit:" + RefOrder + ":" + outTradeNo
}

// RefundIdempotencyKey 是一笔退款出账的幂等键。
//
// 与 Service.Refund 内部拼的键（"refund:" + RefType + ":" + RefID）逐字一致。
// 一笔订单只支持**全额**退款，所以仅用订单号就能唯一确定这笔出账；真要支持部分
// 退款，键里得再加退款单号，也就需要一张单独的退款单表了。
func RefundIdempotencyKey(outTradeNo string) string {
	return "refund:" + RefOrder + ":" + outTradeNo
}

// SettleIdempotencyKey 是会话结算的幂等键。
func SettleIdempotencyKey(sessionID string) string {
	return "settle:" + sessionID
}

// EventIdempotencyKey 是一条用量事件入账的幂等键。
func EventIdempotencyKey(eventID string) string {
	return "settle:" + eventID
}

// ReserveIdempotencyKey 是预冻结的幂等键（按外部引用）。
func ReserveIdempotencyKey(refType, refID string) string {
	return "reserve:" + refType + ":" + refID
}
