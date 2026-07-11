package agent

import (
	"context"
	"testing"

	"github.com/liuscraft/orion-x/internal/session"
)

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

func TestBuildContextMessagesWithSystemPrompt(t *testing.T) {
	a := &Agent{systemPrompt: "custom prompt", memorySvc: nil}
	sess := newTestSessionWithUserMessage("你好")

	msgs := a.buildContextMessages(context.Background(), sess)

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "system" || msgs[0].Content != "custom prompt" {
		t.Errorf("expected custom system prompt, got %+v", msgs[0])
	}
}
