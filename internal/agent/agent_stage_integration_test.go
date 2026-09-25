package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/pkg/pipeline"
)

// 这是一个集成测试示例，展示如何组合多个 Stage

// mockAgent 简化的 Mock Agent
type mockAgent struct{}

func (m *mockAgent) SetLanguage(lang string) {}

func (m *mockAgent) Run(ctx context.Context, sess *session.Session) (<-chan agent.AgentEvent, error) {
	text := ""
	if len(sess.Messages) > 0 {
		text = sess.Messages[len(sess.Messages)-1].Content
	}
	ch := make(chan agent.AgentEvent, 2)
	ch <- &agent.TextChunkEvent{Chunk: "Response to: " + text}
	ch <- &agent.FinishedEvent{}
	close(ch)
	return ch, nil
}

func TestMultipleStages_MessageFlow(t *testing.T) {
	// 测试消息在多个 Stage 之间的流转
	mockAg := &mockAgent{}

	p := pipeline.NewBuilder().
		AddStage(agent.NewAgentStage(mockAg, session.New(session.SessionMeta{Model: "test"}))).
		AddStage(pipeline.NewEmotionExtractorStage()).
		AddStage(pipeline.NewTextFilterStage()).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer func() { _ = p.Stop() }()

	// 发送输入
	go func() {
		p.Input() <- pipeline.NewMessage(
			pipeline.MessageTypeData,
			"I'm <emotion>happy</emotion> <metadata>today</metadata>",
		)
	}()

	// 接收第一条消息
	msg := <-p.Output()

	// 验证: Emotion 应该被提取
	if msg.Metadata.Emotion != "happy" {
		t.Errorf("Expected emotion 'happy', got '%s'", msg.Metadata.Emotion)
	}

	// 验证: metadata 标签应该被过滤
	if text, ok := msg.Payload.(string); ok {
		if text != "Response to: I'm <emotion>happy</emotion> " {
			t.Errorf("Unexpected filtered text: %s", text)
		}
	}
}
