package agent

import (
	"context"
)

// VoiceAgent 语音Agent，负责LLM流式调用、工具调用、情绪标注、Markdown过滤
type VoiceAgent interface {
	Process(ctx context.Context, text string) (<-chan AgentEvent, error)
	GetToolType(tool string) ToolType
}

// ToolType 工具类型
type ToolType int

const (
	ToolTypeQuery  ToolType = iota // 查询类：需要LLM总结
	ToolTypeAction                 // 动作类：直接执行+播报
)

func (t ToolType) String() string {
	switch t {
	case ToolTypeQuery:
		return "Query"
	case ToolTypeAction:
		return "Action"
	default:
		return "Unknown"
	}
}

// ToolInfo 工具信息
type ToolInfo struct {
	Name        string
	Description string
	Type        ToolType
}

// Config VoiceAgent配置
type Config struct {
	APIKey          string
	BaseURL         string
	Model           string
	ToolTypes       map[string]ToolType
	ActionResponses map[string]string
}
