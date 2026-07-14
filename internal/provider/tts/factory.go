package tts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/liuscraft/orion-x/internal/language"
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

// SentenceBoundary marks a point in a SynthesisStream's audio output.
// For sentence-end (IsBegin=false): Offset is the cumulative byte count
// AudioReader has produced once that sentence's audio is fully available,
// and Text is the sentence's full original text.
// For sentence-begin (IsBegin=true): Offset is ignored (set to -1 by
// convention) — the boundary signals that the next audio chunk from
// AudioReader belongs to a sentence whose text is Text.
type SentenceBoundary struct {
	Offset  int
	Text    string
	IsBegin bool
}

// SynthesisStream 是一次 TTS 会话的流式接口。
// 调用方顺序：WriteTextChunk → Finish → 读取 AudioReader()。
type SynthesisStream interface {
	WriteTextChunk(ctx context.Context, text string) error
	// Finish 发送 finish-task，立即返回，不等 task-finished。
	// receiver goroutine 负责在 task-finished 后关闭 audioBuf 和 conn。
	Finish(ctx context.Context) error
	// AudioReader 返回流式音频 reader，可在 Finish 前开始读；task-finished 后 EOF。
	AudioReader() io.ReadCloser
	// Abort 中止 stream，关闭连接和 audioBuf（打断场景）。
	Abort()
	// SentenceBoundaries returns a channel that receives one SentenceBoundary
	// per sentence once that sentence's audio has been fully written to
	// AudioReader. Implementations that can't detect sentence boundaries may
	// return nil — receiving from a nil channel blocks forever, so callers
	// selecting on it alongside other cases simply never see that case fire,
	// with no extra nil-checking needed.
	SentenceBoundaries() <-chan SentenceBoundary
}

// StreamingProvider 是支持流式合成的 Provider 扩展接口。
// ttsProcessor 通过类型断言检测并使用此接口；不实现则回退到 Synthesize。
type StreamingProvider interface {
	Provider
	StartSynthesis(ctx context.Context, opts SynthesisOptions) (SynthesisStream, error)
}

// WarmableProvider 是支持预连接预热的 Provider 扩展接口。
// Warm 由 TTSProcessor 在每轮第一个 token 到达时在 goroutine 里调用，
// 阻塞直到连接就绪（或 ctx 取消），返回可立即写文本的 stream。
// 取消预热只需取消传入的 ctx，无需额外接口。
type WarmableProvider interface {
	Warm(ctx context.Context, opts SynthesisOptions) SynthesisStream
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

// ListRegistered returns all registered TTS provider types with their metadata.
func ListRegistered() map[string]ProviderMeta {
	out := make(map[string]ProviderMeta, len(constructors))
	for k, v := range constructors {
		out[k] = v.meta
	}
	return out
}

func NewProvider(cfg ProviderConfig) (Provider, error) {
	providerType := normalizeType(cfg.Type, TypeAliyun)
	reg, ok := constructors[providerType]
	if !ok {
		return nil, fmt.Errorf("unsupported tts provider: %s", cfg.Type)
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
