package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const TypeOpenAI = "openai"

type Config struct {
	Type        string
	APIKey      string
	BaseURL     string
	Model       string
	ExtraFields map[string]any
}

type ChatModel interface {
	BindTools(tools []*schema.ToolInfo) error
	Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error)
	Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error)
}

type Constructor func(context.Context, Config) (ChatModel, error)

var constructors = map[string]Constructor{}

func Register(providerType string, constructor Constructor) {
	providerType = normalizeType(providerType, "")
	if providerType == "" || constructor == nil {
		return
	}
	constructors[providerType] = constructor
}

func NewChatModel(ctx context.Context, cfg Config) (ChatModel, error) {
	providerType := normalizeType(cfg.Type, TypeOpenAI)
	constructor, ok := constructors[providerType]
	if !ok {
		return nil, fmt.Errorf("unsupported llm provider: %s", cfg.Type)
	}
	return constructor(ctx, cfg)
}

func normalizeType(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}
