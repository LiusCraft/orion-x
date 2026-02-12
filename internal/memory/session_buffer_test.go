package memory

import (
	"context"
	"sync"
	"testing"
)

type mockSummarizer struct {
	mu     sync.Mutex
	count  int
	result string
}

func (m *mockSummarizer) Summarize(ctx context.Context, turns []Turn) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.count++
	return m.result, nil
}

func TestSessionBufferSummarize(t *testing.T) {
	summarizer := &mockSummarizer{result: "summary"}
	buf := NewSessionBuffer(2, 2, summarizer)

	buf.Add(context.Background(), Turn{UserText: "u1", AssistantText: "a1"})
	buf.Add(context.Background(), Turn{UserText: "u2", AssistantText: "a2"})

	if summarizer.count != 1 {
		t.Fatalf("expected summarizer called once, got %d", summarizer.count)
	}

	msgs := buf.Messages()
	if len(msgs) < 1 {
		t.Fatalf("expected messages, got %d", len(msgs))
	}
	if got := msgs[0].Content; got == "" {
		t.Fatalf("expected summary message")
	}
}

func TestSessionBufferMaxTurns(t *testing.T) {
	buf := NewSessionBuffer(2, 0, nil)
	buf.Add(context.Background(), Turn{UserText: "u1", AssistantText: "a1"})
	buf.Add(context.Background(), Turn{UserText: "u2", AssistantText: "a2"})
	buf.Add(context.Background(), Turn{UserText: "u3", AssistantText: "a3"})

	msgs := buf.Messages()
	// 每轮产生用户+助手两条
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "u2" || msgs[1].Content != "a2" {
		t.Fatalf("expected oldest turn dropped")
	}
}
