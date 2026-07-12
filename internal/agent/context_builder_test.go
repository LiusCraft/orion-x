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

	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages (soul+rules+user), got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "system" || msgs[0].Content != "═══════════════════ 身份设定 (SOUL) ═══════════════════\n"+SoulPrompt() {
		t.Errorf("expected soul prompt, got %+v", msgs[0])
	}
	if msgs[1].Role != "system" || msgs[1].Content != RulesPrompt() {
		t.Errorf("expected rules prompt, got %+v", msgs[1])
	}
	if msgs[2].Role != "user" || msgs[2].Content != "你好" {
		t.Errorf("expected user message, got %+v", msgs[2])
	}
}

func TestBuildContextMessagesWithCustomSoul(t *testing.T) {
	a := &Agent{soulPrompt: "custom soul", rulesPrompt: "custom rules", memorySvc: nil}
	sess := newTestSessionWithUserMessage("你好")

	msgs := a.buildContextMessages(context.Background(), sess)

	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "system" || msgs[0].Content != "═══════════════════ 身份设定 (SOUL) ═══════════════════\n"+"custom soul" {
		t.Errorf("expected custom soul, got %+v", msgs[0])
	}
	if msgs[1].Role != "system" || msgs[1].Content != "custom rules" {
		t.Errorf("expected custom rules, got %+v", msgs[1])
	}
}
