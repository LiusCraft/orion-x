package openai

import (
	"context"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	llm "github.com/liuscraft/orion-x/internal/provider/llm"
)

type Config struct {
	APIKey      string
	BaseURL     string
	Model       string
	ExtraFields map[string]any
}

func init() {
	llm.Register(llm.TypeOpenAI, func(ctx context.Context, cfg llm.Config) (llm.ChatModel, error) {
		return NewChatModel(ctx, Config{
			APIKey:      cfg.APIKey,
			BaseURL:     cfg.BaseURL,
			Model:       cfg.Model,
			ExtraFields: cfg.ExtraFields,
		})
	})
}

func NewChatModel(ctx context.Context, cfg Config) (*einoopenai.ChatModel, error) {
	return einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
		BaseURL:     cfg.BaseURL,
		Model:       cfg.Model,
		APIKey:      cfg.APIKey,
		ExtraFields: cfg.ExtraFields,
	})
}
