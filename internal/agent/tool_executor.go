package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/session"
)

// toolCallOutcome 是单个工具调用的执行结果。fatal 非 nil 时代表框架级错误，
// 调用方必须终止整个 Run；否则 message 是要回写 session 的 tool 消息。
type toolCallOutcome struct {
	message session.Message
	fatal   error
}

// executeToolCalls 执行本轮全部工具调用，结果按 calls 的原始顺序返回。
// 当所有调用都标记为 ParallelSafe 时并发执行，否则串行执行。
// 返回的 error 非 nil 时，调用方（runLoop）应终止整个 Run。
func (a *Agent) executeToolCalls(ctx context.Context, calls []llm.ToolCall) ([]session.Message, error) {
	outcomes := make([]toolCallOutcome, len(calls))

	if a.allParallelSafe(calls) {
		var wg sync.WaitGroup
		for i, call := range calls {
			wg.Add(1)
			go func(i int, call llm.ToolCall) {
				defer wg.Done()
				outcomes[i] = a.runOneToolCall(ctx, call)
			}(i, call)
		}
		wg.Wait()
	} else {
		for i, call := range calls {
			outcomes[i] = a.runOneToolCall(ctx, call)
		}
	}

	messages := make([]session.Message, 0, len(outcomes))
	for _, o := range outcomes {
		if o.fatal != nil {
			return messages, o.fatal
		}
		messages = append(messages, o.message)
	}
	return messages, nil
}

// allParallelSafe 判断本轮工具调用是否可以整体并发执行：至少两个调用，且全部
// 标记为 ParallelSafe。只要有一个不是（含未注册工具），整轮退回串行。
func (a *Agent) allParallelSafe(calls []llm.ToolCall) bool {
	if len(calls) < 2 {
		return false
	}
	for _, call := range calls {
		if !a.registry.IsParallelSafe(call.Name) {
			return false
		}
	}
	return true
}

// runOneToolCall 执行单个工具调用。工具不存在会生成一条可恢复的 tool 错误消息
// 而不终止 Run；Execute 返回的 Go error 视为框架级故障，终止整个 Run；
// Result.Error 非空时记录日志，并在 Output 为空时用错误文本兜底，确保 LLM
// 总能看到失败原因。
func (a *Agent) runOneToolCall(ctx context.Context, call llm.ToolCall) toolCallOutcome {
	if !a.registry.CanExecute(call.Name) {
		logging.Warnf("Agent: unknown tool: %s", call.Name)
		return toolCallOutcome{message: session.Message{
			Role:       session.RoleTool,
			ToolCallID: call.ID,
			Content:    fmt.Sprintf("error: unknown tool %q", call.Name),
		}}
	}

	logging.Infof("Agent: executing tool: %s", call.Name)
	result, err := a.registry.Execute(ctx, call.Name, json.RawMessage(call.Arguments))
	if err != nil {
		return toolCallOutcome{fatal: err}
	}

	content := result.Output
	if result.Error != nil {
		logging.Errorf("Agent: tool error: %v", result.Error)
		if content == "" {
			content = result.Error.Error()
		}
	}

	return toolCallOutcome{message: session.Message{
		Role:       session.RoleTool,
		ToolCallID: call.ID,
		Content:    content,
	}}
}
