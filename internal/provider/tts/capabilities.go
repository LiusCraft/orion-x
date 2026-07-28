package tts

import (
	"context"

	"github.com/liuscraft/orion-x/internal/language"
)

// CapabilityProvider 允许 Manager 动态查询 provider 的能力。
// Manager 通过类型断言使用；不实现则回退到 ProviderMeta 静态声明。
type CapabilityProvider interface {
	Synthesizer
	GetCapabilities(ctx context.Context) (*Capabilities, error)
}

// Capabilities 是 provider 的完整能力声明。
type Capabilities struct {
	Models   []ModelCapability
	Features []Feature
}

// ModelCapability 是单个模型的能力描述。
type ModelCapability struct {
	Name                 string
	SupportedLanguages   []language.Code
	SupportedFormats     []AudioFormat
	SupportedSampleRates []int
	SupportedEmotions    []string
	Voices               []VoiceInfo
}

// Feature 是 provider 支持的能力标签。
type Feature string

const (
	FeatureStreaming    Feature = "streaming"
	FeatureSSML         Feature = "ssml"
	FeatureEmotion      Feature = "emotion"
	FeatureVoiceCloning Feature = "voice_cloning"
	FeatureWarmup       Feature = "warmup"
)
