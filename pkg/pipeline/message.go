package pipeline

import "time"

// MessageType 消息类型
type MessageType string

const (
	// MessageTypeData 数据帧，Payload 的具体类型由各 Stage 自行约定。
	// Pipeline 框架不感知 Payload 内容，只负责透传。
	MessageTypeData MessageType = "data"

	// 控制信号：框架级别，Stage 感知并处理。
	MessageTypeFinished  MessageType = "finished"  // 流正常结束
	MessageTypeError     MessageType = "error"     // 错误（携带于 Metadata.Error）
	MessageTypeInterrupt MessageType = "interrupt" // 打断/取消
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
	Language  string // ASR-detected language code, e.g. "zh"
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
