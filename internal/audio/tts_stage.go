package audio

import (
	"context"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/provider/tts"
	"github.com/liuscraft/orion-x/pkg/pipeline"
)

// TTSStage wraps a TTSProcessor as a pipeline stage. It consumes text
// messages (writing them to the processor) and produces TTSChunk
// messages (from the processor's OnChunk callback) as output, so a
// downstream output stage (PortAudio, WebSocket, ...) can consume audio via
// the standard pipeline Message bus instead of a side-channel callback.
type TTSStage struct {
	*pipeline.BaseStage
	proc TTSProcessor
}

// NewTTSStage creates a TTSStage. The proc must be started externally before
// the pipeline starts (e.g., proc.Start in main before pipeline.Start).
func NewTTSStage(proc TTSProcessor) pipeline.Stage {
	return &TTSStage{
		BaseStage: pipeline.NewBaseStage("tts"),
		proc:      proc,
	}
}

func (s *TTSStage) Process(ctx context.Context, input <-chan pipeline.Message) <-chan pipeline.Message {
	output := make(chan pipeline.Message, 16)

	// send/closeOutput 共享 mu，确保 OnChunk 回调（在 TTSProcessor 内部的
	// dispatcher/playAudio goroutine 里触发）和主循环都不会在 output 已关闭后
	// 写入它，避免 send on closed channel。
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

	s.proc.OnChunk(func(chunk TTSChunk) {
		send(pipeline.Message{
			Type:     pipeline.MessageTypeData,
			Payload:  chunk,
			Metadata: pipeline.Metadata{Timestamp: time.Now()},
		})
	})

	go func() {
		defer closeOutput()
		defer func() {
			// 确保管道关闭时把剩余文本送出去
			_ = s.proc.Flush(s.currentOpts())
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
						send(pipeline.Message{
							Type:     pipeline.MessageTypeError,
							Metadata: pipeline.Metadata{Error: err, Timestamp: time.Now()},
						})
					}

				case pipeline.MessageTypeFinished:
					if err := s.proc.Flush(s.currentOpts()); err != nil {
						logging.Errorf("TTSStage: flush TTS error: %v", err)
					}

				case pipeline.MessageTypeInterrupt:
					_ = s.proc.Interrupt()
					send(msg)
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

	return s.proc.Write(text, tts.SynthesisOptions{})
}

func (s *TTSStage) currentOpts() tts.SynthesisOptions {
	return tts.SynthesisOptions{}
}
