package stages

import (
	"context"
	"fmt"
	"testing"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/pipeline"
	"github.com/liuscraft/orion-x/internal/session"
)

// mockAgentRunner 测试用 Mock Agent
type mockAgentRunner struct {
	processFunc func(ctx context.Context, sess *session.Session) (<-chan agent.AgentEvent, error)
}

func (m *mockAgentRunner) Run(ctx context.Context, sess *session.Session) (<-chan agent.AgentEvent, error) {
	if m.processFunc != nil {
		return m.processFunc(ctx, sess)
	}
	ch := make(chan agent.AgentEvent)
	close(ch)
	return ch, nil
}

func TestAgentStage_TextChunk(t *testing.T) {
	mockAgent := &mockAgentRunner{
		processFunc: func(ctx context.Context, sess *session.Session) (<-chan agent.AgentEvent, error) {
			text := ""
			if len(sess.Messages) > 0 {
				text = sess.Messages[len(sess.Messages)-1].Content
			}
			ch := make(chan agent.AgentEvent, 2)
			ch <- &agent.TextChunkEvent{Chunk: "Response: " + text}
			ch <- &agent.FinishedEvent{}
			close(ch)
			return ch, nil
		},
	}

	stage := NewAgentStage(mockAgent)
	input := make(chan pipeline.Message, 1)
	ctx := context.Background()

	output := stage.Process(ctx, input)

	// 发送文本消息
	input <- pipeline.NewMessage(pipeline.MessageTypeTextChunk, "hello")
	close(input)

	// 验证输出
	var messages []pipeline.Message
	for msg := range output {
		messages = append(messages, msg)
	}

	if len(messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(messages))
	}

	// 第一个消息是文本响应
	if messages[0].Type != pipeline.MessageTypeTextChunk {
		t.Errorf("Expected MessageTypeTextChunk, got %s", messages[0].Type)
	}
	if messages[0].Payload != "Response: hello" {
		t.Errorf("Expected 'Response: hello', got '%s'", messages[0].Payload)
	}

	// 第二个消息是 Finished
	if messages[1].Type != pipeline.MessageTypeFinished {
		t.Errorf("Expected MessageTypeFinished, got %s", messages[1].Type)
	}
}

func TestAgentStage_Error(t *testing.T) {
	mockAgent := &mockAgentRunner{
		processFunc: func(ctx context.Context, sess *session.Session) (<-chan agent.AgentEvent, error) {
			return nil, fmt.Errorf("agent error")
		},
	}

	stage := NewAgentStage(mockAgent)
	input := make(chan pipeline.Message, 1)
	ctx := context.Background()

	output := stage.Process(ctx, input)

	input <- pipeline.NewMessage(pipeline.MessageTypeTextChunk, "test")
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
	stage := NewAgentStage(mockAgent)
	input := make(chan pipeline.Message, 1)
	ctx := context.Background()

	output := stage.Process(ctx, input)

	// 发送非文本消息，应该透传
	input <- pipeline.NewMessage(pipeline.MessageTypeAudioData, []byte{1, 2, 3})
	close(input)

	msg := <-output
	if msg.Type != pipeline.MessageTypeAudioData {
		t.Errorf("Expected MessageTypeAudioData, got %s", msg.Type)
	}
}
