package agent

import (
	"context"
	"errors"

	"github.com/liuscraft/orion-x/internal/llm"
	llmprovider "github.com/liuscraft/orion-x/internal/llm/provider"
	"github.com/liuscraft/orion-x/internal/memory"
	"github.com/liuscraft/orion-x/internal/tools"
)

type Agent struct {
	client       llm.Client
	registry     *tools.Registry
	model        string
	memorySvc    memory.Service
	maxSteps     int
	systemPrompt string
}

func New(ctx context.Context, cfg Config, mgr *tools.Manager, memorySvc memory.Service) (*Agent, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	if mgr == nil {
		return nil, errors.New("tool manager is required")
	}

	client, err := llmprovider.NewClientWithDefault(ctx, llmprovider.Config{
		Type:        normalized.Provider,
		BaseURL:     normalized.BaseURL,
		Model:       normalized.Model,
		APIKey:      normalized.APIKey,
		ExtraFields: normalized.ExtraFields,
	})
	if err != nil {
		return nil, err
	}

	return newWithClient(client, mgr.Registry(), normalized.Model, memorySvc, normalized.SystemPrompt), nil
}

// newWithClient 使用已构造好的 llm.Client 组装 Agent，供测试注入 fake client。
func newWithClient(client llm.Client, registry *tools.Registry, model string, memorySvc memory.Service, systemPrompt string) *Agent {
	return &Agent{
		client:       client,
		registry:     registry,
		model:        model,
		memorySvc:    memorySvc,
		maxSteps:     10,
		systemPrompt: systemPrompt,
	}
}

func (a *Agent) SetMaxSteps(n int) {
	if n > 0 {
		a.maxSteps = n
	}
}
