package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/logging"
)

func (v *Agent) Process(ctx context.Context, input string) (<-chan AgentEvent, error) {
	logging.Infof("Agent: processing input: %s", input)
	eventChan := make(chan AgentEvent)
	var wg sync.WaitGroup
	processStart := time.Now()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(eventChan)

		messages := v.buildMessages(ctx, input)

		logging.Infof("Agent: starting LLM stream...")
		streamStart := time.Now()
		stream, err := v.chatModel.Stream(ctx, messages)
		if err != nil {
			logging.Errorf("Agent: LLM stream error: %v", err)
			eventChan <- &FinishedEvent{Error: err}
			return
		}
		defer stream.Close()
		logging.Infof("Agent: LLM stream established in %v", time.Since(streamStart))

		fullText := ""
		bufferedContent := ""
		lastFilteredLength := 0
		firstChunkLogged := false

		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				if !firstChunkLogged {
					logging.Infof(
						"Agent: LLM stream completed without text chunk (request_to_finish=%v, process_to_finish=%v)",
						time.Since(streamStart),
						time.Since(processStart),
					)
				}
				logging.Infof("Agent: LLM stream completed, total text length: %d", len(fullText))
				break
			}
			if err != nil {
				logging.Errorf("Agent: stream receive error: %v", err)
				eventChan <- &FinishedEvent{Error: err}
				return
			}

			if msg.Content != "" {
				bufferedContent += msg.Content

				newContent, nextLength := deltaFromBufferedContent(bufferedContent, lastFilteredLength)
				if newContent != "" {
					if !firstChunkLogged {
						firstChunkLogged = true
						logging.Infof(
							"Agent: first LLM text chunk received (request_to_first_chunk=%v, process_to_first_chunk=%v)",
							time.Since(streamStart),
							time.Since(processStart),
						)
					}
					eventChan <- &TextChunkEvent{Chunk: newContent}
					fullText += newContent
				}
				lastFilteredLength = nextLength
			}

			// 处理工具调用（内部执行）
			for _, toolCall := range msg.ToolCalls {
				if v.toolManager != nil && !v.toolManager.Has(toolCall.Function.Name) {
					err := fmt.Errorf("unknown tool: %s", toolCall.Function.Name)
					logging.Errorf("Agent: %v", err)
					eventChan <- &FinishedEvent{Error: err}
					return
				}
				args, err := parseToolArgs(toolCall.Function.Arguments)
				if err != nil {
					logging.Errorf("Agent: tool args parse error: %v", err)
					eventChan <- &FinishedEvent{Error: err}
					return
				}

				logging.Infof("Agent: executing tool: %s, args: %v", toolCall.Function.Name, args)
				
				// 执行工具（内部方法）
				result, err := v.executeTool(ctx, toolCall.Function.Name, args)
				if err != nil {
					logging.Errorf("Agent: tool execution error: %v", err)
					eventChan <- &FinishedEvent{Error: err}
					return
				}
				
				// 调用 LLM 总结工具结果
				logging.Infof("Agent: summarizing tool result")
				if err := v.summarizeToolResult(ctx, eventChan, toolCall.Function.Name, args, result); err != nil {
					logging.Errorf("Agent: tool summary error: %v", err)
					eventChan <- &FinishedEvent{Error: err}
					return
				}
			}
		}

		logging.Infof("Agent: processing finished")
		eventChan <- &FinishedEvent{Error: nil}
	}()

	return eventChan, nil
}

func parseToolArgs(argsJSON string) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	if argsJSON == "" {
		return result, nil
	}

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return result, fmt.Errorf("parse tool args: %w", err)
	}

	return args, nil
}

// executeTool 执行工具调用（内部方法）
func (v *Agent) executeTool(ctx context.Context, toolName string, args map[string]interface{}) (interface{}, error) {
	// 1. 获取工具
	loadedTool, ok := v.toolManager.GetTool(toolName)
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", toolName)
	}

	// 2. 类型断言为 InvokableTool
	invokableTool, ok := loadedTool.(interface {
		InvokableRun(ctx context.Context, argumentsInJSON string) (string, error)
	})
	if !ok {
		return nil, fmt.Errorf("tool is not invokable: %s", toolName)
	}

	// 3. 序列化参数
	if args == nil {
		args = map[string]interface{}{}
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("marshal tool args: %w", err)
	}

	// 4. 执行工具
	result, err := invokableTool.InvokableRun(ctx, string(argsJSON))
	if err != nil {
		return nil, fmt.Errorf("invoke tool %s: %w", toolName, err)
	}

	// 5. 解析结果
	return parseToolOutput(result), nil
}

// parseToolOutput 解析工具输出
func parseToolOutput(raw string) interface{} {
	var parsed interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		// 如果解析失败，返回原始字符串
		return raw
	}
	return parsed
}
