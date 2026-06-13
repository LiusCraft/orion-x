package audio

import "context"

// PlaybackFinishedCallback 播放完成回调
type PlaybackFinishedCallback func()

// TTSItemStartedCallback 单条 TTS 开始播放回调
type TTSItemStartedCallback func(text string, emotion string)

// TTSPipeline TTS 异步处理管道
// 负责管理文本队列、TTS 生成队列、播放队列
// 支持快速中断（清空所有队列）
type TTSPipeline interface {
	// EnqueueText 入队文本（非阻塞，立即返回）
	EnqueueText(text string, emotion string) error

	// BeginSession 开始流式 TTS 会话：建立 TTS 连接，立即开始接收音频
	BeginSession(emotion string) error

	// WriteChunk 向当前流式会话写入文本 chunk
	WriteChunk(chunk string) error

	// EndSession 结束流式会话的文本输入（不等待合成完成）
	EndSession() error

	// Interrupt 中断所有任务（清空队列、停止播放）
	Interrupt() error

	// Start 启动 Pipeline
	Start(ctx context.Context) error

	// Stop 停止 Pipeline
	Stop() error

	// Stats 获取统计信息（用于调试和监控）
	Stats() PipelineStats

	// SetSink 设置音频输出目标
	SetSink(sink AudioSink)

	// SetOnPlaybackFinished 设置播放完成回调
	// 当所有队列清空且播放完成时触发
	SetOnPlaybackFinished(callback PlaybackFinishedCallback)

	// SetOnItemStarted 设置单条 TTS 开始播放回调
	SetOnItemStarted(callback TTSItemStartedCallback)
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
	MaxTTSBuffer int `json:"max_tts_buffer"`

	// MaxConcurrentTTS 最大并发 TTS 生成数
	MaxConcurrentTTS int `json:"max_concurrent_tts"`

	// TextQueueSize 文本队列大小
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
