package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/liuscraft/orion-x/internal/logging"
)

func (v *Agent) Process(ctx context.Context, input string) (<-chan AgentEvent, error) {
	logging.Infof("Agent: processing input: %s", input)
	eventChan := make(chan AgentEvent)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(eventChan)

		messages := v.buildMessages(ctx, input)

		logging.Infof("Agent: starting LLM stream...")
		stream, err := v.chatModel.Stream(ctx, messages)
		if err != nil {
			logging.Errorf("Agent: LLM stream error: %v", err)
			eventChan <- &FinishedEvent{Error: err}
			return
		}
		defer stream.Close()

		fullText := ""
		bufferedContent := ""
		lastFilteredLength := 0

		for {
			msg, err := stream.Recv()
			if err == io.EOF {
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
					eventChan <- &TextChunkEvent{Chunk: newContent}
					fullText += newContent
				}
				lastFilteredLength = nextLength
			}

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

				logging.Infof("Agent: tool call requested: %s, args: %v", toolCall.Function.Name, args)
				eventChan <- &ToolCallRequestedEvent{
					Tool: toolCall.Function.Name,
					Args: args,
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
