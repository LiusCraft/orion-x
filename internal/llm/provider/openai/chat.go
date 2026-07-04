package openai

import (
	"context"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/llm/provider"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
)

type Config struct {
	APIKey      string
	BaseURL     string
	Model       string
	ExtraFields map[string]any
}

type chatAdapter struct {
	client      openai.Client
	cfg         Config
	extraFields map[string]any
}

func init() {
	provider.Register("openai", func(ctx context.Context, cfg provider.Config) (provider.Adapter, error) {
		return New(ctx, Config{
			APIKey:      cfg.APIKey,
			BaseURL:     cfg.BaseURL,
			Model:       cfg.Model,
			ExtraFields: cfg.ExtraFields,
		})
	}, provider.ProviderMeta{
		Name:           "OpenAI",
		DefaultBaseURL: "https://api.openai.com",
	})
}

func New(ctx context.Context, cfg Config) (provider.Adapter, error) {
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return &chatAdapter{
		client:      openai.NewClient(opts...),
		cfg:         cfg,
		extraFields: cfg.ExtraFields,
	}, nil
}

func (c *chatAdapter) Chat(ctx context.Context, req llm.Request) (*llm.StreamReader, error) {
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(c.cfg.Model),
		Messages: convertMessages(req.Messages),
		Tools:    convertTools(req.Tools),
	}
	for k, v := range c.extraFields {
		params.SetExtraFields(map[string]any{k: v})
	}

	ctx, cancel := context.WithCancel(ctx)
	stream := c.client.Chat.Completions.NewStreaming(ctx, params)
	sr := llm.NewStreamReader(cancel)

	go func() {
		defer sr.Close()
		acc := openai.ChatCompletionAccumulator{}

		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				if delta.Content != "" {
					sr.Send(llm.Message{Role: "assistant", Content: delta.Content})
				}
			}
		}

		if err := stream.Err(); err != nil {
			sr.SendError(err)
			return
		}

		if len(acc.Choices) > 0 && len(acc.Choices[0].Message.ToolCalls) > 0 {
			toolCalls := convertToolCalls(acc.Choices[0].Message.ToolCalls)
			sr.Send(llm.Message{Role: "assistant", ToolCalls: toolCalls})
		}
	}()

	return sr, nil
}

func (c *chatAdapter) ChatSync(ctx context.Context, req llm.Request) (llm.Message, error) {
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(c.cfg.Model),
		Messages: convertMessages(req.Messages),
	}
	for k, v := range c.extraFields {
		params.SetExtraFields(map[string]any{k: v})
	}

	completion, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return llm.Message{}, err
	}

	if len(completion.Choices) == 0 {
		return llm.Message{Role: "assistant"}, nil
	}

	choice := completion.Choices[0]
	return llm.Message{
		Role:      "assistant",
		Content:   choice.Message.Content,
		ToolCalls: convertToolCalls(choice.Message.ToolCalls),
	}, nil
}

func convertMessages(messages []llm.Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case "system":
			result[i] = openai.SystemMessage(msg.Content)
		case "user":
			result[i] = openai.UserMessage(msg.Content)
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				assistant := openai.ChatCompletionAssistantMessageParam{}
				if msg.Content != "" {
					assistant.Content.OfString = param.NewOpt(msg.Content)
				}
				assistant.ToolCalls = make([]openai.ChatCompletionMessageToolCallUnionParam, len(msg.ToolCalls))
				for j, tc := range msg.ToolCalls {
					assistant.ToolCalls[j] = openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: tc.ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Name,
								Arguments: tc.Arguments,
							},
						},
					}
				}
				result[i] = openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant}
			} else {
				result[i] = openai.AssistantMessage(msg.Content)
			}
		case "tool":
			result[i] = openai.ToolMessage(msg.Content, msg.ToolCallID)
		}
	}
	return result
}

func convertTools(defs []llm.ToolDefinition) []openai.ChatCompletionToolUnionParam {
	if len(defs) == 0 {
		return nil
	}
	result := make([]openai.ChatCompletionToolUnionParam, len(defs))
	for i, def := range defs {
		result[i] = openai.ChatCompletionToolUnionParam{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: openai.FunctionDefinitionParam{
					Name:        def.Name,
					Description: openai.String(def.Description),
					Parameters:  openai.FunctionParameters(def.Parameters),
				},
			},
		}
	}
	return result
}

func convertToolCalls(toolCalls []openai.ChatCompletionMessageToolCallUnion) []llm.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	result := make([]llm.ToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		result[i] = llm.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		}
	}
	return result
}
