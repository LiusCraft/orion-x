package audio

import (
	"context"
)

// AudioInPipe 音频输入管道，负责音频采集、VAD监听、ASR调用
type AudioInPipe interface {
	Start(ctx context.Context) error
	Stop() error
	SendAudio(audio []byte) error
	OnASRResult(handler func(text string, isFinal bool))
}

// InPipeConfig InPipe配置
type InPipeConfig struct {
	SampleRate   int
	Channels     int
	EnableVAD    bool
	VADThreshold float64
}

// DefaultInPipeConfig 默认配置
func DefaultInPipeConfig() *InPipeConfig {
	return &InPipeConfig{
		SampleRate:   16000,
		Channels:     1,
		EnableVAD:    true,
		VADThreshold: 0.5,
	}
}
