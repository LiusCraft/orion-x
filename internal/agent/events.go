package agent

// AgentEvent Agent事件
type AgentEvent interface {
	Type() AgentEventType
}

// AgentEventType Agent事件类型
type AgentEventType int

const (
	AgentEventTypeTextChunk         AgentEventType = iota // 文本块
	AgentEventTypeEmotionChanged                          // 情绪变化
	AgentEventTypeToolCallRequested                       // 工具调用请求
	AgentEventTypeFinished                                // 完成
)

// TextChunkEvent 文本块事件
type TextChunkEvent struct {
	Chunk   string
	Emotion string
}

func (e *TextChunkEvent) Type() AgentEventType {
	return AgentEventTypeTextChunk
}

// EmotionChangedEvent 情绪变化事件
type EmotionChangedEvent struct {
	Emotion string
}

func (e *EmotionChangedEvent) Type() AgentEventType {
	return AgentEventTypeEmotionChanged
}

// ToolCallRequestedEvent 工具调用请求事件
type ToolCallRequestedEvent struct {
	Tool     string
	Args     map[string]interface{}
	ToolType ToolType // 查询类 or 动作类
}

func (e *ToolCallRequestedEvent) Type() AgentEventType {
	return AgentEventTypeToolCallRequested
}

// FinishedEvent 完成事件
type FinishedEvent struct {
	Error error
}

func (e *FinishedEvent) Type() AgentEventType {
	return AgentEventTypeFinished
}
