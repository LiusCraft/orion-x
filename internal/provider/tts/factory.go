package tts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

const TypeAliyun = "aliyun"

type Config struct {
	APIKey               string
	Endpoint             string
	Workspace            string
	Model                string
	Voice                string
	Format               string
	SampleRate           int
	Volume               int
	Rate                 float64
	Pitch                float64
	EnableSSML           bool
	TextType             string
	EnableDataInspection *bool
}

type Provider interface {
	Start(ctx context.Context, cfg Config) (Stream, error)
}

type Stream interface {
	WriteTextChunk(ctx context.Context, text string) error
	// Finish 通知 TTS 服务文本已发送完毕，立即返回，不等待音频合成完成
	Finish(ctx context.Context) error
	Close(ctx context.Context) error
	AudioReader() io.ReadCloser
	SampleRate() int
	Channels() int
}

var (
	ErrTransient  = errors.New("tts transient error")
	ErrAuth       = errors.New("tts auth error")
	ErrBadRequest = errors.New("tts bad request")
)

type ProviderConfig struct {
	Type string
}

type Constructor func() Provider

var constructors = map[string]Constructor{}

func Register(providerType string, constructor Constructor) {
	providerType = normalizeType(providerType, "")
	if providerType == "" || constructor == nil {
		return
	}
	constructors[providerType] = constructor
}

func NewProvider(cfg ProviderConfig) (Provider, error) {
	providerType := normalizeType(cfg.Type, TypeAliyun)
	constructor, ok := constructors[providerType]
	if !ok {
		return nil, fmt.Errorf("unsupported tts provider: %s", cfg.Type)
	}
	return constructor(), nil
}

func normalizeType(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}
