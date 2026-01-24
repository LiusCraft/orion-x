package agent

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
)

type voiceAgentImpl struct {
	chatModel         *openai.ChatModel
	emotionExtractor  EmotionExtractor
	markdownFilter    MarkdownFilter
	toolClassifier    *ToolClassifier
	actionResponseGen *ActionResponseGenerator
}

func NewVoiceAgent(ctx context.Context) (VoiceAgent, error) {
	key := "b69a4b0f05124a59835be2adf1ac84a5.yvFfw6QQNJDXT7IP"
	baseURL := "https://open.bigmodel.cn/api/coding/paas/v4"

	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseURL,
		Model:   "glm-4-flash",
		APIKey:  key,
	})
	if err != nil {
		return nil, err
	}

	return &voiceAgentImpl{
		chatModel:         chatModel,
		emotionExtractor:  NewEmotionExtractor(),
		markdownFilter:    NewMarkdownFilter(),
		toolClassifier:    NewToolClassifier(),
		actionResponseGen: NewActionResponseGenerator(),
	}, nil
}

func (v *voiceAgentImpl) Process(ctx context.Context, input string) (<-chan AgentEvent, error) {
	eventChan := make(chan AgentEvent)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(eventChan)

		messages := []*schema.Message{
			schema.SystemMessage("你是一个语音助手。在每个句子的开头包含情绪标签，格式为 [EMO:emotion]，可选值：happy, sad, angry, calm, excited。例如：[EMO:happy] 你好啊！[EMO:calm] 今天有什么可以帮你？"),
			schema.UserMessage(input),
		}

		stream, err := v.chatModel.Stream(ctx, messages)
		if err != nil {
			eventChan <- &FinishedEvent{Error: err}
			return
		}
		defer stream.Close()

		currentEmotion := ""
		fullText := ""
		bufferedContent := ""

		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				eventChan <- &FinishedEvent{Error: err}
				return
			}

			if msg.Content != "" {
				bufferedContent += msg.Content

				emotion := v.emotionExtractor.Extract(bufferedContent)
				if emotion != "" && emotion != currentEmotion {
					currentEmotion = emotion
					eventChan <- &EmotionChangedEvent{Emotion: emotion}
				}

				filtered := v.markdownFilter.Filter(msg.Content)
				if filtered != "" {
					eventChan <- &TextChunkEvent{Chunk: filtered, Emotion: currentEmotion}
					fullText += filtered
				}
			}

			for _, toolCall := range msg.ToolCalls {
				toolType := v.toolClassifier.GetToolType(toolCall.Function.Name)
				args := parseToolArgs(toolCall.Function.Arguments)

				eventChan <- &ToolCallRequestedEvent{
					Tool:     toolCall.Function.Name,
					Args:     args,
					ToolType: toolType,
				}

				if toolType == ToolTypeAction {
					response := v.actionResponseGen.GenerateResponse(toolCall.Function.Name, args)
					filtered := v.markdownFilter.Filter(response)
					emotion := v.emotionExtractor.Extract(response)

					if emotion != "" && emotion != currentEmotion {
						currentEmotion = emotion
						eventChan <- &EmotionChangedEvent{Emotion: emotion}
					}

					if filtered != "" {
						eventChan <- &TextChunkEvent{Chunk: filtered, Emotion: currentEmotion}
					}
				}
			}
		}

		eventChan <- &FinishedEvent{Error: nil}
	}()

	return eventChan, nil
}

func (v *voiceAgentImpl) GetToolType(tool string) ToolType {
	return v.toolClassifier.GetToolType(tool)
}

func parseToolArgs(argsJSON string) map[string]interface{} {
	result := make(map[string]interface{})
	if argsJSON == "" {
		return result
	}

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		log.Printf("Failed to parse tool args: %v", err)
		return result
	}

	return args
}
