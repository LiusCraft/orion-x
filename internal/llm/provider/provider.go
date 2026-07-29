package provider

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/liuscraft/orion-x/internal/language"
	"github.com/liuscraft/orion-x/internal/llm"
)

type Adapter interface {
	llm.GenerationClient
}

type Config struct {
	Adapter         string
	APIKey          string
	BaseURL         string
	Model           string
	Scope           string
	Headers         map[string]string
	Options         []byte
	ExtraFields     map[string]any
	Thinking        llm.ThinkingConfig
	MaxOutputTokens int

	// Type is the deprecated name for Adapter.
	Type string
}

type AdapterConstructor func(context.Context, Config) (Adapter, error)

type ModelInfo struct {
	SupportedLanguages []language.Code
}

// ProviderMeta 是 LLM Provider 的静态元数据。
type ProviderMeta struct {
	Name           string
	DefaultBaseURL string
	Models         map[string]ModelInfo
	ContentHash    string // 由 Register() 自动计算：元数据的确定性哈希，用于变更检测
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
	meta.ContentHash = meta.MetaHash()
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
	providerType := strings.ToLower(strings.TrimSpace(cfg.Adapter))
	if providerType == "" {
		providerType = strings.ToLower(strings.TrimSpace(cfg.Type))
	}
	if providerType == "" {
		providerType = "openai-completions"
	}
	if providerType == "openai" {
		providerType = "openai-completions"
	}

	constructor, ok := registry.Get(providerType)
	if !ok {
		return nil, fmt.Errorf("unsupported llm provider: %s", providerType)
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
	stream, err := c.adapter.Stream(ctx, req)
	if err != nil {
		return nil, err
	}
	legacy := llm.NewStreamReader(func() { _ = stream.Close() })
	go func() {
		defer legacy.Close()
		for {
			event, recvErr := stream.Recv()
			if recvErr != nil {
				if recvErr != io.EOF {
					legacy.SendError(recvErr)
				}
				return
			}
			switch event.Type {
			case llm.EventTextDelta:
				legacy.Send(llm.Message{Role: string(llm.RoleAssistant), Content: event.TextDelta})
			case llm.EventResponseDone:
				if event.Response == nil {
					continue
				}
				calls := event.Response.Message.Calls()
				if len(calls) > 0 {
					legacy.Send(llm.Message{Role: string(llm.RoleAssistant), ToolCalls: calls})
				}
			}
		}
	}()
	return legacy, nil
}

func (c *Client) ChatSync(ctx context.Context, req llm.Request) (llm.Message, error) {
	resp, err := c.adapter.Generate(ctx, req)
	if err != nil {
		return llm.Message{}, err
	}
	msg := resp.Message
	msg.Content = msg.Text()
	msg.ToolCalls = msg.Calls()
	return msg, nil
}

func (c *Client) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	return c.adapter.Generate(ctx, req)
}

func (c *Client) Stream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	return c.adapter.Stream(ctx, req)
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

func NewGenerationClientWithDefault(ctx context.Context, cfg Config) (llm.GenerationClient, error) {
	client, err := NewClient(ctx, defaultRegistry, cfg)
	if err != nil {
		return nil, err
	}
	return client.(*Client), nil
}
