package audio

import (
	"context"

	"github.com/liuscraft/orion-x/internal/asr"
)

// AudioInPipe 音频输入管道，负责音频输入管理和ASR调用
type AudioInPipe interface {
	Start(ctx context.Context) error
	Stop() error
	SendAudio(audio []byte) error
	OnASRResult(handler func(text string, isFinal bool))
	OnUserSpeakingDetected(handler func())
}

// AudioSource 音频输入源接口
type AudioSource interface {
	Read(ctx context.Context) ([]byte, error)
	Close() error
}

// InPipeConfig InPipe配置
type InPipeConfig struct {
	SampleRate   int
	Channels     int
	EnableVAD    bool
	VADThreshold float64
	ASRModel     string
	ASREndpoint  string
}

// DefaultInPipeConfig 默认配置
func DefaultInPipeConfig() *InPipeConfig {
	return &InPipeConfig{
		SampleRate:   16000,
		Channels:     1,
		EnableVAD:    true,
		VADThreshold: 0.5,
		ASRModel:     "fun-asr-realtime",
	}
}

// NewInPipe 创建新的AudioInPipe
func NewInPipe(apiKey string, config *InPipeConfig) (AudioInPipe, error) {
	if config == nil {
		config = DefaultInPipeConfig()
	}

	asrCfg := asr.Config{
		APIKey:     apiKey,
		Model:      config.ASRModel,
		Endpoint:   config.ASREndpoint,
		Format:     "pcm",
		SampleRate: config.SampleRate,
	}

	recognizer, err := asr.NewDashScopeRecognizer(asrCfg)
	if err != nil {
		return nil, err
	}

	return NewInPipeWithRecognizer(config, recognizer), nil
}

// NewInPipeWithAudioSource 创建带有音频源的AudioInPipe
func NewInPipeWithAudioSource(apiKey string, config *InPipeConfig, source AudioSource) (AudioInPipe, error) {
	if config == nil {
		config = DefaultInPipeConfig()
	}

	asrCfg := asr.Config{
		APIKey:     apiKey,
		Model:      config.ASRModel,
		Endpoint:   config.ASREndpoint,
		Format:     "pcm",
		SampleRate: config.SampleRate,
	}

	recognizer, err := asr.NewDashScopeRecognizer(asrCfg)
	if err != nil {
		return nil, err
	}

	pipe := NewInPipeWithRecognizer(config, recognizer)

	if impl, ok := pipe.(*inPipeImpl); ok {
		impl.SetAudioSource(source)
	}

	return pipe, nil
}
