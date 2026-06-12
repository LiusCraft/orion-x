package audio

import "io"

// TTSPlaybackObserver 接收 TTS 播放状态通知，供 AudioInPipe 等组件同步噪声基准等策略
type TTSPlaybackObserver interface {
	OnTTSPlaybackStarted()
	OnTTSPlaybackStopped()
}

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
	SetTTSPlaybackObserver(observer TTSPlaybackObserver)
	SetSink(sink AudioSink)
	Start() error
	Stop() error
}

// MixerConfig Mixer配置
type MixerConfig struct {
	TTSVolume       float64 // 默认TTS音量
	ResourceVolume  float64 // 默认资源音频音量
	SampleRate      int     // 系统采样率 (Hz)，默认 16000
	Channels        int     // 输出声道数，默认 2 (立体声)
	FramesPerBuffer int     // 每次输出的采样帧数，默认 1024
	// 当TTS播放时，资源音频自动降为50%
}

// DefaultMixerConfig 默认配置
// 参考 Python 实现：
// - TTS 音量：100%
// - Resource 音量：15%（Ducking 效果，避免掩盖 TTS）
// - TTS 播放时：Resource 音量降为 7.5%（15% * 0.5）
func DefaultMixerConfig() *MixerConfig {
	return &MixerConfig{
		TTSVolume:       1.0,
		ResourceVolume:  1.0,
		SampleRate:      16000, // 默认 16kHz
		Channels:        2,     // 默认立体声
		FramesPerBuffer: 1024,
	}
}
