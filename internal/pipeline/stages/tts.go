package stages

import (
	"context"

	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/pipeline"
)

// TTSStage wraps a TTSProcessor as a pipeline sink stage.
type TTSStage struct {
	*pipeline.BaseStage
	proc      audio.TTSProcessor
	ttsActive bool
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
			if s.ttsActive {
				_ = s.proc.EndStream()
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
					if err := s.handleTextChunk(msg); err != nil {
						logging.Errorf("TTSStage: handle text chunk error: %v", err)
						msg = msg.WithError(err)
					}

				case pipeline.MessageTypeFinished:
					if s.ttsActive {
						if err := s.proc.EndStream(); err != nil {
							logging.Errorf("TTSStage: end TTS stream error: %v", err)
						}
						s.ttsActive = false

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
					if s.ttsActive {
						_ = s.proc.Interrupt()
						s.ttsActive = false
					}
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

	if !s.ttsActive {
		emotion := msg.Metadata.Emotion
		if emotion == "" {
			emotion = "default"
		}
		if err := s.proc.BeginStream(audio.TTSRequest{Emotion: emotion}); err != nil {
			return err
		}
		s.ttsActive = true
		logging.Infof("TTSStage: TTS stream started with emotion: %s", emotion)
	}

	return s.proc.WriteChunk(text)
}
