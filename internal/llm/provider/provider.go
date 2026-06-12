package provider

import (
	"context"
	"fmt"
	"strings"

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

type Registry struct {
	adapters map[string]AdapterConstructor
}

func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]AdapterConstructor)}
}

func (r *Registry) Register(key string, constructor AdapterConstructor) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" || constructor == nil {
		return
	}
	r.adapters[key] = constructor
}

func (r *Registry) Get(key string) (AdapterConstructor, bool) {
	c, ok := r.adapters[key]
	return c, ok
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

func Register(key string, constructor AdapterConstructor) {
	defaultRegistry.Register(key, constructor)
}

func NewClientWithDefault(ctx context.Context, cfg Config) (llm.Client, error) {
	return NewClient(ctx, defaultRegistry, cfg)
}
