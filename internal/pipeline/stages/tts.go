package stages

import (
	"context"

	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/pipeline"
	"github.com/liuscraft/orion-x/internal/provider/tts"
)

// TTSStage wraps a TTSProcessor as a pipeline sink stage.
type TTSStage struct {
	*pipeline.BaseStage
	proc        audio.TTSProcessor
	lastEmotion string
}

// NewTTSStage creates a TTSStage. The proc must be started externally before
// the pipeline starts (e.g., proc.Start in main before pipeline.Start).
func NewTTSStage(proc audio.TTSProcessor) pipeline.Stage {
	return &TTSStage{
		BaseStage: pipeline.NewBaseStage("tts"),
		proc:      proc,
	}
}

func (s *TTSStage) Process(ctx context.Context, input <-chan pipeline.Message) <-chan pipeline.Message {
	output := make(chan pipeline.Message)

	go func() {
		defer close(output)
		defer func() {
			// 确保管道关闭时把剩余文本送出去
			opts := s.currentOpts()
			_ = s.proc.Flush(opts)
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
				case pipeline.MessageTypeData:
					if err := s.handleTextChunk(msg); err != nil {
						logging.Errorf("TTSStage: handle text chunk error: %v", err)
						msg = msg.WithError(err)
					}

				case pipeline.MessageTypeFinished:
					opts := s.currentOpts()
					if err := s.proc.Flush(opts); err != nil {
						logging.Errorf("TTSStage: flush TTS error: %v", err)
					}
					s.lastEmotion = ""

				case pipeline.MessageTypeInterrupt:
					_ = s.proc.Interrupt()
					s.lastEmotion = ""
				}

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

func (s *TTSStage) handleTextChunk(msg pipeline.Message) error {
	text, ok := msg.Payload.(string)
	if !ok {
		return nil
	}

	emotion := msg.Metadata.Emotion
	if emotion == "" {
		emotion = "default"
	}
	s.lastEmotion = emotion

	return s.proc.Write(text, tts.SynthesisOptions{Emotion: emotion})
}

func (s *TTSStage) currentOpts() tts.SynthesisOptions {
	emotion := s.lastEmotion
	if emotion == "" {
		emotion = "default"
	}
	return tts.SynthesisOptions{Emotion: emotion}
}
