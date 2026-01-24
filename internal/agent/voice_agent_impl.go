package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
	"github.com/liuscraft/orion-x/internal/logging"
)

type voiceAgentImpl struct {
	chatModel         *openai.ChatModel
	emotionExtractor  EmotionExtractor
	markdownFilter    MarkdownFilter
	toolClassifier    *ToolClassifier
	actionResponseGen *ActionResponseGenerator
}

const (
	defaultLLMBaseURL = "https://open.bigmodel.cn/api/coding/paas/v4"
	defaultLLMModel   = "glm-4-flash"
)

func NewVoiceAgent(ctx context.Context) (VoiceAgent, error) {
	key := os.Getenv("ZHIPU_API_KEY")
	if key == "" {
		key = os.Getenv("DASHSCOPE_API_KEY")
	}
	return NewVoiceAgentWithConfig(ctx, Config{
		APIKey: key,
	})
}

func NewVoiceAgentWithConfig(ctx context.Context, cfg Config) (VoiceAgent, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}

	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: normalized.BaseURL,
		Model:   normalized.Model,
		APIKey:  normalized.APIKey,
	})
	if err != nil {
		return nil, err
	}

	classifier := NewToolClassifierWithTypes(normalized.ToolTypes)
	responseGen := NewActionResponseGeneratorWithTemplates(normalized.ActionResponses)

	return &voiceAgentImpl{
		chatModel:         chatModel,
		emotionExtractor:  NewEmotionExtractor(),
		markdownFilter:    NewMarkdownFilter(),
		toolClassifier:    classifier,
		actionResponseGen: responseGen,
	}, nil
}

func (v *voiceAgentImpl) Process(ctx context.Context, input string) (<-chan AgentEvent, error) {
	logging.Infof("VoiceAgent: processing input: %s", input)
	eventChan := make(chan AgentEvent)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(eventChan)

		messages := []*schema.Message{
			schema.SystemMessage(`你是一个语音助手。

规则：
1. 当用户询问时间时，请使用 getTime 工具获取准确时间。

2. 当用户询问天气时，请使用 getWeather 工具。

工具定义：
- getTime: 获取当前时间，返回日期、时间、星期、时区等信息
- getWeather: 获取指定城市的天气信息，需要参数 city（城市名称）`),
			schema.UserMessage(input),
		}

		logging.Infof("VoiceAgent: starting LLM stream...")
		stream, err := v.chatModel.Stream(ctx, messages)
		if err != nil {
			logging.Errorf("VoiceAgent: LLM stream error: %v", err)
			eventChan <- &FinishedEvent{Error: err}
			return
		}
		defer stream.Close()

		currentEmotion := "default"
		fullText := ""
		bufferedContent := ""
		lastFilteredLength := 0

		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				logging.Infof("VoiceAgent: LLM stream completed, total text length: %d", len(fullText))
				break
			}
			if err != nil {
				logging.Errorf("VoiceAgent: stream receive error: %v", err)
				eventChan <- &FinishedEvent{Error: err}
				return
			}

			if msg.Content != "" {
				bufferedContent += msg.Content

				// emotion := v.emotionExtractor.Extract(bufferedContent)
				// if emotion != "" && emotion != currentEmotion {
				// 	currentEmotion = emotion
				// 	log.Printf("VoiceAgent: emotion changed to: %s", emotion)
				// 	eventChan <- &EmotionChangedEvent{Emotion: emotion}
				// }

				// 移除缓冲内容中的情绪标签
				// cleanBufferedContent := v.markdownFilter.RemoveEmotionTags(bufferedContent)
				cleanBufferedContent := bufferedContent

				newContent, nextLength := deltaFromBufferedContent(cleanBufferedContent, lastFilteredLength)
				if newContent != "" {
					logging.Infof("VoiceAgent: text chunk: %s (emotion: %s)", newContent, currentEmotion)
					eventChan <- &TextChunkEvent{Chunk: newContent, Emotion: currentEmotion}
					fullText += newContent
				}
				lastFilteredLength = nextLength
			}

			for _, toolCall := range msg.ToolCalls {
				toolType := v.toolClassifier.GetToolType(toolCall.Function.Name)
				args := parseToolArgs(toolCall.Function.Arguments)

				logging.Infof("VoiceAgent: tool call requested: %s (type: %s), args: %v", toolCall.Function.Name, toolType, args)
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
						logging.Infof("VoiceAgent: emotion changed to: %s (from action response)", emotion)
						eventChan <- &EmotionChangedEvent{Emotion: emotion}
					}

					if filtered != "" {
						logging.Infof("VoiceAgent: action response: %s", filtered)
						eventChan <- &TextChunkEvent{Chunk: filtered, Emotion: currentEmotion}
					}
				}
			}
		}

		logging.Infof("VoiceAgent: processing finished")
		eventChan <- &FinishedEvent{Error: nil}
	}()

	return eventChan, nil
}

func (v *voiceAgentImpl) GetToolType(tool string) ToolType {
	return v.toolClassifier.GetToolType(tool)
}

func deltaFromBufferedContent(content string, lastLength int) (string, int) {
	if lastLength < 0 {
		lastLength = 0
	}
	if lastLength > len(content) {
		lastLength = len(content)
	}
	return content[lastLength:], len(content)
}

func normalizeConfig(cfg Config) (Config, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return Config{}, errors.New("llm api_key is required")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = defaultLLMBaseURL
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = defaultLLMModel
	}
	return cfg, nil
}

func parseToolArgs(argsJSON string) map[string]interface{} {
	result := make(map[string]interface{})
	if argsJSON == "" {
		return result
	}

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		logging.Errorf("Failed to parse tool args: %v", err)
		return result
	}

	return args
}
