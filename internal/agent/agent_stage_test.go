package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/pkg/pipeline"
)

// mockAgentRunner 测试用 Mock Agent
type mockAgentRunner struct {
	processFunc func(ctx context.Context, sess *session.Session) (<-chan AgentEvent, error)
}

func (m *mockAgentRunner) Run(ctx context.Context, sess *session.Session) (<-chan AgentEvent, error) {
	if m.processFunc != nil {
		return m.processFunc(ctx, sess)
	}
	ch := make(chan AgentEvent)
	close(ch)
	return ch, nil
}

func (m *mockAgentRunner) SetLanguage(lang string) {}

func TestAgentStage_TextChunk(t *testing.T) {
	mockAgent := &mockAgentRunner{
		processFunc: func(ctx context.Context, sess *session.Session) (<-chan AgentEvent, error) {
			text := ""
			if len(sess.Messages) > 0 {
				text = sess.Messages[len(sess.Messages)-1].Content
			}
			ch := make(chan AgentEvent, 2)
			ch <- &TextChunkEvent{Chunk: "Response: " + text}
			ch <- &FinishedEvent{}
			close(ch)
			return ch, nil
		},
	}

	stage := NewAgentStage(mockAgent, session.New(session.SessionMeta{Model: "test"}))
	input := make(chan pipeline.Message, 1)
	ctx := context.Background()

	output := stage.Process(ctx, input)

	input <- pipeline.NewMessage(pipeline.MessageTypeData, "hello")

	var messages []pipeline.Message
	for i := 0; i < 2; i++ {
		msg, ok := <-output
		if !ok {
			break
		}
		messages = append(messages, msg)
	}
	close(input)
	for msg := range output {
		messages = append(messages, msg)
	}

	if len(messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(messages))
	}

	if messages[0].Type != pipeline.MessageTypeData {
		t.Errorf("Expected MessageTypeData, got %s", messages[0].Type)
	}
	if messages[0].Payload != "Response: hello" {
		t.Errorf("Expected 'Response: hello', got '%s'", messages[0].Payload)
	}

	if messages[1].Type != pipeline.MessageTypeFinished {
		t.Errorf("Expected MessageTypeFinished, got %s", messages[1].Type)
	}
}

func TestAgentStage_Error(t *testing.T) {
	mockAgent := &mockAgentRunner{
		processFunc: func(ctx context.Context, sess *session.Session) (<-chan AgentEvent, error) {
			return nil, fmt.Errorf("agent error")
		},
	}

	stage := NewAgentStage(mockAgent, session.New(session.SessionMeta{Model: "test"}))
	input := make(chan pipeline.Message, 1)
	ctx := context.Background()

	output := stage.Process(ctx, input)

	input <- pipeline.NewMessage(pipeline.MessageTypeData, "test")
	close(input)

	// 验证错误
	msg := <-output
	if !msg.IsError() {
		t.Error("Expected error message")
	}
	if msg.Metadata.Error.Error() != "agent error" {
		t.Errorf("Expected 'agent error', got '%v'", msg.Metadata.Error)
	}
}

func TestAgentStage_Passthrough(t *testing.T) {
	mockAgent := &mockAgentRunner{}
	stage := NewAgentStage(mockAgent, session.New(session.SessionMeta{Model: "test"}))
	input := make(chan pipeline.Message, 1)
	ctx := context.Background()

	output := stage.Process(ctx, input)

	// 发送非文本消息，应该透传
	input <- pipeline.NewMessage(pipeline.MessageTypeData, []byte{1, 2, 3})
	close(input)

	msg := <-output
	if msg.Type != pipeline.MessageTypeData {
		t.Errorf("Expected MessageTypeData, got %s", msg.Type)
	}
}
