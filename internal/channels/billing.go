package channels

import (
	"context"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/billing"
)

// 原始事实 → item_code 的映射就放在这一层：internal/audio 与 internal/agent 只吐
// 原始量（合成了多少 rune、厂商报了多少秒、各类 token 各是多少），计费的词汇不到
// 那些包里（docs/billing-design.md §6.2 / §13）。
//
// 归一化的减法（缓存命中、reasoning 从 input/output 里剔除）已经在 agent.Usage 里
// 做完了，这里只负责映射。

// BillingSessionMeta 是一个设备会话的计费标识。
type BillingSessionMeta struct {
	DeviceID   string
	SessionID  string
	Channel    string
	VoicebotID string
}

// BillingSession 是通道用的会话级计费句柄。实现住在 internal/billing/client
// （数据面客户端），这里只依赖 port 形状，所以计费关闭时传 nil 即可。
type BillingSession interface {
	// Authorize 做准入校验。控制面明确拒绝时返回 Allowed=false 且 err == nil。
	Authorize(ctx context.Context) (billing.AuthorizeResponse, error)
	// Start 启动兜底 flush。
	Start(ctx context.Context)
	// Record 累加一笔用量（非阻塞）。
	Record(itemCode string, quantity int64, dims map[string]any)
	// Flush 在 turn / 会话边界批量上报。
	Flush(ctx context.Context) error
	// SetTurn 设置后续 Record 归属的 turn 序号。
	SetTurn(index int64)
	// NearLimit 估算到了预冻结的 90%：不再接受新的 listen 窗口。
	NearLimit() bool
	// Exhausted 估算到了预冻结的 100%：中断会话。
	Exhausted() bool
	// Settle 交账（含 flush）。
	Settle(ctx context.Context, reason string) (billing.SettleResponse, error)
	// Close 释放本地资源。
	Close()
}

// BillingFactory 按设备会话创建计费句柄。返回 nil 表示这个会话不计费。
type BillingFactory func(ctx context.Context, meta BillingSessionMeta) BillingSession

// RecordLLMUsage 把一个 turn 累计的 LLM 用量映射成计费项。
//
// quantity 口径（§13）：任意两个计费项不能重叠，所以 input 已剔除缓存读写、
// output 已剔除 reasoning（减法在 agent.Usage 里完成）。某个量是 0 就不发事件，
// 免得给事件表塞一堆 0。
func RecordLLMUsage(sess BillingSession, usage agent.Usage, base map[string]any) {
	if sess == nil || usage.IsZero() {
		return
	}
	dims := withDims(base, "step", usage.Steps)
	if usage.InputTokens > 0 {
		sess.Record(billing.ItemLLMInput, usage.InputTokens, dims)
	}
	if usage.CacheReadTokens > 0 {
		sess.Record(billing.ItemLLMCacheRead, usage.CacheReadTokens, dims)
	}
	if usage.CacheWriteTokens > 0 {
		sess.Record(billing.ItemLLMCacheWrite, usage.CacheWriteTokens, dims)
	}
	if usage.OutputTokens > 0 {
		sess.Record(billing.ItemLLMOutput, usage.OutputTokens, dims)
	}
	if usage.ReasoningTokens > 0 {
		sess.Record(billing.ItemLLMReasoning, usage.ReasoningTokens, dims)
	}
}

// RecordTTSSynthesis 记一句已经发给厂商合成的文本（单位：rune）。分句已定稿的
// 那一句就算数：用户打断的是播放，厂商那边已经合成并计费了（§13）。
func RecordTTSSynthesis(sess BillingSession, runes int64, dims map[string]any) {
	if sess == nil || runes <= 0 {
		return
	}
	sess.Record(billing.ItemTTSCharacters, runes, dims)
}

// RecordASRSeconds 记一段厂商返回的识别时长（单位：秒）。
func RecordASRSeconds(sess BillingSession, seconds int64, dims map[string]any) {
	if sess == nil || seconds <= 0 {
		return
	}
	sess.Record(billing.ItemASRAudioSecond, seconds, dims)
}

// withDims 复制一份维度并加一个 key；base 为 nil 时也安全。
func withDims(base map[string]any, key string, value any) map[string]any {
	out := make(map[string]any, len(base)+1)
	for k, v := range base {
		out[k] = v
	}
	out[key] = value
	return out
}
