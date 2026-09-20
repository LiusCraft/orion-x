package provider

import "github.com/liuscraft/orion-x/internal/llm"

// UsageParts 是厂商在响应里报的原始 token 明细（可能带包含关系）。
type UsageParts struct {
	InputTokens      int64 // prompt / input，可能已含缓存命中
	OutputTokens     int64 // completion / output，可能已含 reasoning
	TotalTokens      int64 // 厂商报的总量，原样带到 llm.Usage
	CacheReadTokens  int64 // prompt_tokens_details.cached_tokens
	CacheWriteTokens int64 // 仅 Anthropic 有；OpenAI 恒 0
	ReasoningTokens  int64 // completion_tokens_details.reasoning_tokens
}

// NormalizeUsage 把厂商报的明细归一成 llm.Usage 的互斥口径（docs/billing-design.md §13）：
// InputTokens 剔除缓存读写，OutputTokens 剔除 reasoning。所有减法在 0 处夹紧——
// 有些兼容厂商会给出“子集比父集还大”的脏数据，负数会直接算成负账。
func NormalizeUsage(parts UsageParts) llm.Usage {
	return llm.Usage{
		InputTokens:      clampSub(parts.InputTokens, parts.CacheReadTokens, parts.CacheWriteTokens),
		OutputTokens:     clampSub(parts.OutputTokens, parts.ReasoningTokens),
		ReasoningTokens:  parts.ReasoningTokens,
		CacheReadTokens:  parts.CacheReadTokens,
		CacheWriteTokens: parts.CacheWriteTokens,
		TotalTokens:      parts.TotalTokens,
	}
}

func clampSub(total int64, subs ...int64) int64 {
	for _, sub := range subs {
		total -= sub
	}
	if total < 0 {
		return 0
	}
	return total
}
