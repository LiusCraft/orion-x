package tts

import (
	"context"
	"io"
)

// Synthesizer 是基本的 TTS 接口（同步、整段合成）。
type Synthesizer interface {
	Synthesize(ctx context.Context, req SynthesizeRequest) (*SynthesizeResult, error)
}

// StreamingSynthesizer 是流式 TTS 接口（低延迟、逐句合成）。
// TTSProcessor 通过类型断言检测，不实现则回退到 Synthesize。
type StreamingSynthesizer interface {
	Synthesizer

	// StartSynthesis 建立连接并返回可复用的流式会话。
	StartSynthesis(ctx context.Context, req SynthesizeRequest) (SynthesisStream, error)
}

// SynthesisStream 是一次 TTS 流式会话，支持多句话连续合成。
// 典型用法：WriteTextChunk → Finish → 读取 AudioReader()。
type SynthesisStream interface {
	// WriteTextChunk 发送一句文本。多句话可连续写入，不需等前句完成。
	WriteTextChunk(ctx context.Context, text string) error

	// Finish 通知服务端文本发送完毕，立即返回（不阻塞等 task-finished）。
	Finish(ctx context.Context) error

	// AudioReader 返回流式音频 reader，可在 Finish 前开始读。
	// task-finished 后返回 EOF。
	AudioReader() io.ReadCloser

	// SentenceBoundaries 返回句边界通知 channel。
	// 不支持服务端分句的 provider 返回 nil。
	SentenceBoundaries() <-chan SentenceBoundary

	// Abort 立即中止会话（打断场景）。
	Abort()
}

// WarmableProvider 是支持预连接预热的扩展接口。
// Warm 在 goroutine 里调用，阻塞直到连接就绪或 ctx 取消。
type WarmableProvider interface {
	Warm(ctx context.Context, req SynthesizeRequest) SynthesisStream
}
