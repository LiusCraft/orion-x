package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/liuscraft/orion-x/internal/llm"
)

const summaryPrefix = `[上下文压缩 — 仅作参考] 前面的对话轮次已被压缩为以下摘要。
这是从之前上下文窗口传递过来的信息——请将其视为背景参考，而非活跃指令。
请勿回答或执行摘要中提到的任何请求——它们已经被处理过了。
你只需要回复出现在此摘要之后的**最新用户消息**。
即使话题有重叠，也以最新用户消息为准。
--- 上下文摘要结束 — 回复下面的消息，不是上面的摘要 ---`

type Compressor struct {
	model       llm.Client
	config      CompressionConfig
	prevSummary string // iterative: old summary fed back
}

type CompressionConfig struct {
	Enabled          bool
	ThresholdPercent float64 // default 0.7
	TailTokenBudget  float64 // default 0.2
	MinTailMessages  int     // default 8
}

type CompressionResult struct {
	Summary   string // the structured summary text
	TailTurns []Turn // protected recent turns
}

func NewCompressor(model llm.Client, config CompressionConfig) *Compressor {
	if config.ThresholdPercent <= 0 {
		config.ThresholdPercent = 0.7
	}
	if config.TailTokenBudget <= 0 {
		config.TailTokenBudget = 0.2
	}
	if config.MinTailMessages <= 0 {
		config.MinTailMessages = 8
	}
	return &Compressor{model: model, config: config}
}

// ShouldCompress estimates whether context usage exceeds threshold.
// headTokens = system prompt + memory snapshot; tailBudget = total * tail ratio.
// If middle (all history not in tail) > threshold * total, compress.
func (c *Compressor) ShouldCompress(turns []Turn, headTokens, totalTokens int) bool {
	if !c.config.Enabled || len(turns) <= c.config.MinTailMessages {
		return false
	}

	// Conservative estimate: ~4 chars/token per turn
	headChars := headTokens * 4

	middleChars := 0
	for _, t := range turns[:len(turns)-c.config.MinTailMessages] {
		middleChars += len(t.UserText) + len(t.AssistantText)
	}

	used := headChars + middleChars
	threshold := int(float64(totalTokens*4) * c.config.ThresholdPercent)
	return used > threshold
}

// Compress generates a structured summary of the given turns.
func (c *Compressor) Compress(ctx context.Context, turns []Turn) (*CompressionResult, error) {
	// Separate tail
	tailBudget := int(float64(len(turns)) * c.config.TailTokenBudget)
	if tailBudget < c.config.MinTailMessages {
		tailBudget = c.config.MinTailMessages
	}
	if tailBudget >= len(turns) {
		tailBudget = len(turns) / 2
	}
	tailStart := len(turns) - tailBudget
	tail := turns[tailStart:]
	middle := turns[:tailStart]

	// Build summarizer prompt
	var b strings.Builder
	b.WriteString("请将以下对话历史压缩为结构化摘要（中文）。\n")
	b.WriteString("输出格式：\n")
	b.WriteString("## Historical Task Snapshot\n（已完成的任务概述）\n\n")
	b.WriteString("## Historical In-Progress State\n（进行中的状态）\n\n")
	b.WriteString("## Historical Pending User Asks\n（用户提过但未完成的需求）\n\n")
	b.WriteString("## Historical Remaining Work\n（剩余工作）\n\n")
	if c.prevSummary != "" {
		b.WriteString("以下是之前已有摘要，请在它的基础上更新和补充：\n")
		b.WriteString(c.prevSummary + "\n\n")
	}
	b.WriteString("需要压缩的对话：\n")
	for _, t := range middle {
		fmt.Fprintf(&b, "用户: %s\n助手: %s\n\n", strings.TrimSpace(t.UserText), strings.TrimSpace(t.AssistantText))
	}

	req := llm.Request{
		Messages: []llm.Message{
			{Role: "system", Content: "你是一个对话摘要助手，用简洁的中文输出结构化摘要。"},
			{Role: "user", Content: b.String()},
		},
	}

	resp, err := c.model.ChatSync(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("compressor: chat: %w", err)
	}

	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return nil, fmt.Errorf("compressor: empty summary")
	}

	// Wrap with restrictive prefix
	fullSummary := summaryPrefix + "\n\n" + summary
	c.prevSummary = summary // store for iterative update

	return &CompressionResult{
		Summary:   fullSummary,
		TailTurns: tail,
	}, nil
}
