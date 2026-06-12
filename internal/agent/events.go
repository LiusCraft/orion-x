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
type FinishedEvent struct {
	Error error
}

func (e *FinishedEvent) Type() AgentEventType {
	return AgentEventTypeFinished
}
