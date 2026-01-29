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
	// PlayTTS 播放 TTS（异步，立即返回）
	PlayTTS(text string, emotion string) error
	PlayResource(audio io.Reader) error
	// Interrupt 中断所有任务（清空队列、停止播放）
	Interrupt() error
	SetMixer(mixer AudioMixer)
	SetReferenceSink(sink ReferenceSink)
	// SetOnPlaybackFinished 设置播放完成回调（每个 TTS 播放完成时调用）
	SetOnPlaybackFinished(callback PlaybackFinishedCallback)
	// Stats 获取 Pipeline 统计信息
	Stats() PipelineStats
}

// OutPipeConfig OutPipe配置
type OutPipeConfig struct {
	Mixer       *MixerConfig
	TTS         tts.Config
	TTSPipeline *TTSPipelineConfig
	VoiceMap    map[string]string
}

// DefaultOutPipeConfig 默认配置
func DefaultOutPipeConfig() *OutPipeConfig {
	return &OutPipeConfig{
		Mixer:       DefaultMixerConfig(),
		TTSPipeline: DefaultTTSPipelineConfig(),
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
