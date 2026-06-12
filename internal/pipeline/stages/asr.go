package stages

import (
	"context"
	"time"

	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/pipeline"
)

// ASRStage ASR 识别 Stage（Source Stage，无输入）
type ASRStage struct {
	*pipeline.BaseStage
	audioInPipe audio.AudioInPipe
}

// NewASRStage 创建 ASRStage
func NewASRStage(audioInPipe audio.AudioInPipe) pipeline.Stage {
	return &ASRStage{
		BaseStage:   pipeline.NewBaseStage("asr"),
		audioInPipe: audioInPipe,
	}
}

// Process 处理消息（Source Stage，忽略 input）
func (s *ASRStage) Process(ctx context.Context, input <-chan pipeline.Message) <-chan pipeline.Message {
	output := make(chan pipeline.Message, 16) // 缓冲，避免阻塞 ASR 回调

	// 设置 ASR 回调
	s.audioInPipe.OnASRResult(func(text string, isFinal bool) {
		msgType := pipeline.MessageTypeTextPartial
		if isFinal {
			msgType = pipeline.MessageTypeTextChunk
		}

		msg := pipeline.Message{
			Type:    msgType,
			Payload: text,
			Metadata: pipeline.Metadata{
				Timestamp: time.Now(),
			},
		}

		select {
		case output <- msg:
		case <-ctx.Done():
		}
	})

	// 设置用户说话检测回调
	s.audioInPipe.OnUserSpeakingDetected(func() {
		msg := pipeline.Message{
			Type: pipeline.MessageTypeInterrupt,
			Metadata: pipeline.Metadata{
				Timestamp: time.Now(),
			},
		}

		select {
		case output <- msg:
		case <-ctx.Done():
		}
	})

	// 启动 AudioInPipe
	go func() {
		defer close(output)

		if err := s.audioInPipe.Start(ctx); err != nil {
			logging.Errorf("ASRStage: start audio in pipe error: %v", err)
			output <- pipeline.Message{
				Type: pipeline.MessageTypeError,
				Metadata: pipeline.Metadata{
					Error:     err,
					Timestamp: time.Now(),
				},
			}
			return
		}

		<-ctx.Done()
		s.audioInPipe.Stop()
	}()

	return output
}
