package stages

import (
	"context"
	"time"

	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/pipeline"
)

// AudioSource is an audio input source that provides raw PCM bytes.
// Defined here because only ASRStage uses it to drive ASRProcessor.Write.
type AudioSource interface {
	Read(ctx context.Context) ([]byte, error)
	Close() error
}

// ASRStage wraps an ASRProcessor as a pipeline source stage.
// If source is non-nil, it reads audio from the source in a background goroutine.
// Otherwise the caller is responsible for pushing audio via proc.Write.
type ASRStage struct {
	*pipeline.BaseStage
	proc   audio.ASRProcessor
	source AudioSource
}

// NewASRStage creates an ASRStage that reads from source.
func NewASRStage(proc audio.ASRProcessor, source AudioSource) pipeline.Stage {
	return &ASRStage{
		BaseStage: pipeline.NewBaseStage("asr"),
		proc:      proc,
		source:    source,
	}
}

func (s *ASRStage) Process(ctx context.Context, input <-chan pipeline.Message) <-chan pipeline.Message {
	output := make(chan pipeline.Message, 16)

	s.proc.OnResult(func(result audio.ASRResult) {
		msgType := pipeline.MessageTypeTextPartial
		if result.IsFinal {
			msgType = pipeline.MessageTypeTextChunk
		}
		select {
		case output <- pipeline.Message{
			Type:    msgType,
			Payload: result.Text,
			Metadata: pipeline.Metadata{
				Timestamp: time.Now(),
			},
		}:
		case <-ctx.Done():
		}
	})

	s.proc.OnSpeechStart(func() {
		select {
		case output <- pipeline.Message{
			Type: pipeline.MessageTypeInterrupt,
			Metadata: pipeline.Metadata{
				Timestamp: time.Now(),
			},
		}:
		case <-ctx.Done():
		}
	})

	go func() {
		defer close(output)

		if err := s.proc.Start(ctx); err != nil {
			logging.Errorf("ASRStage: start error: %v", err)
			select {
			case output <- pipeline.Message{
				Type: pipeline.MessageTypeError,
				Metadata: pipeline.Metadata{
					Error:     err,
					Timestamp: time.Now(),
				},
			}:
			default:
			}
			return
		}

		if s.source != nil {
			go s.readFromSource(ctx)
		}

		<-ctx.Done()
		_ = s.proc.Stop()
	}()

	return output
}

func (s *ASRStage) readFromSource(ctx context.Context) {
	defer func() { _ = s.source.Close() }()
	for {
		data, err := s.source.Read(ctx)
		if err != nil {
			return
		}
		if err := s.proc.Write(data); err != nil {
			return
		}
	}
}
