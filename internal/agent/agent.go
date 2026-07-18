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
	client      llm.Client
	registry    *tools.Registry
	model       string
	memorySvc   *memory.Service
	maxSteps    int
	soulPrompt  string
	rulesPrompt string
	currentLang string // language code currently reflected in system prompt
}

// RegisterBuiltinTool adds a tool spec to the agent's registry.
// Builtin tools are always available regardless of MCP configuration.
func (a *Agent) RegisterBuiltinTool(spec tools.Spec) {
	a.registry.Add(spec)
}

func New(ctx context.Context, cfg Config, mgr *tools.Manager, memorySvc *memory.Service) (*Agent, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
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

	return newWithClientForConfig(client, normalized, mgr, memorySvc)
}

// NewWithClient creates an Agent with a shared LLM client and isolated tools
// and memory services.
func NewWithClient(cfg Config, client llm.Client, mgr *tools.Manager, memorySvc *memory.Service) (*Agent, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	return newWithClientForConfig(client, normalized, mgr, memorySvc)
}

func newWithClientForConfig(client llm.Client, cfg Config, mgr *tools.Manager, memorySvc *memory.Service) (*Agent, error) {
	if client == nil {
		return nil, errors.New("llm client is required")
	}
	if mgr == nil {
		return nil, errors.New("tool manager is required")
	}
	return newWithClient(client, mgr.Registry(), cfg.Model, memorySvc, cfg.SoulPrompt, cfg.RulesPrompt), nil
}

// newWithClient 使用已构造好的 llm.Client 组装 Agent，供测试注入 fake client。
func newWithClient(client llm.Client, registry *tools.Registry, model string, memorySvc *memory.Service, soulPrompt, rulesPrompt string) *Agent {
	return &Agent{
		client:      client,
		registry:    registry,
		model:       model,
		memorySvc:   memorySvc,
		maxSteps:    10,
		soulPrompt:  soulPrompt,
		rulesPrompt: rulesPrompt,
	}
}

func (a *Agent) SetMaxSteps(n int) {
	if n > 0 {
		a.maxSteps = n
	}
}

// SetLanguage sets the language code for prompt adaptation.
// When lang is empty or unchanged, it is a no-op.
func (a *Agent) SetLanguage(lang string) {
	if lang == "" || lang == a.currentLang {
		return
	}
	a.currentLang = lang
}
