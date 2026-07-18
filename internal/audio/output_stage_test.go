package audio

import (
	"context"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/pkg/pipeline"
)

func TestOutputStagePassesMessage(t *testing.T) {
	stage := NewOutputStage()
	in := make(chan pipeline.Message, 1)
	in <- pipeline.NewMessage(pipeline.MessageTypeData, "hello")
	close(in)
	out := stage.Process(context.Background(), in)
	select {
	case msg := <-out:
		if msg.Payload != "hello" {
			t.Fatalf("payload = %v", msg.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for output")
	}
}
