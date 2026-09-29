package wecom

import (
	"testing"
	"time"
)

// 会话在空闲超过 TTL 后必须被回收：否则 wsserver 会按会话数无限增长内存。
func TestSweepSessionsReclaimsIdleOnly(t *testing.T) {
	ch := NewWeComChannel(nil, nil, nil)
	body := &callbackBody{ChatType: "single", From: callbackFrom{UserID: "u1"}}

	idle := ch.chat("dev-1", body, "model-x")
	if _, ok := ch.sessions.Get(idle.sess.ID); !ok {
		t.Fatal("new chat session is not registered")
	}

	idle.mu.Lock()
	idle.lastUsed = time.Now().Add(-sessionIdleTTL - time.Minute)
	idle.mu.Unlock()

	active := ch.chat("dev-1", &callbackBody{ChatType: "single", From: callbackFrom{UserID: "u2"}}, "model-x")

	ch.sweepSessions()

	if _, ok := ch.sessions.Get(idle.sess.ID); ok {
		t.Error("idle session is still registered")
	}
	ch.mu.Lock()
	_, present := ch.chats[idle.key]
	ch.mu.Unlock()
	if present {
		t.Error("idle chat state is still present")
	}
	if _, ok := ch.sessions.Get(active.sess.ID); !ok {
		t.Error("active session was reclaimed")
	}
}

// 同一会话重复取用必须返回同一个 session，历史才能延续。
func TestChatReusesSessionPerConversation(t *testing.T) {
	ch := NewWeComChannel(nil, nil, nil)
	first := ch.chat("dev-1", &callbackBody{ChatType: "single", From: callbackFrom{UserID: "u1"}}, "model-x")
	second := ch.chat("dev-1", &callbackBody{ChatType: "single", From: callbackFrom{UserID: "u1"}}, "model-x")
	other := ch.chat("dev-1", &callbackBody{ChatType: "single", From: callbackFrom{UserID: "u2"}}, "model-x")

	if first != second {
		t.Error("same conversation returned different chat state")
	}
	if first == other {
		t.Error("different conversations share one chat state")
	}
	if first.sess.ID != "wecom:dev-1:u1" {
		t.Errorf("session id = %q, want %q", first.sess.ID, "wecom:dev-1:u1")
	}
}
