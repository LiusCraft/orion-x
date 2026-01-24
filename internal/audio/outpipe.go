package audio

import (
	"context"
	"io"
)

// AudioOutPipe 音频输出管道，负责音频混合播放、队列管理、中断处理
type AudioOutPipe interface {
	Start(ctx context.Context) error
	Stop() error
	PlayTTS(text string, emotion string) error
	PlayResource(audio io.Reader) error
	Interrupt() error
	SetMixer(mixer AudioMixer)
}

// OutPipeConfig OutPipe配置
type OutPipeConfig struct {
	Mixer *MixerConfig
}

// DefaultOutPipeConfig 默认配置
func DefaultOutPipeConfig() *OutPipeConfig {
	return &OutPipeConfig{
		Mixer: DefaultMixerConfig(),
	}
}
