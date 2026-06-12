package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/session"
)

func (a *Agent) Run(ctx context.Context, sess *session.Session) (<-chan AgentEvent, error) {
	maxSteps := a.maxSteps

	eventChan := make(chan AgentEvent)

	go func() {
		defer close(eventChan)
		processStart := time.Now()

		for step := 0; step < maxSteps; step++ {
			select {
			case <-ctx.Done():
				return
			default:
			}

			logging.Infof("Agent: step %d/%d", step+1, maxSteps)

			messages := a.buildSessionMessages(ctx, sess)

			streamStart := time.Now()
			stream, err := a.client.Chat(ctx, llm.Request{
				Messages: messages,
				Tools:    a.registry.Definitions(),
			})
			if err != nil {
				logging.Errorf("Agent: LLM stream error: %v", err)
				select {
				case eventChan <- &FinishedEvent{Error: err}:
				case <-ctx.Done():
				}
				return
			}
			logging.Infof("Agent: LLM stream established in %v", time.Since(streamStart))

			fullText := ""
			bufferedContent := ""
			lastFilteredLength := 0
			firstChunkLogged := false
			var toolCalls []llm.ToolCall

			for {
				msg, err := stream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					logging.Errorf("Agent: stream receive error: %v", err)
					select {
					case eventChan <- &FinishedEvent{Error: err}:
					case <-ctx.Done():
					}
					stream.Close()
					return
				}

				if msg.Content != "" {
					bufferedContent += msg.Content
					newContent, nextLength := deltaFromBufferedContent(bufferedContent, lastFilteredLength)
					if newContent != "" {
						if !firstChunkLogged {
							firstChunkLogged = true
							logging.Infof("Agent: first chunk in %v", time.Since(streamStart))
						}
						select {
						case eventChan <- &TextChunkEvent{Chunk: newContent}:
						case <-ctx.Done():
							return
						}
						fullText += newContent
					}
					lastFilteredLength = nextLength
				}

				if len(msg.ToolCalls) > 0 {
					toolCalls = msg.ToolCalls
				}
			}
			stream.Close()

			if len(toolCalls) == 0 {
				sess.Add(session.Message{Role: session.RoleAssistant, Content: fullText})
				logging.Infof("Agent: done (no tool calls), total time=%v", time.Since(processStart))
				select {
				case eventChan <- &FinishedEvent{Error: nil}:
				case <-ctx.Done():
				}
				return
			}

			sessionToolCalls := make([]session.ToolCall, len(toolCalls))
			for i, tc := range toolCalls {
				sessionToolCalls[i] = session.ToolCall{
					ID:        tc.ID,
					Name:      tc.Name,
					Arguments: tc.Arguments,
				}
			}
			sess.Add(session.Message{Role: session.RoleAssistant, Content: fullText, ToolCalls: sessionToolCalls})

			for _, call := range toolCalls {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if !a.registry.CanExecute(call.Name) {
					err := fmt.Errorf("unknown tool: %s", call.Name)
					logging.Errorf("Agent: %v", err)
					select {
					case eventChan <- &FinishedEvent{Error: err}:
					case <-ctx.Done():
					}
					return
				}

				logging.Infof("Agent: executing tool: %s", call.Name)
				rawArgs := json.RawMessage(call.Arguments)
				result, err := a.registry.Execute(ctx, call.Name, rawArgs)
				if err != nil {
					logging.Errorf("Agent: tool exec error: %v", err)
					select {
					case eventChan <- &FinishedEvent{Error: err}:
					case <-ctx.Done():
					}
					return
				}

				if result.Error != nil {
					logging.Errorf("Agent: tool error: %v", result.Error)
				}

				sess.Add(session.Message{
					Role:       session.RoleTool,
					ToolCallID: call.ID,
					Content:    result.Output,
				})
			}
		}

		logging.Infof("Agent: reached max steps (%d)", maxSteps)
		select {
		case eventChan <- &FinishedEvent{Error: fmt.Errorf("reached max steps (%d)", maxSteps)}:
		case <-ctx.Done():
		}
	}()

	return eventChan, nil
}

func (a *Agent) buildSessionMessages(ctx context.Context, sess *session.Session) []llm.Message {
	msgs := sess.ToLLMMessages()

	if a.memorySvc != nil {
		memMsgs, err := a.memorySvc.BuildContextMessages(ctx, "")
		if err != nil {
			logging.Warnf("Agent: build memory context failed: %v", err)
		} else {
			result := make([]llm.Message, 0, len(memMsgs)+len(msgs))
			for _, m := range memMsgs {
				if m.Role == "system" {
					result = append(result, llm.Message{Role: string(m.Role), Content: m.Content})
				}
			}
			result = append(result, msgs...)
			return result
		}
	}

	result := make([]llm.Message, 0, len(msgs)+1)
	result = append(result, llm.Message{Role: "system", Content: defaultSystemPrompt})
	result = append(result, msgs...)
	return result
}
