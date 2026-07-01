package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/session"
)

// Run 启动一次 Agent 循环：构建上下文 → 调用 LLM → （如需要）执行工具 → 重复，
// 直至模型给出无工具调用的最终回复、达到 maxSteps，或 ctx 被取消。
func (a *Agent) Run(ctx context.Context, sess *session.Session) (<-chan AgentEvent, error) {
	eventChan := make(chan AgentEvent)

	go func() {
		defer close(eventChan)
		emit := func(e AgentEvent) bool {
			select {
			case eventChan <- e:
				return true
			case <-ctx.Done():
				return false
			}
		}
		a.runLoop(ctx, sess, emit)
	}()

	return eventChan, nil
}

// runLoop 是循环编排骨架：驱动每一步的上下文构建、LLM 调用与工具执行，并裁决何时终止。
func (a *Agent) runLoop(ctx context.Context, sess *session.Session, emit func(AgentEvent) bool) {
	processStart := time.Now()

	for step := 0; step < a.maxSteps; step++ {
		if ctx.Err() != nil {
			return
		}
		logging.Infof("Agent: step %d/%d", step+1, a.maxSteps)

		messages := a.buildContextMessages(ctx, sess)

		result, err := a.runStep(ctx, messages, emit)
		if err != nil {
			if isContextErr(err) {
				return
			}
			logging.Errorf("Agent: step error: %v", err)
			emit(&FinishedEvent{Error: err})
			return
		}

		sess.Add(session.Message{
			Role:      session.RoleAssistant,
			Content:   result.text,
			ToolCalls: toSessionToolCalls(result.toolCalls),
		})

		if len(result.toolCalls) == 0 {
			logging.Infof("Agent: done (no tool calls), total time=%v", time.Since(processStart))
			emit(&FinishedEvent{Error: nil})
			return
		}

		toolMessages, fatalErr := a.executeToolCalls(ctx, result.toolCalls)
		for _, msg := range toolMessages {
			sess.Add(msg)
		}
		if fatalErr != nil {
			if isContextErr(fatalErr) {
				return
			}
			logging.Errorf("Agent: tool exec error: %v", fatalErr)
			emit(&FinishedEvent{Error: fatalErr})
			return
		}
	}

	logging.Infof("Agent: reached max steps (%d)", a.maxSteps)
	emit(&FinishedEvent{Error: fmt.Errorf("reached max steps (%d)", a.maxSteps)})
}

// isContextErr 判断错误是否源自 ctx 取消/超时——这类错误不应产生 FinishedEvent，
// 因为 channel 消费方已经不再监听。
func isContextErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// toSessionToolCalls 将 LLM 工具调用转换为 session 存储格式。
func toSessionToolCalls(calls []llm.ToolCall) []session.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]session.ToolCall, len(calls))
	for i, tc := range calls {
		result[i] = session.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments}
	}
	return result
}
