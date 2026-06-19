package tts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

const TypeAliyun = "aliyun"

// SynthesisOptions 是每次合成时动态传入的参数。
// Emotion 是系统内部值（如 "happy"/"sad"），Provider 内部负责转换为
// 当前 voice 下可用的实际 emotion 参数（每个 voice 支持的 emotion 不同）。
type SynthesisOptions struct {
	Emotion string  // 系统内部情感值，空 = 用 Provider 默认
	Rate    float64 // 语速，0 = 用 Provider 默认
}

// Config 是创建 Provider 时注入的基础配置（连接参数 + 默认合成参数）。
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

// Provider 是 TTS 服务的抽象。基础配置在创建时注入，每次合成只传动态参数。
type Provider interface {
	Synthesize(ctx context.Context, text string, opts SynthesisOptions) (io.ReadCloser, error)
}

var (
	ErrTransient  = errors.New("tts transient error")
	ErrAuth       = errors.New("tts auth error")
	ErrBadRequest = errors.New("tts bad request")
)

type ProviderConfig struct {
	Type   string
	Config Config
}

type Constructor func(cfg Config) (Provider, error)

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
	return constructor(cfg.Config)
}

func normalizeType(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}
