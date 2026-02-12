package memory

import (
	"context"
	"testing"
	"time"
)

type fakeStore struct {
	queryItems []MemoryItem
	savedTurn  bool
	savedItems bool
}

func (f *fakeStore) SaveTurn(turn Turn) error {
	f.savedTurn = true
	return nil
}

func (f *fakeStore) SaveItems(items []MemoryItem) error {
	if len(items) > 0 {
		f.savedItems = true
	}
	return nil
}

func (f *fakeStore) Query(userID, query string, limit int, minScore float64) ([]MemoryItem, error) {
	return f.queryItems, nil
}

func (f *fakeStore) Purge(now time.Time, retentionDays int) error { return nil }

func (f *fakeStore) Close() error { return nil }

func TestServiceBuildContextMessagesNone(t *testing.T) {
	svc, err := NewService(Config{Mode: ModeNone}, Options{SystemPrompt: "SYS"})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	msgs, err := svc.BuildContextMessages(context.Background(), "hello")
	if err != nil {
		t.Fatalf("build messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "SYS" || msgs[1].Content != "hello" {
		t.Fatalf("unexpected messages content")
	}
}

func TestServiceSessionHistory(t *testing.T) {
	svc, err := NewService(Config{Mode: ModeSession, SessionMaxTurns: 3}, Options{SystemPrompt: "SYS"})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	turn := Turn{UserText: "u1", AssistantText: "a1", StartedAt: time.Now(), EndedAt: time.Now()}
	if err := svc.RecordTurn(context.Background(), turn); err != nil {
		t.Fatalf("record turn: %v", err)
	}
	msgs, err := svc.BuildContextMessages(context.Background(), "u2")
	if err != nil {
		t.Fatalf("build messages: %v", err)
	}
	if len(msgs) < 4 {
		t.Fatalf("expected history messages, got %d", len(msgs))
	}
	if msgs[1].Content != "u1" || msgs[2].Content != "a1" {
		t.Fatalf("expected history in messages")
	}
}

func TestServiceLongTermQueryAndRecord(t *testing.T) {
	store := &fakeStore{queryItems: []MemoryItem{{Content: "记忆"}}}
	svc, err := NewService(Config{Mode: ModeLongTerm}, Options{SystemPrompt: "SYS", Store: store})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	ctx := WithContext(context.Background(), Context{UserID: "u1"})
	msgs, err := svc.BuildContextMessages(ctx, "问题")
	if err != nil {
		t.Fatalf("build messages: %v", err)
	}
	if len(msgs) < 3 {
		t.Fatalf("expected long-term memory message, got %d", len(msgs))
	}
	turn := Turn{UserText: "u1", AssistantText: "a1", StartedAt: time.Now(), EndedAt: time.Now()}
	if err := svc.RecordTurn(ctx, turn); err != nil {
		t.Fatalf("record turn: %v", err)
	}
	if !store.savedTurn {
		t.Fatalf("expected save turn")
	}
}
