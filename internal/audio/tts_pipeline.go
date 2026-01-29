package audio

import (
	"context"
)

// PlaybackFinishedCallback 播放完成回调
type PlaybackFinishedCallback func()

// TTSPipeline TTS 异步处理管道
// 负责管理文本队列、TTS 生成队列、播放队列
// 支持快速中断（清空所有队列）
type TTSPipeline interface {
	// EnqueueText 入队文本（非阻塞，立即返回）
	EnqueueText(text string, emotion string) error

	// Interrupt 中断所有任务（清空队列、停止播放）
	Interrupt() error

	// Start 启动 Pipeline
	Start(ctx context.Context) error

	// Stop 停止 Pipeline
	Stop() error

	// Stats 获取统计信息（用于调试和监控）
	Stats() PipelineStats

	// SetMixer 设置音频混音器
	SetMixer(mixer AudioMixer)

	// SetReferenceSink 设置参考音频输出（用于 AEC）
	SetReferenceSink(sink ReferenceSink)

	// SetOnPlaybackFinished 设置播放完成回调
	// 当所有队列清空且播放完成时触发
	SetOnPlaybackFinished(callback PlaybackFinishedCallback)
}

// PipelineStats Pipeline 统计信息
type PipelineStats struct {
	TextQueueSize   int  // 文本队列长度
	TTSBufferSize   int  // TTS 缓冲区长度
	IsPlaying       bool // 是否正在播放
	TotalEnqueued   int  // 总入队数
	TotalPlayed     int  // 总播放数
	TotalInterrupts int  // 总中断次数
}

// TTSPipelineConfig TTS Pipeline 配置
type TTSPipelineConfig struct {
	// MaxTTSBuffer TTS 缓冲区最大容量
	// 已生成但未播放的 TTS 流数量上限
	// 超出则阻塞 TTS Worker，等待播放器消费
	// 默认: 3
	MaxTTSBuffer int `json:"max_tts_buffer"`

	// MaxConcurrentTTS 最大并发 TTS 生成数
	// 控制同时调用 TTS 服务的数量，避免过多并发
	// 默认: 2
	MaxConcurrentTTS int `json:"max_concurrent_tts"`

	// TextQueueSize 文本队列大小
	// 待处理的文本数量上限，防止内存爆炸
	// 超出则阻塞入队（保护内存）
	// 默认: 100
	TextQueueSize int `json:"text_queue_size"`
}

// DefaultTTSPipelineConfig 默认 TTS Pipeline 配置
func DefaultTTSPipelineConfig() *TTSPipelineConfig {
	return &TTSPipelineConfig{
		MaxTTSBuffer:     3,
		MaxConcurrentTTS: 2,
		TextQueueSize:    100,
	}
}

// textItem 文本队列项
type textItem struct {
	Text    string
	Emotion string
}
