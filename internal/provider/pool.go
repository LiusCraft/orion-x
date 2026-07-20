package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/llm"
	llmprovider "github.com/liuscraft/orion-x/internal/llm/provider"
	"github.com/liuscraft/orion-x/internal/provider/asr"
	"github.com/liuscraft/orion-x/internal/provider/tts"
)

// Pool caches reusable TTS providers. ASR recognizers represent a single
// streaming recognition session and are therefore deliberately created for
// every caller; pooling those instances would mix concurrent audio streams.
type Pool struct {
	mu           sync.Mutex
	llmClients   map[string]llm.Client
	ttsProviders map[string]tts.Provider
}

func NewPool() *Pool {
	return &Pool{llmClients: make(map[string]llm.Client), ttsProviders: make(map[string]tts.Provider)}
}

func (p *Pool) GetOrCreateLLM(ctx context.Context, providerType string, cfg config.LLMConfig) (llm.Client, error) {
	if p == nil {
		return nil, fmt.Errorf("provider pool is nil")
	}
	keyBytes, err := json.Marshal(struct {
		ProviderType string           `json:"provider_type"`
		Config       config.LLMConfig `json:"config"`
	}{ProviderType: providerType, Config: cfg})
	if err != nil {
		return nil, fmt.Errorf("encode LLM client key: %w", err)
	}
	key := string(keyBytes)
	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, ok := p.llmClients[key]; ok {
		return existing, nil
	}
	created, err := llmprovider.NewClientWithDefault(ctx, llmprovider.Config{
		Adapter: providerType, Type: providerType, APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, Model: cfg.Model,
		Options: cfg.Options, ExtraFields: cfg.ExtraFields, Thinking: cfg.Thinking,
		MaxOutputTokens: cfg.MaxOutputTokens,
	})
	if err != nil {
		return nil, err
	}
	p.llmClients[key] = created
	return created, nil
}

func (p *Pool) GetOrCreateASR(providerType string, cfg config.ASRConfig) (asr.Recognizer, error) {
	return asr.NewRecognizer(asr.ProviderConfig{Type: providerType, Config: asr.Config{
		APIKey: cfg.APIKey, Endpoint: cfg.Endpoint, Model: cfg.Model,
		Format: "pcm", SampleRate: audio.InternalSampleRate,
	}})
}

func (p *Pool) GetOrCreateTTS(providerType string, cfg config.TTSConfig, sampleRate int) (tts.Provider, error) {
	if p == nil {
		return nil, fmt.Errorf("provider pool is nil")
	}
	if sampleRate <= 0 {
		sampleRate = audio.InternalSampleRate
	}
	// The sample rate is part of a provider's immutable synthesis configuration.
	keyCfg := cfg
	keyCfg.SampleRate = sampleRate
	keyBytes, err := json.Marshal(struct {
		ProviderType string           `json:"provider_type"`
		Config       config.TTSConfig `json:"config"`
	}{ProviderType: providerType, Config: keyCfg})
	if err != nil {
		return nil, fmt.Errorf("encode TTS provider key: %w", err)
	}
	key := string(keyBytes)
	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, ok := p.ttsProviders[key]; ok {
		return existing, nil
	}
	created, err := tts.NewProvider(tts.ProviderConfig{Type: providerType, Config: tts.Config{
		APIKey: cfg.APIKey, Endpoint: cfg.Endpoint, Workspace: cfg.Workspace,
		Model: cfg.Model, Voice: cfg.Voice, Format: "pcm", SampleRate: sampleRate,
		Volume: cfg.Volume, Rate: cfg.Rate, Pitch: cfg.Pitch, EnableSSML: cfg.EnableSSML,
		TextType: cfg.TextType, EnableDataInspection: cfg.EnableDataInspection,
	}})
	if err != nil {
		return nil, err
	}
	p.ttsProviders[key] = created
	return created, nil
}
