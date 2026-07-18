package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/pkg/pipeline"
)

type fakeSubAgentRunner struct{ events []AgentEvent }

func (f fakeSubAgentRunner) Run(_ context.Context, _ *session.Session) (<-chan AgentEvent, error) {
	ch := make(chan AgentEvent, len(f.events))
	for _, event := range f.events {
		ch <- event
	}
	close(ch)
	return ch, nil
}
func (fakeSubAgentRunner) SetLanguage(string) {}

func TestSubAgentForwardsEventsAsPipelineMessages(t *testing.T) {
	sa := NewSubAgent("sub-1", "task-1", fakeSubAgentRunner{events: []AgentEvent{
		&TextChunkEvent{Chunk: "draft"}, &FinishedEvent{},
	}})
	if err := sa.Start(context.Background(), session.New(session.SessionMeta{})); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	var messages []pipeline.Message
	for msg := range sa.OutputCh {
		messages = append(messages, msg)
	}
	if len(messages) != 2 || messages[0].Payload != "draft" || messages[1].Type != pipeline.MessageTypeFinished {
		t.Fatalf("messages = %#v", messages)
	}
	if messages[0].Metadata.Extra["task_id"] != "task-1" {
		t.Fatalf("metadata = %#v", messages[0].Metadata.Extra)
	}
}

func TestSubAgentRejectsMissingDependencies(t *testing.T) {
	sa := NewSubAgent("sub-1", "task-1", nil)
	if err := sa.Start(context.Background(), session.New(session.SessionMeta{})); err == nil {
		t.Fatal("Start() without agent succeeded")
	}
	sa = NewSubAgent("sub-1", "task-1", fakeSubAgentRunner{})
	if err := sa.Start(context.Background(), nil); err == nil {
		t.Fatal("Start() without session succeeded")
	}
}

func TestSubAgentForwardsFailure(t *testing.T) {
	sa := NewSubAgent("sub-1", "task-1", fakeSubAgentRunner{events: []AgentEvent{&FinishedEvent{Error: errors.New("failed")}}})
	if err := sa.Start(context.Background(), session.New(session.SessionMeta{})); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case msg := <-sa.OutputCh:
		if msg.Type != pipeline.MessageTypeError || msg.Metadata.Error == nil {
			t.Fatalf("message = %#v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for error")
	}
}
