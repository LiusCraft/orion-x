package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/cloudwego/eino/schema"
	"github.com/liuscraft/orion-x/internal/logging"
)

// summarizeToolResult 内部方法：调用 LLM 总结工具结果，直接写入 eventChan
func (v *Agent) summarizeToolResult(ctx context.Context, eventChan chan<- AgentEvent, tool string, args map[string]interface{}, result interface{}) error {
	if v.chatModel == nil {
		return errors.New("llm chat model is not initialized")
	}

	messages := buildToolSummaryMessages(tool, args, result)

	logging.Infof("Agent: starting tool summary stream (tool=%s)...", tool)
	stream, err := v.chatModel.Stream(ctx, messages)
	if err != nil {
		logging.Errorf("Agent: tool summary stream error: %v", err)
		return err
	}
	defer stream.Close()

	fullText := ""
	bufferedContent := ""
	lastFilteredLength := 0

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			logging.Infof("Agent: tool summary completed, total text length: %d", len(fullText))
			break
		}
		if err != nil {
			logging.Errorf("Agent: tool summary receive error: %v", err)
			return err
		}

		if len(msg.ToolCalls) > 0 {
			err := fmt.Errorf("tool summary should not call tools: %s", msg.ToolCalls[0].Function.Name)
			logging.Errorf("Agent: %v", err)
			return err
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
	}

	return nil
}

func buildToolSummaryMessages(tool string, args map[string]interface{}, result interface{}) []*schema.Message {
	argsText := stringifyJSONOrValue(args)
	resultText := stringifyJSONOrValue(result)

	userContent := fmt.Sprintf("工具: %s\n参数: %s\n结果: %s\n请用简洁中文向用户总结结果，必要时分点说明，不要调用工具。", tool, argsText, resultText)

	return []*schema.Message{
		schema.SystemMessage(`你是一个通用 AI 助手，请根据工具结果生成简洁回复。
不要调用任何工具。`),
		schema.UserMessage(userContent),
	}
}

func stringifyJSONOrValue(value interface{}) string {
	if value == nil {
		return "null"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}
