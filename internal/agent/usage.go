package agent

import (
	"context"
	"sync"

	"github.com/liuscraft/orion-x/internal/llm"
)

// Usage 是一个 turn 内累计的 LLM 用量（归一口径：input 已剔除缓存读写，
// output 已剔除 reasoning，见 docs/billing-design.md §13）。
//
// 想拿“总共用了多少 token”的调用方必须自己把各字段加起来——字段之间互不重叠。
type Usage struct {
	InputTokens      int64 // 已剔除缓存读写
	OutputTokens     int64 // 已剔除 reasoning
	ReasoningTokens  int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	Steps            int64 // 本次 turn 实际发生的 LLM 调用次数
}

// IsZero 判断是否没有任何用量（含 step 数）。
func (u Usage) IsZero() bool {
	return u.InputTokens == 0 &&
		u.OutputTokens == 0 &&
		u.ReasoningTokens == 0 &&
		u.CacheReadTokens == 0 &&
		u.CacheWriteTokens == 0 &&
		u.Steps == 0
}

// UsageCollector 收集一个 turn 内所有 LLM 调用的用量。主 agent 与子代理共用
// 同一个 ctx，所以子代理的用量也会被算进来（runStep 是两者共用路径）。
//
// 零值可用；并发安全。
type UsageCollector struct {
	mu    sync.Mutex
	usage Usage
}

// NewUsageCollector 创建一个空 collector。
func NewUsageCollector() *UsageCollector { return &UsageCollector{} }

// Add 累加一次 LLM 调用的用量，并把 Steps 加一。nil 接收者安全（no-op）；
// 零值 llm.Usage 不改变 token 数，但仍然计一次调用。
func (c *UsageCollector) Add(u llm.Usage) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.usage.InputTokens += u.InputTokens
	c.usage.OutputTokens += u.OutputTokens
	c.usage.ReasoningTokens += u.ReasoningTokens
	c.usage.CacheReadTokens += u.CacheReadTokens
	c.usage.CacheWriteTokens += u.CacheWriteTokens
	c.usage.Steps++
}

// Take 取走并清零（turn 边界调用一次）。nil 接收者返回零值。
func (c *UsageCollector) Take() Usage {
	if c == nil {
		return Usage{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	usage := c.usage
	c.usage = Usage{}
	return usage
}

type usageCollectorCtxKey struct{}

// WithUsageCollector 把 collector 挂到 ctx 上。c 为 nil 或 ctx 为 nil 时原样返回。
func WithUsageCollector(ctx context.Context, c *UsageCollector) context.Context {
	if ctx == nil || c == nil {
		return ctx
	}
	return context.WithValue(ctx, usageCollectorCtxKey{}, c)
}

// UsageCollectorFromContext 取出 ctx 上的 collector，没有时返回 nil。
func UsageCollectorFromContext(ctx context.Context) *UsageCollector {
	if ctx == nil {
		return nil
	}
	c, _ := ctx.Value(usageCollectorCtxKey{}).(*UsageCollector)
	return c
}

// TakeUsage 取走 ctx 上 collector 累计的用量并清零，返回可直接塞进 FinishedEvent
// 的指针；没有 collector（计费未启用）或用量全为零时返回 nil。
func TakeUsage(ctx context.Context) *Usage {
	usage := UsageCollectorFromContext(ctx).Take()
	if usage.IsZero() {
		return nil
	}
	return &usage
}

// recordUsage 把一次 LLM 响应的用量累加进 ctx 上的 collector；没有 collector 时是 no-op。
func recordUsage(ctx context.Context, resp *llm.Response) {
	if resp == nil {
		return
	}
	UsageCollectorFromContext(ctx).Add(resp.Usage)
}
