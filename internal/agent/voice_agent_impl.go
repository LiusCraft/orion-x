package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/tools"
)

type voiceAgentImpl struct {
	chatModel         *openai.ChatModel
	emotionExtractor  EmotionExtractor
	markdownFilter    MarkdownFilter
	actionResponseGen *ActionResponseGenerator
	toolManager       tools.ToolManager
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
	return NewVoiceAgentWithConfig(ctx, tools.ManagerConfig{
		APIKey: key,
	})
}

func NewVoiceAgentWithConfig(ctx context.Context, cfg tools.ManagerConfig) (VoiceAgent, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}

	toolManager, err := tools.NewToolManager(ctx, normalized)
	if err != nil {
		return nil, err
	}

	agent, err := NewVoiceAgentWithToolManager(ctx, normalized, toolManager)
	if err != nil {
		_ = toolManager.Close()
		return nil, err
	}

	return agent, nil
}

func NewVoiceAgentWithToolManager(ctx context.Context, cfg tools.ManagerConfig, toolManager tools.ToolManager) (VoiceAgent, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	if toolManager == nil {
		return nil, errors.New("tool manager is required")
	}

	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: normalized.BaseURL,
		Model:   normalized.Model,
		APIKey:  normalized.APIKey,
	})
	if err != nil {
		return nil, err
	}

	if err := chatModel.BindTools(toolManager.ToolInfos()); err != nil {
		return nil, err
	}

	responseGen := NewActionResponseGeneratorWithToolManager(toolManager)

	return &voiceAgentImpl{
		chatModel:         chatModel,
		emotionExtractor:  NewEmotionExtractor(),
		markdownFilter:    NewMarkdownFilter(),
		actionResponseGen: responseGen,
		toolManager:       toolManager,
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

当用户请求需要外部能力时，请调用合适的工具。
工具名称必须与提供的工具列表完全一致。`),
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

				newContent, nextLength := deltaFromBufferedContent(bufferedContent, lastFilteredLength)
				if newContent != "" {
					eventChan <- &TextChunkEvent{Chunk: newContent, Emotion: currentEmotion}
					fullText += newContent
				}
				lastFilteredLength = nextLength
			}

			for _, toolCall := range msg.ToolCalls {
				if v.toolManager != nil && !v.toolManager.Has(toolCall.Function.Name) {
					err := fmt.Errorf("unknown tool: %s", toolCall.Function.Name)
					logging.Errorf("VoiceAgent: %v", err)
					eventChan <- &FinishedEvent{Error: err}
					return
				}
				toolType := v.toolManager.GetToolType(toolCall.Function.Name)
				args := parseToolArgs(toolCall.Function.Arguments)

				logging.Infof("VoiceAgent: tool call requested: %s (type: %s), args: %v", toolCall.Function.Name, toolType, args)
				eventChan <- &ToolCallRequestedEvent{
					Tool:     toolCall.Function.Name,
					Args:     args,
					ToolType: toAgentToolType(toolType),
				}

				if toolType == tools.ToolTypeAction {
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

func (v *voiceAgentImpl) SummarizeToolResult(ctx context.Context, tool string, args map[string]interface{}, result interface{}) (<-chan AgentEvent, error) {
	if v.chatModel == nil {
		return nil, errors.New("llm chat model is not initialized")
	}

	eventChan := make(chan AgentEvent)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(eventChan)

		messages := buildToolSummaryMessages(tool, args, result)

		logging.Infof("VoiceAgent: starting tool summary stream (tool=%s)...", tool)
		stream, err := v.chatModel.Stream(ctx, messages)
		if err != nil {
			logging.Errorf("VoiceAgent: tool summary stream error: %v", err)
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
				logging.Infof("VoiceAgent: tool summary completed, total text length: %d", len(fullText))
				break
			}
			if err != nil {
				logging.Errorf("VoiceAgent: tool summary receive error: %v", err)
				eventChan <- &FinishedEvent{Error: err}
				return
			}

			if len(msg.ToolCalls) > 0 {
				err := fmt.Errorf("tool summary should not call tools: %s", msg.ToolCalls[0].Function.Name)
				logging.Errorf("VoiceAgent: %v", err)
				eventChan <- &FinishedEvent{Error: err}
				return
			}

			if msg.Content != "" {
				bufferedContent += msg.Content
				cleanBufferedContent := bufferedContent

				newContent, nextLength := deltaFromBufferedContent(cleanBufferedContent, lastFilteredLength)
				if newContent != "" {
					eventChan <- &TextChunkEvent{Chunk: newContent, Emotion: currentEmotion}
					fullText += newContent
				}
				lastFilteredLength = nextLength
			}
		}

		eventChan <- &FinishedEvent{Error: nil}
	}()

	return eventChan, nil
}

func (v *voiceAgentImpl) GetToolType(tool string) ToolType {
	return toAgentToolType(v.toolManager.GetToolType(tool))
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

func normalizeConfig(cfg tools.ManagerConfig) (tools.ManagerConfig, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return tools.ManagerConfig{}, errors.New("llm api_key is required")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = defaultLLMBaseURL
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = defaultLLMModel
	}
	if cfg.ToolTypes == nil {
		cfg.ToolTypes = make(map[string]string)
	}
	if cfg.ActionResponses == nil {
		cfg.ActionResponses = make(map[string]string)
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

func buildToolSummaryMessages(tool string, args map[string]interface{}, result interface{}) []*schema.Message {
	argsText := stringifyJSONOrValue(args)
	resultText := stringifyJSONOrValue(result)

	userContent := fmt.Sprintf("工具: %s\n参数: %s\n结果: %s\n请用简洁中文向用户总结结果，必要时分点说明，不要调用工具。", tool, argsText, resultText)

	return []*schema.Message{
		schema.SystemMessage(`你是一个语音助手，请根据工具结果生成对用户可听的简洁回复。
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

func toAgentToolType(tt tools.ToolType) ToolType {
	if tt == tools.ToolTypeAction {
		return ToolTypeAction
	}
	return ToolTypeQuery
}
