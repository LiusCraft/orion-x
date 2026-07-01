package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/memory"
	"github.com/liuscraft/orion-x/internal/session"
)

type fakeMemoryService struct {
	buildFunc func(ctx context.Context, userText string) ([]*llm.Message, error)
}

func (f *fakeMemoryService) BuildContextMessages(ctx context.Context, userText string) ([]*llm.Message, error) {
	return f.buildFunc(ctx, userText)
}

func (f *fakeMemoryService) RecordTurn(ctx context.Context, turn memory.Turn) error { return nil }

func (f *fakeMemoryService) Close() error { return nil }

func newTestSessionWithUserMessage(text string) *session.Session {
	sess := session.New(session.SessionMeta{Model: "test"})
	sess.Add(session.Message{Role: session.RoleUser, Content: text})
	return sess
}

func TestBuildContextMessagesWithoutMemoryService(t *testing.T) {
	a := &Agent{memorySvc: nil}
	sess := newTestSessionWithUserMessage("你好")

	msgs := a.buildContextMessages(context.Background(), sess)

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "system" || msgs[0].Content != defaultSystemPrompt {
		t.Errorf("expected default system prompt, got %+v", msgs[0])
	}
	if msgs[1].Role != "user" || msgs[1].Content != "你好" {
		t.Errorf("expected user message, got %+v", msgs[1])
	}
}

func TestBuildContextMessagesFallsBackOnMemoryError(t *testing.T) {
	a := &Agent{memorySvc: &fakeMemoryService{
		buildFunc: func(ctx context.Context, userText string) ([]*llm.Message, error) {
			return nil, errors.New("boom")
		},
	}}
	sess := newTestSessionWithUserMessage("你好")

	msgs := a.buildContextMessages(context.Background(), sess)

	if len(msgs) != 2 || msgs[0].Content != defaultSystemPrompt {
		t.Fatalf("expected fallback to default system prompt, got %+v", msgs)
	}
}

func TestBuildContextMessagesUsesMemoryServiceSystemMessages(t *testing.T) {
	a := &Agent{memorySvc: &fakeMemoryService{
		buildFunc: func(ctx context.Context, userText string) ([]*llm.Message, error) {
			return []*llm.Message{
				{Role: "system", Content: "记忆上下文"},
				{Role: "user", Content: "不应该出现"},
			}, nil
		},
	}}
	sess := newTestSessionWithUserMessage("你好")

	msgs := a.buildContextMessages(context.Background(), sess)

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (1 system + 1 history), got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "system" || msgs[0].Content != "记忆上下文" {
		t.Errorf("expected memory system message, got %+v", msgs[0])
	}
	if msgs[1].Role != "user" || msgs[1].Content != "你好" {
		t.Errorf("expected user history message, got %+v", msgs[1])
	}
}
