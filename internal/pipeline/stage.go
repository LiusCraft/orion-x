package pipeline

import "context"

// Stage Pipeline 中的处理节点
type Stage interface {
	// Name 返回 Stage 名称（用于日志/metrics）
	Name() string

	// Process 处理输入流，返回输出流
	// ctx 用于取消和超时控制
	// input 可能为 nil（对于源 Stage，如 AudioInput）
	Process(ctx context.Context, input <-chan Message) <-chan Message
}

// PassthroughStage 透传 Stage，用于插入 metrics/logging/filtering
type PassthroughStage interface {
	Stage
	// OnMessage 处理每条消息，可修改/过滤
	// 返回 nil 表示过滤该消息
	OnMessage(msg Message) *Message
}

// BaseStage 提供基础实现
type BaseStage struct {
	name string
}

func (s *BaseStage) Name() string {
	return s.name
}

// NewBaseStage 创建基础 Stage
func NewBaseStage(name string) *BaseStage {
	return &BaseStage{name: name}
}
