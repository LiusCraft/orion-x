package realtime

import (
	"context"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/pkg/pipeline"
)

type realtimeASRStage struct {
	*pipeline.BaseStage
	proc     audio.ASRProcessor
	onResult func(audio.ASRResult)
	onSpeech func()
}

func newRealtimeASRStage(proc audio.ASRProcessor, onResult func(audio.ASRResult), onSpeech func()) pipeline.Stage {
	return &realtimeASRStage{
		BaseStage: pipeline.NewBaseStage("asr"),
		proc:      proc,
		onResult:  onResult,
		onSpeech:  onSpeech,
	}
}

func (s *realtimeASRStage) Process(ctx context.Context, input <-chan pipeline.Message) <-chan pipeline.Message {
	output := make(chan pipeline.Message, 16)
	done := make(chan struct{})
	var mu sync.Mutex
	closed := false
	send := func(msg pipeline.Message) {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return
		}
		select {
		case output <- msg:
		case <-ctx.Done():
		}
	}
	closeOutput := func() {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return
		}
		closed = true
		close(output)
	}
	s.proc.OnResult(func(result audio.ASRResult) {
		if s.onResult != nil {
			s.onResult(result)
		}
		if !result.IsFinal || result.Text == "" {
			return
		}
		send(pipeline.Message{
			Type:     pipeline.MessageTypeData,
			Payload:  result.Text,
			Metadata: pipeline.Metadata{Timestamp: time.Now()},
		})
	})
	s.proc.OnSpeechStart(func() {
		if s.onSpeech != nil {
			s.onSpeech()
		}
		send(pipeline.Message{Type: pipeline.MessageTypeInterrupt, Metadata: pipeline.Metadata{Timestamp: time.Now()}})
	})

	go func() {
		defer closeOutput()
		defer close(done)
		if err := s.proc.Start(ctx); err != nil {
			logging.Errorf("realtime asr stage: start error: %v", err)
			send(pipeline.Message{Type: pipeline.MessageTypeError, Metadata: pipeline.Metadata{Error: err, Timestamp: time.Now()}})
			return
		}
		<-ctx.Done()
		_ = s.proc.Stop()
	}()

	go func() {
		for {
			select {
			case <-done:
				return
			case msg, ok := <-input:
				if !ok {
					return
				}
				if msg.Type != pipeline.MessageTypeData && msg.Type != pipeline.MessageTypeInterrupt {
					continue
				}
				send(msg)
			}
		}
	}()
	return output
}
