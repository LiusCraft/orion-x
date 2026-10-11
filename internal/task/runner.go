package task

import (
	"context"

	"github.com/liuscraft/orion-x/internal/agent"
)

// Runner 为一个任务启动后台执行。实现既可以是进程内的 LLM 循环
// （LocalRunner），也可以是远程托管 agent（Claude Managed Agents、
// 阿里云百炼 Managed Agents 等）。
//
// Runner 自己持有执行生命周期（构造时注入 base ctx），Start 返回后
// 执行仍在后台进行，不受调用方 tool-call ctx 影响。
type Runner interface {
	// Start 启动一次执行并立即返回 Run 句柄；执行过程中的输出通过
	// Run.Events 流式返回。
	Start(req RunRequest) (Run, error)
}

// RunRequest 是一次任务执行需要的最小输入。后端特有的配置（模型、
// 系统提示、远端 Agent/Environment）由各自 Runner 的构造参数承担。
type RunRequest struct {
	TaskID string
	Title  string
}

// Run 是一次后台执行的句柄。
type Run interface {
	// Events 返回执行事件流；在 EventDone/EventError 之后通道关闭。
	Events() <-chan Event
	// Ref 是执行后端的引用（本地执行为 run:<taskID>，远程托管为远端
	// 会话 ID），写入 Task.RunID。
	Ref() string
	// Interrupt 请求后端停止当前执行但不释放资源。后端不支持打断语义
	// 时可以退化为取消（如进程内 loop）；调用方可用 Close 兜底。
	Interrupt(ctx context.Context) error
	// Close 释放执行资源并终止尚未完成的任务；幂等。事件流正常结束后
	// 实现方应自行释放资源，Close 主要用于提前取消或兜底。
	Close() error
}

// EventType 标识任务执行事件的类型。
type EventType string

const (
	// EventText 是一次模型文本增量。
	EventText EventType = "task:text"
	// EventToolUse 表示后端发起了一次工具调用。
	EventToolUse EventType = "task:tool:use"
	// EventDone 表示任务正常结束，是终止事件。
	EventDone EventType = "task:done"
	// EventError 表示任务失败或被取消，是终止事件。
	EventError EventType = "task:error"
)

// Event 是各执行后端上报的统一事件。本地 loop 与远程托管 agent 都
// 映射到这一种事件，Registry 只认它。
type Event struct {
	Type EventType
	// Text 仅在 EventText 时有值。
	Text string
	// Tool 仅在 EventToolUse 时有值。
	Tool *ToolUse
	// Usage 仅在 EventDone 时有值，计费未启用或后端不上报用量时为 nil。
	// 口径与 agent.Usage 一致，Registry 会把它放进 pipeline 元数据。
	Usage *agent.Usage
	// Err 仅在 EventError 时有值。
	Err error
}

// ToolUse 描述一次工具调用，供进度展示与调试使用。
type ToolUse struct {
	Name  string
	Input string
}
