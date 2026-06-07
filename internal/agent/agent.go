package agent

import (
	"context"
	"errors"

	"github.com/liuscraft/orion-x/internal/memory"
	llmfactory "github.com/liuscraft/orion-x/internal/provider/llm"
	"github.com/liuscraft/orion-x/internal/tools"
)

// Agent 负责 LLM 流式调用、工具调用和输出事件生成。
type Agent struct {
	chatModel   llmfactory.ChatModel
	toolManager tools.ToolManager
	memorySvc   memory.Service
}

func NewAgent(ctx context.Context, cfg Config, toolManager tools.ToolManager, memorySvc memory.Service) (*Agent, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	if toolManager == nil {
		return nil, errors.New("tool manager is required")
	}

	chatModel, err := llmfactory.NewChatModel(ctx, llmfactory.Config{
		Type:        normalized.Provider,
		BaseURL:     normalized.BaseURL,
		Model:       normalized.Model,
		APIKey:      normalized.APIKey,
		ExtraFields: normalized.ExtraFields,
	})
	if err != nil {
		return nil, err
	}

	if err := chatModel.BindTools(toolManager.ToolInfos()); err != nil {
		return nil, err
	}

	return &Agent{
		chatModel:   chatModel,
		toolManager: toolManager,
		memorySvc:   memorySvc,
	}, nil
}

// ToolInfo 工具信息
type ToolInfo struct {
	Name        string
	Description string
}
