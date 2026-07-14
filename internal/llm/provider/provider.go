package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/liuscraft/orion-x/internal/language"
	"github.com/liuscraft/orion-x/internal/llm"
)

type Adapter interface {
	Chat(ctx context.Context, req llm.Request) (*llm.StreamReader, error)
	ChatSync(ctx context.Context, req llm.Request) (llm.Message, error)
}

type Config struct {
	Type        string
	APIKey      string
	BaseURL     string
	Model       string
	ExtraFields map[string]any
}

type AdapterConstructor func(context.Context, Config) (Adapter, error)

type ModelInfo struct {
	SupportedLanguages []language.Code
}

type ProviderMeta struct {
	Name           string
	DefaultBaseURL string
	Models         map[string]ModelInfo
}

type registration struct {
	constructor AdapterConstructor
	meta        ProviderMeta
}

type Registry struct {
	adapters map[string]registration
}

func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]registration)}
}

func (r *Registry) Register(key string, constructor AdapterConstructor, meta ProviderMeta) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" || constructor == nil {
		return
	}
	r.adapters[key] = registration{constructor: constructor, meta: meta}
}

func (r *Registry) Get(key string) (AdapterConstructor, bool) {
	reg, ok := r.adapters[key]
	return reg.constructor, ok
}

// ListRegistered returns all registered LLM provider types with their metadata.
func (r *Registry) ListRegistered() map[string]ProviderMeta {
	out := make(map[string]ProviderMeta, len(r.adapters))
	for k, v := range r.adapters {
		out[k] = v.meta
	}
	return out
}

func (r *Registry) SupportsLanguage(providerType, model string, lang language.Code) bool {
	reg, ok := r.adapters[strings.ToLower(strings.TrimSpace(providerType))]
	if !ok {
		return false
	}
	info, ok := reg.meta.Models[model]
	if !ok {
		return true
	}
	if len(info.SupportedLanguages) == 0 {
		return true
	}
	for _, s := range info.SupportedLanguages {
		if s == lang {
			return true
		}
	}
	return false
}

type Client struct {
	registry *Registry
	cfg      Config
	adapter  Adapter
}

func NewClient(ctx context.Context, registry *Registry, cfg Config) (llm.Client, error) {
	providerType := strings.ToLower(strings.TrimSpace(cfg.Type))
	if providerType == "" {
		providerType = "openai"
	}

	constructor, ok := registry.Get(providerType)
	if !ok {
		return nil, fmt.Errorf("unsupported llm provider: %s", cfg.Type)
	}

	adapter, err := constructor(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &Client{
		registry: registry,
		cfg:      cfg,
		adapter:  adapter,
	}, nil
}

func (c *Client) Chat(ctx context.Context, req llm.Request) (*llm.StreamReader, error) {
	return c.adapter.Chat(ctx, req)
}

func (c *Client) ChatSync(ctx context.Context, req llm.Request) (llm.Message, error) {
	return c.adapter.ChatSync(ctx, req)
}

var defaultRegistry = NewRegistry()

func DefaultRegistry() *Registry {
	return defaultRegistry
}

func Register(key string, constructor AdapterConstructor, meta ProviderMeta) {
	defaultRegistry.Register(key, constructor, meta)
}

func NewClientWithDefault(ctx context.Context, cfg Config) (llm.Client, error) {
	return NewClient(ctx, defaultRegistry, cfg)
}
