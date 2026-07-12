package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
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
			a.recordTurn(ctx, sess, processStart)
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

	a.recordTurn(ctx, sess, processStart)
	logging.Infof("Agent: reached max steps (%d)", a.maxSteps)
	emit(&FinishedEvent{Error: fmt.Errorf("reached max steps (%d)", a.maxSteps)})
}

// toolCallRecord holds one tool invocation + its result for turn persistence.
// This intentionally mirrors session.ToolCall but adds the execution result
// so the Manager can display tool usage without re-playing the conversation.
type toolCallRecord struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Result    string `json:"result"`
}

// recordTurn captures the last user/assistant pair from the session, including
// any intermediate tool calls, and saves it to memory.
func (a *Agent) recordTurn(ctx context.Context, sess *session.Session, start time.Time) {
	if a.memorySvc == nil {
		return
	}
	msgs := sess.Messages
	if len(msgs) < 2 {
		return
	}

	// Find the last user message.
	lastUserIdx := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == session.RoleUser {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx < 0 {
		return
	}
	userText := msgs[lastUserIdx].Content

	// Find the first assistant message after lastUserIdx that has no tool calls
	// (the "final" response). The intermediate assistant message(s) with
	// tool_calls are skipped because they are not the actual answer.
	var (
		assistantText     string
		finalAssistantIdx = -1
	)
	for i := lastUserIdx + 1; i < len(msgs); i++ {
		if msgs[i].Role == session.RoleAssistant && len(msgs[i].ToolCalls) == 0 {
			assistantText = msgs[i].Content
			finalAssistantIdx = i
			break
		}
	}
	if assistantText == "" {
		return
	}

	// Collect tool calls between the user message and the final assistant
	// response. Each assistant(tool_calls) message carries the invocation;
	// the immediately following RoleTool messages carry the results keyed by
	// ToolCallID.
	var toolRecords []toolCallRecord
	for i := lastUserIdx + 1; i < finalAssistantIdx; i++ {
		if msgs[i].Role == session.RoleAssistant && len(msgs[i].ToolCalls) > 0 {
			for _, tc := range msgs[i].ToolCalls {
				// Find the matching tool result message.
				var result string
				for j := i + 1; j < finalAssistantIdx; j++ {
					if msgs[j].Role == session.RoleTool && msgs[j].ToolCallID == tc.ID {
						result = msgs[j].Content
						break
					}
				}
				toolRecords = append(toolRecords, toolCallRecord{
					Name:      tc.Name,
					Arguments: tc.Arguments,
					Result:    result,
				})
			}
		}
	}

	var toolsJSON string
	if len(toolRecords) > 0 {
		data, _ := json.Marshal(toolRecords)
		toolsJSON = string(data)
	}

	// Use the count of user messages as the sequential turn number.
	// This is correct even when tool-call messages inflate len(msgs),
	// because each conversation turn produces exactly one user message.
	var turnSeq int64
	for _, m := range msgs {
		if m.Role == session.RoleUser {
			turnSeq++
		}
	}

	memCtx, _ := memory.FromContext(ctx)
	turn := memory.Turn{
		TurnID:        turnSeq,
		UserText:      userText,
		AssistantText: assistantText,
		ToolsJSON:     toolsJSON,
		StartedAt:     start,
		EndedAt:       time.Now(),
		SessionID:     sess.ID,
		DeviceID:      memCtx.DeviceID,
		UserID:        memCtx.UserID,
	}
	if err := a.memorySvc.RecordTurn(ctx, turn); err != nil {
		logging.Warnf("Agent: record turn: %v", err)
	}
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
