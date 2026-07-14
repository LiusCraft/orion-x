package asr

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/liuscraft/orion-x/internal/language"
)

const TypeAliyun = "aliyun"

var ErrAPIKeyRequired = errors.New("DASHSCOPE_API_KEY is required")

type Config struct {
	APIKey                     string
	Endpoint                   string
	Model                      string
	Format                     string
	SampleRate                 int
	VocabularyID               string
	SemanticPunctuationEnabled *bool
	MaxSentenceSilence         int
	MultiThresholdModeEnabled  *bool
	Heartbeat                  *bool
	LanguageHints              []string
}

type Result struct {
	Text          string
	IsFinal       bool
	BeginTimeMs   int64
	EndTimeMs     *int64
	UsageDuration *int
}

type Recognizer interface {
	Start(ctx context.Context) error
	SendAudio(ctx context.Context, data []byte) error
	Finish(ctx context.Context) error
	Close() error
	OnResult(handler func(Result))
}

type ProviderConfig struct {
	Type   string
	Config Config
}

type Constructor func(Config) (Recognizer, error)

type ModelInfo struct {
	SupportedLanguages []language.Code
}

type ProviderMeta struct {
	Name           string
	DefaultBaseURL string
	Models         map[string]ModelInfo
}

type registration struct {
	constructor Constructor
	meta        ProviderMeta
}

var constructors = map[string]registration{}

func Register(providerType string, constructor Constructor, meta ProviderMeta) {
	providerType = normalizeType(providerType, "")
	if providerType == "" || constructor == nil {
		return
	}
	constructors[providerType] = registration{constructor: constructor, meta: meta}
}

// ListRegistered returns all registered ASR provider types with their metadata.
func ListRegistered() map[string]ProviderMeta {
	out := make(map[string]ProviderMeta, len(constructors))
	for k, v := range constructors {
		out[k] = v.meta
	}
	return out
}

func NewRecognizer(cfg ProviderConfig) (Recognizer, error) {
	providerType := normalizeType(cfg.Type, TypeAliyun)
	reg, ok := constructors[providerType]
	if !ok {
		return nil, fmt.Errorf("unsupported asr provider: %s", cfg.Type)
	}
	return reg.constructor(cfg.Config)
}

func SupportsLanguage(providerType, model string, lang language.Code) bool {
	reg, ok := constructors[normalizeType(providerType, "")]
	if !ok {
		return true // provider not registered → no restriction
	}
	info, ok := reg.meta.Models[model]
	if !ok {
		return true // model not declared → no restriction
	}
	if len(info.SupportedLanguages) == 0 {
		return true // empty list → all languages
	}
	for _, s := range info.SupportedLanguages {
		if s == lang {
			return true
		}
	}
	return false
}

func normalizeType(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}
