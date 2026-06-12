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
	client    llm.Client
	registry  *tools.Registry
	model     string
	memorySvc memory.Service
	maxSteps  int
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

	return &Agent{
		client:    client,
		registry:  mgr.Registry(),
		model:     normalized.Model,
		memorySvc: memorySvc,
		maxSteps:  2,
	}, nil
}

func (a *Agent) SetMaxSteps(n int) {
	if n > 0 {
		a.maxSteps = n
	}
}
