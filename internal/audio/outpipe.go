package audio

import (
	"context"
	"io"

	"github.com/liuscraft/orion-x/internal/tts"
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
	Mixer    *MixerConfig
	TTS      tts.Config
	VoiceMap map[string]string
}

// DefaultOutPipeConfig 默认配置
func DefaultOutPipeConfig() *OutPipeConfig {
	return &OutPipeConfig{
		Mixer: DefaultMixerConfig(),
		TTS: tts.Config{
			Model:      "cosyvoice-v3-flash",
			Voice:      "longanyang",
			Format:     "pcm",
			SampleRate: 16000,
			Volume:     50,
			Rate:       1.0,
			Pitch:      1.0,
			TextType:   "PlainText",
		},
		VoiceMap: map[string]string{
			"happy":   "longanyang",
			"sad":     "zhichu",
			"angry":   "zhimeng",
			"calm":    "longxiaochun",
			"excited": "longanyang",
			"default": "longanyang",
		},
	}
}
