package stages_test

import (
	"context"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/pipeline"
	"github.com/liuscraft/orion-x/internal/pipeline/stages"
	"github.com/liuscraft/orion-x/internal/session"
)

// 这是一个集成测试示例，展示如何组合多个 Stage

// mockAgent 简化的 Mock Agent
type mockAgent struct{}

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

func TestAgentStage_Integration(t *testing.T) {
	// 构建简单的 Pipeline: AgentStage -> TextFilterStage
	agent := &mockAgent{}

	p := pipeline.NewBuilder().
		AddStage(stages.NewAgentStage(agent, session.New(session.SessionMeta{Model: "test"}))).
		AddStage(pipeline.NewTextFilterStage()).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer p.Stop()

	// 发送用户输入
	go func() {
		p.Input() <- pipeline.NewMessage(pipeline.MessageTypeTextChunk, "hello <metadata>world</metadata>")
	}()

	// 接收输出
	var messages []pipeline.Message
	timeout := time.After(1 * time.Second)

	for {
		select {
		case msg, ok := <-p.Output():
			if !ok {
				t.Fatal("Pipeline output closed unexpectedly")
			}
			messages = append(messages, msg)

			// 收到 Finished 消息后退出
			if msg.Type == pipeline.MessageTypeFinished {
				goto done
			}

		case <-timeout:
			t.Fatal("Timeout waiting for response")
		}
	}

done:
	// 验证: 应该收到 1 个文本响应 + 1 个 Finished
	if len(messages) < 2 {
		t.Fatalf("Expected at least 2 messages, got %d", len(messages))
	}

	// 第一个是文本响应
	if messages[0].Type != pipeline.MessageTypeTextChunk {
		t.Errorf("Expected first message to be TextChunk, got %s", messages[0].Type)
	}

	// 最后一个是 Finished
	lastMsg := messages[len(messages)-1]
	if lastMsg.Type != pipeline.MessageTypeFinished {
		t.Errorf("Expected last message to be Finished, got %s", lastMsg.Type)
	}

	// 验证文本已被过滤
	if text, ok := messages[0].Payload.(string); ok {
		if text == "Response to: hello <metadata>world</metadata>" {
			t.Error("Text filter did not work, metadata tag still present")
		}
	}
}

func TestMultipleStages_MessageFlow(t *testing.T) {
	// 测试消息在多个 Stage 之间的流转
	agent := &mockAgent{}

	p := pipeline.NewBuilder().
		AddStage(stages.NewAgentStage(agent, session.New(session.SessionMeta{Model: "test"}))).
		AddStage(pipeline.NewEmotionExtractorStage()).
		AddStage(pipeline.NewTextFilterStage()).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer p.Stop()

	// 发送输入
	go func() {
		p.Input() <- pipeline.NewMessage(
			pipeline.MessageTypeTextChunk,
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
