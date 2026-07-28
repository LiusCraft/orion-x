package tts

import (
	"errors"
	"fmt"
	"strings"

	"github.com/liuscraft/orion-x/internal/language"
)

const TypeAliyun = "aliyun"

// ── 兼容类型别名 ──

// Provider 是旧的 Provider 接口类型别名，兼容旧代码。
// 新代码应使用 Synthesizer。
type Provider = Synthesizer

// StreamingProvider 是旧的流式接口类型别名，兼容旧代码。
// 新代码应使用 StreamingSynthesizer。
type StreamingProvider = StreamingSynthesizer

// ── 兼容的 SynthesisOptions ──

// SynthesisOptions 是每次合成时动态传入的参数（已废弃，请使用 SynthesizeRequest）。
// Emotion 是系统内部值（如 emoji 或 "happy"/"sad"），Provider 内部负责转换为
// 当前 voice 下可用的实际 emotion 参数。
type SynthesisOptions struct {
	Emotion string  // 系统内部情感值，空 = 用 Provider 默认
	Rate    float64 // 语速，0 = 用 Provider 默认
}

// ── Config ──

// Config 是创建 Provider 时注入的基础配置（连接参数 + 默认合成参数）。
// Provider 专属参数放入 Extra map，公共层不做校验。
type Config struct {
	APIKey     string         // API 密钥
	Endpoint   string         // 服务端点
	Model      string         // 默认模型
	Voice      string         // 默认音色
	SampleRate int            // 默认采样率（Hz）
	Extra      map[string]any // provider 专属参数（workspace、data_inspection 等）
}

// ── Provider 注册 ──

// ProviderConfig 是带类型标签的 Provider 配置。
type ProviderConfig struct {
	Type   string
	Config Config
}

// Constructor 是 Provider 的构造器函数。
// 返回 Synthesizer 以适配新旧代码。
type Constructor func(cfg Config) (Synthesizer, error)

// ModelInfo 是单个模型的元信息。
type ModelInfo struct {
	SupportedLanguages []language.Code
	SystemVoices       []VoiceInfo
}

// ProviderMeta 是 Provider 的静态元数据，供 Manager 展示和音色同步。
type ProviderMeta struct {
	Name           string
	Description    string // 新增：Provider 描述
	DefaultBaseURL string
	Models         map[string]ModelInfo
	Features       []Feature // 新增：streaming / ssml / emotion / voice_cloning / warmup
}

type registration struct {
	constructor Constructor
	meta        ProviderMeta
}

var constructors = map[string]registration{}

// Register 注册一个 TTS Provider 构造器。
func Register(providerType string, constructor Constructor, meta ProviderMeta) {
	providerType = normalizeType(providerType, "")
	if providerType == "" || constructor == nil {
		return
	}
	constructors[providerType] = registration{constructor: constructor, meta: meta}
}

// ListRegistered 返回所有已注册的 TTS Provider 及其元数据。
func ListRegistered() map[string]ProviderMeta {
	out := make(map[string]ProviderMeta, len(constructors))
	for k, v := range constructors {
		out[k] = v.meta
	}
	return out
}

// NewProvider 创建一个新的 TTS Provider 实例。
func NewProvider(cfg ProviderConfig) (Synthesizer, error) {
	providerType := normalizeType(cfg.Type, TypeAliyun)
	reg, ok := constructors[providerType]
	if !ok {
		return nil, fmt.Errorf("unsupported tts provider: %s", cfg.Type)
	}
	return reg.constructor(cfg.Config)
}

// SupportsLanguage 检查指定 provider 的 model 是否支持目标语言。
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

// ── 错误哨兵 ──

var (
	ErrTransient  = errors.New("tts transient error")
	ErrAuth       = errors.New("tts auth error")
	ErrBadRequest = errors.New("tts bad request")
)

// APIError 是 Provider 返回的结构化错误。
type APIError struct {
	Provider   string
	StatusCode int
	Code       string
	Message    string
	RequestID  string
	Retryable  bool
	Cause      error
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("[%s] %s", e.Provider, e.Message)
	if e.Code != "" {
		msg = fmt.Sprintf("[%s] code=%s: %s", e.Provider, e.Code, e.Message)
	}
	if e.RequestID != "" {
		msg += fmt.Sprintf(" (request_id=%s)", e.RequestID)
	}
	return msg
}

func (e *APIError) Unwrap() error {
	return e.Cause
}

// ── 兼容方法 ──

// SynthesizeRequestFromOpts 将旧的 SynthesisOptions 转换为 SynthesizeRequest。
// 用于向后兼容旧代码，将旧的 opts 与文本合并为基础请求。
func SynthesizeRequestFromOpts(text string, opts SynthesisOptions) SynthesizeRequest {
	return SynthesizeRequest{
		Input:  TextInput{Text: text},
		Speech: SpeechParams{Emotion: opts.Emotion, Speed: opts.Rate},
	}
}
