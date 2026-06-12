package pipeline

import "time"

// MessageType 消息类型
type MessageType string

const (
	MessageTypeTextChunk   MessageType = "text_chunk"
	MessageTypeTextPartial MessageType = "text_partial" // ASR interim result
	MessageTypeAudioData   MessageType = "audio_data"
	MessageTypeToolCall    MessageType = "tool_call"
	MessageTypeToolResult  MessageType = "tool_result"
	MessageTypeEmotion     MessageType = "emotion"
	MessageTypeFinished    MessageType = "finished"
	MessageTypeError       MessageType = "error"
	MessageTypeInterrupt   MessageType = "interrupt" // 用户打断
	MessageTypeTTSStart    MessageType = "tts_start" // TTS 开始
	MessageTypeTTSStop     MessageType = "tts_stop"  // TTS 停止
)

// Message Pipeline 中流转的数据单元
type Message struct {
	Type     MessageType
	Payload  interface{}
	Metadata Metadata
}

// Metadata 消息元数据，贯穿整个 Pipeline
type Metadata struct {
	TurnID    int64
	TraceID   string
	Emotion   string
	Timestamp time.Time
	Error     error
	Extra     map[string]interface{}
}

// WithError 设置错误
func (m Metadata) WithError(err error) Metadata {
	m.Error = err
	return m
}

// NewMessage 创建新消息
func NewMessage(msgType MessageType, payload interface{}) Message {
	return Message{
		Type:    msgType,
		Payload: payload,
		Metadata: Metadata{
			Timestamp: time.Now(),
		},
	}
}

// WithMetadata 设置消息元数据
func (m Message) WithMetadata(md Metadata) Message {
	m.Metadata = md
	return m
}

// WithError 设置错误
func (m Message) WithError(err error) Message {
	m.Metadata.Error = err
	return m
}

// IsError 判断是否为错误消息
func (m Message) IsError() bool {
	return m.Metadata.Error != nil
}
