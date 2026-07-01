package agent

import (
	"context"
	"io"
	"time"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/logging"
)

// stepResult 是单轮 LLM 调用的结果：累积的完整文本与本轮收集到的工具调用。
type stepResult struct {
	text      string
	toolCalls []llm.ToolCall
}

// runStep 执行一次 LLM 调用并消费其流式响应：文本增量通过 emit 发送 TextChunkEvent，
// 工具调用被收集后返回。emit 返回 false（ctx 已取消）时提前返回 ctx.Err()。
func (a *Agent) runStep(ctx context.Context, messages []llm.Message, emit func(AgentEvent) bool) (stepResult, error) {
	streamStart := time.Now()
	stream, err := a.client.Chat(ctx, llm.Request{
		Messages: messages,
		Tools:    a.registry.Definitions(),
	})
	if err != nil {
		return stepResult{}, err
	}
	defer stream.Close()
	logging.Infof("Agent: LLM stream established in %v", time.Since(streamStart))

	var fullText, bufferedContent string
	lastFilteredLength := 0
	firstChunkLogged := false
	var toolCalls []llm.ToolCall

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return stepResult{}, err
		}

		if msg.Content != "" {
			bufferedContent += msg.Content
			newContent, nextLength := deltaFromBufferedContent(bufferedContent, lastFilteredLength)
			if newContent != "" {
				if !firstChunkLogged {
					firstChunkLogged = true
					logging.Infof("Agent: first chunk in %v", time.Since(streamStart))
				}
				if !emit(&TextChunkEvent{Chunk: newContent}) {
					return stepResult{}, ctx.Err()
				}
				fullText += newContent
			}
			lastFilteredLength = nextLength
		}

		if len(msg.ToolCalls) > 0 {
			toolCalls = msg.ToolCalls
		}
	}

	return stepResult{text: fullText, toolCalls: toolCalls}, nil
}

// deltaFromBufferedContent 从累积 buffer 中提取自 lastLength 之后的新增量文本。
func deltaFromBufferedContent(content string, lastLength int) (string, int) {
	if lastLength < 0 {
		lastLength = 0
	}
	if lastLength > len(content) {
		lastLength = len(content)
	}
	return content[lastLength:], len(content)
}
