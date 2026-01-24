package audio

import (
	"io"
)

// AudioMixer 音频混音器，负责音频混合和音量控制
type AudioMixer interface {
	AddTTSStream(audio io.Reader)
	AddResourceStream(audio io.Reader)
	RemoveTTSStream()
	RemoveResourceStream()
	SetTTSVolume(volume float64)
	SetResourceVolume(volume float64)
	OnTTSStarted()
	OnTTSFinished()
	Start()
	Stop()
}

// MixerConfig Mixer配置
type MixerConfig struct {
	TTSVolume      float64 // 默认TTS音量
	ResourceVolume float64 // 默认资源音频音量
	// 当TTS播放时，资源音频自动降为50%
}

// DefaultMixerConfig 默认配置
// 参考 Python 实现：
// - TTS 音量：100%
// - Resource 音量：15%（Ducking 效果，避免掩盖 TTS）
// - TTS 播放时：Resource 音量降为 7.5%（15% * 0.5）
func DefaultMixerConfig() *MixerConfig {
	return &MixerConfig{
		TTSVolume:      1.0,
		ResourceVolume: 1.0,
	}
}
