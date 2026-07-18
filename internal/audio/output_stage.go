package audio

import (
	"context"

	"github.com/liuscraft/orion-x/pkg/pipeline"
)

// OutputStage is a generic DAG sink. It provides one named merge point for a
// channel's protocol adapter while preserving each pipeline message unchanged.
type OutputStage struct{ *pipeline.BaseStage }

func NewOutputStage() pipeline.Stage {
	return &OutputStage{BaseStage: pipeline.NewBaseStage("session_output")}
}

func (s *OutputStage) Process(ctx context.Context, input <-chan pipeline.Message) <-chan pipeline.Message {
	output := make(chan pipeline.Message)
	go func() {
		defer close(output)
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-input:
				if !ok {
					return
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
