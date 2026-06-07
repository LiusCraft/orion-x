package asr

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

var constructors = map[string]Constructor{}

func Register(providerType string, constructor Constructor) {
	providerType = normalizeType(providerType, "")
	if providerType == "" || constructor == nil {
		return
	}
	constructors[providerType] = constructor
}

func NewRecognizer(cfg ProviderConfig) (Recognizer, error) {
	providerType := normalizeType(cfg.Type, TypeAliyun)
	constructor, ok := constructors[providerType]
	if !ok {
		return nil, fmt.Errorf("unsupported asr provider: %s", cfg.Type)
	}
	return constructor(cfg.Config)
}

func normalizeType(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}
