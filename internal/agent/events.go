package agent

// AgentEvent Agent事件
type AgentEvent interface {
	Type() AgentEventType
}

// AgentEventType Agent事件类型
type AgentEventType int

const (
	AgentEventTypeTextChunk AgentEventType = iota // 文本块
	AgentEventTypeFinished                        // 完成
)

// TextChunkEvent 文本块事件
type TextChunkEvent struct {
	Chunk string
}

func (e *TextChunkEvent) Type() AgentEventType {
	return AgentEventTypeTextChunk
}

// FinishedEvent 完成事件
//
// Usage 是本 turn 累计的 LLM 用量，供接线层换算成计费量；计费未启用时为 nil。
// 错误路径也会带上——中断的 turn 已经产生了 token（docs/billing-design.md §13）。
type FinishedEvent struct {
	Error error
	Usage *Usage
}

func (e *FinishedEvent) Type() AgentEventType {
	return AgentEventTypeFinished
}
