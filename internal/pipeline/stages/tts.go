package stages

import (
	"context"

	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/pipeline"
)

// TTSStage TTS 合成 Stage
type TTSStage struct {
	*pipeline.BaseStage
	audioOutPipe audio.AudioOutPipe
	ttsActive    bool
}

// NewTTSStage 创建 TTSStage
func NewTTSStage(audioOutPipe audio.AudioOutPipe) pipeline.Stage {
	return &TTSStage{
		BaseStage:    pipeline.NewBaseStage("tts"),
		audioOutPipe: audioOutPipe,
	}
}

// Process 处理消息
func (s *TTSStage) Process(ctx context.Context, input <-chan pipeline.Message) <-chan pipeline.Message {
	output := make(chan pipeline.Message)

	go func() {
		defer close(output)
		defer func() {
			if s.ttsActive {
				s.audioOutPipe.EndTTSStream()
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-input:
				if !ok {
					return
				}

				switch msg.Type {
				case pipeline.MessageTypeTextChunk:
					// 处理文本 chunk
					if err := s.handleTextChunk(msg); err != nil {
						logging.Errorf("TTSStage: handle text chunk error: %v", err)
						msg = msg.WithError(err)
					}

				case pipeline.MessageTypeFinished:
					// 结束 TTS session
					if s.ttsActive {
						if err := s.audioOutPipe.EndTTSStream(); err != nil {
							logging.Errorf("TTSStage: end TTS stream error: %v", err)
						}
						s.ttsActive = false

						// 发送 TTS 停止消息
						select {
						case output <- pipeline.Message{
							Type:     pipeline.MessageTypeTTSStop,
							Metadata: msg.Metadata,
						}:
						case <-ctx.Done():
							return
						}
					}

				case pipeline.MessageTypeInterrupt:
					// 打断 TTS
					if s.ttsActive {
						s.audioOutPipe.Interrupt()
						s.ttsActive = false
					}
				}

				// 透传消息
				select {
				case output <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return output
}

// handleTextChunk 处理文本块
func (s *TTSStage) handleTextChunk(msg pipeline.Message) error {
	text, ok := msg.Payload.(string)
	if !ok {
		return nil // 跳过非文本消息
	}

	// 第一个 chunk 建立 TTS session
	if !s.ttsActive {
		emotion := msg.Metadata.Emotion
		if emotion == "" {
			emotion = "default"
		}

		if err := s.audioOutPipe.BeginTTSStream(emotion); err != nil {
			return err
		}
		s.ttsActive = true
		logging.Infof("TTSStage: TTS stream started with emotion: %s", emotion)
	}

	// 写入文本到 TTS
	if err := s.audioOutPipe.WriteTTSChunk(text); err != nil {
		return err
	}

	return nil
}
