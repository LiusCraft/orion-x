package session

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	meta := SessionMeta{
		UserID: "user-1",
		Model:  "gpt-4",
	}
	sess := New(meta)

	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if !strings.HasPrefix(sess.ID, "sess_") {
		t.Errorf("ID should start with 'sess_', got %q", sess.ID)
	}
	if sess.Meta.UserID != "user-1" {
		t.Errorf("UserID: got %q, want %q", sess.Meta.UserID, "user-1")
	}
	if sess.Meta.Model != "gpt-4" {
		t.Errorf("Model: got %q, want %q", sess.Meta.Model, "gpt-4")
	}
	if len(sess.Messages) != 0 {
		t.Fatalf("expected empty messages, got %d", len(sess.Messages))
	}
	if sess.Meta.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if sess.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

func TestNewDefaultCreatedAt(t *testing.T) {
	meta := SessionMeta{
		UserID: "u1",
		Model:  "m1",
	}
	sess := New(meta)
	if sess.Meta.CreatedAt.IsZero() {
		t.Error("CreatedAt should be auto-filled if zero")
	}
}

func TestAdd(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})

	msg := Message{Role: RoleUser, Content: "hello"}
	sess.Add(msg)

	if len(sess.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sess.Messages))
	}
	got := sess.Messages[0]
	if got.Role != RoleUser {
		t.Errorf("Role: got %v, want %v", got.Role, RoleUser)
	}
	if got.Content != "hello" {
		t.Errorf("Content: got %q, want %q", got.Content, "hello")
	}
	if got.ID == "" {
		t.Error("ID should be auto-generated")
	}
	if !strings.HasPrefix(got.ID, "msg_") {
		t.Errorf("ID should start with 'msg_', got %q", got.ID)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestAddPreservesID(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})

	msg := Message{ID: "my-custom-id", Role: RoleUser, Content: "hi"}
	sess.Add(msg)

	if sess.Messages[0].ID != "my-custom-id" {
		t.Errorf("ID should be preserved, got %q", sess.Messages[0].ID)
	}
}

func TestAddUpdatesUpdatedAt(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	before := sess.UpdatedAt

	time.Sleep(time.Millisecond)
	msg := Message{Role: RoleUser, Content: "hi"}
	sess.Add(msg)

	if !sess.UpdatedAt.After(before) {
		t.Errorf("UpdatedAt not updated: before=%v, after=%v", before, sess.UpdatedAt)
	}
}

func TestMessageIDUnique(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "a"})
	sess.Add(Message{Role: RoleUser, Content: "b"})
	sess.Add(Message{Role: RoleUser, Content: "c"})

	ids := map[string]bool{
		sess.Messages[0].ID: true,
		sess.Messages[1].ID: true,
		sess.Messages[2].ID: true,
	}
	if len(ids) != 3 {
		t.Errorf("IDs should be unique")
	}
}

func TestSessionJSONRoundTrip(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "gpt-4"})
	sess.Add(Message{Role: RoleUser, Content: "hello"})
	sess.Add(Message{Role: RoleAssistant, Content: "hi", ToolCalls: []ToolCall{
		{ID: "call_1", Name: "greet", Arguments: `{"name":"world"}`},
	}})
	sess.Add(Message{Role: RoleTool, Content: "result", ToolCallID: "call_1", Status: "completed"})

	data, err := json.Marshal(sess)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var restored Session
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if restored.ID != sess.ID {
		t.Errorf("ID mismatch")
	}
	if restored.Meta.UserID != sess.Meta.UserID {
		t.Errorf("UserID mismatch")
	}
	if len(restored.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(restored.Messages))
	}
	if restored.Messages[1].ToolCalls[0].Name != "greet" {
		t.Errorf("ToolCall name mismatch")
	}
	if restored.Messages[2].ToolCallID != "call_1" {
		t.Errorf("ToolCallID mismatch")
	}
	if restored.Messages[2].Status != "completed" {
		t.Errorf("Status mismatch")
	}
}

func TestPop(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})

	_, ok := sess.Pop()
	if ok {
		t.Fatal("expected false for empty session")
	}

	sess.Add(Message{Role: RoleUser, Content: "first"})
	sess.Add(Message{Role: RoleUser, Content: "second"})

	msg, ok := sess.Pop()
	if !ok {
		t.Fatal("expected true for non-empty session")
	}
	if msg.Content != "second" {
		t.Errorf("Content: got %q, want 'second'", msg.Content)
	}
	if len(sess.Messages) != 1 {
		t.Fatalf("expected 1 remaining message, got %d", len(sess.Messages))
	}
	if sess.Messages[0].Content != "first" {
		t.Errorf("remaining message: got %q, want 'first'", sess.Messages[0].Content)
	}
}

func TestPopN(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})

	// PopN on empty
	removed := sess.PopN(2)
	if len(removed) != 0 {
		t.Fatalf("expected 0 removed, got %d", len(removed))
	}

	sess.Add(Message{Role: RoleUser, Content: "a"})
	sess.Add(Message{Role: RoleAssistant, Content: "b"})
	sess.Add(Message{Role: RoleTool, Content: "c"})
	sess.Add(Message{Role: RoleAssistant, Content: "d"})

	// PopN more than length
	removed = sess.PopN(10)
	if len(removed) != 4 {
		t.Fatalf("expected 4 removed, got %d", len(removed))
	}
	if len(sess.Messages) != 0 {
		t.Fatalf("expected 0 remaining, got %d", len(sess.Messages))
	}
	if removed[0].Content != "a" {
		t.Errorf("removed[0]: got %q, want 'a'", removed[0].Content)
	}
	if removed[3].Content != "d" {
		t.Errorf("removed[3]: got %q, want 'd'", removed[3].Content)
	}
}

func TestPopNPartial(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "a"})
	sess.Add(Message{Role: RoleAssistant, Content: "b"})
	sess.Add(Message{Role: RoleTool, Content: "c"})

	removed := sess.PopN(2)
	if len(removed) != 2 {
		t.Fatalf("expected 2 removed, got %d", len(removed))
	}
	if removed[0].Content != "b" || removed[1].Content != "c" {
		t.Errorf("wrong removed: %v %v", removed[0].Content, removed[1].Content)
	}
	if len(sess.Messages) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(sess.Messages))
	}
	if sess.Messages[0].Content != "a" {
		t.Errorf("remaining: got %q, want 'a'", sess.Messages[0].Content)
	}
}

func TestPopNZero(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "a"})

	removed := sess.PopN(0)
	if len(removed) != 0 {
		t.Errorf("PopN(0) should return nil, got %v", removed)
	}
	if len(sess.Messages) != 1 {
		t.Errorf("PopN(0) should not modify messages")
	}
}

func TestPopUpdatesUpdatedAt(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "hi"})

	before := sess.UpdatedAt
	time.Sleep(time.Millisecond)
	sess.Pop()

	if !sess.UpdatedAt.After(before) {
		t.Error("UpdatedAt should be updated after Pop")
	}
}

func TestPopNUpdatesUpdatedAt(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "a"})
	sess.Add(Message{Role: RoleUser, Content: "b"})

	before := sess.UpdatedAt
	time.Sleep(time.Millisecond)
	sess.PopN(1)

	if !sess.UpdatedAt.After(before) {
		t.Error("UpdatedAt should be updated after PopN")
	}
}

func TestPopNNegative(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "a"})

	removed := sess.PopN(-1)
	if len(removed) != 0 {
		t.Errorf("PopN(-1) should return nil, got %v", removed)
	}
	if len(sess.Messages) != 1 {
		t.Errorf("PopN(-1) should not modify messages")
	}
}

func TestToLLMMessages(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "hello"})
	sess.Add(Message{Role: RoleAssistant, Content: "hi", ToolCalls: []ToolCall{
		{ID: "call_1", Name: "grep", Arguments: `{"a":"b"}`},
	}})
	sess.Add(Message{Role: RoleTool, Content: "result", ToolCallID: "call_1", Status: "completed"})

	result := sess.ToLLMMessages()
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}

	// user message
	if result[0].Role != "user" {
		t.Errorf("Role[0]: got %q, want 'user'", result[0].Role)
	}
	if result[0].Content != "hello" {
		t.Errorf("Content[0]: got %q", result[0].Content)
	}

	// assistant with tool calls
	if result[1].Role != "assistant" {
		t.Errorf("Role[1]: got %q", result[1].Role)
	}
	if len(result[1].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result[1].ToolCalls))
	}
	if result[1].ToolCalls[0].Name != "grep" {
		t.Errorf("ToolCalls[0].Name: got %q", result[1].ToolCalls[0].Name)
	}

	// tool message
	if result[2].Role != "tool" {
		t.Errorf("Role[2]: got %q, want 'tool'", result[2].Role)
	}
	if result[2].ToolCallID != "call_1" {
		t.Errorf("ToolCallID: got %q", result[2].ToolCallID)
	}
	if result[2].Content != "result" {
		t.Errorf("Content[2]: got %q", result[2].Content)
	}
}

func TestToLLMMessagesEmpty(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	result := sess.ToLLMMessages()
	if len(result) != 0 {
		t.Fatalf("expected empty, got %d", len(result))
	}
}

func TestToLLMMessagesNoToolCalls(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleAssistant, Content: "plain response"})

	result := sess.ToLLMMessages()
	if result[0].ToolCalls != nil {
		t.Errorf("expected nil ToolCalls, got %v", result[0].ToolCalls)
	}
}

func TestLastN(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "a"})
	sess.Add(Message{Role: RoleAssistant, Content: "b"})
	sess.Add(Message{Role: RoleUser, Content: "c"})
	sess.Add(Message{Role: RoleAssistant, Content: "d"})

	got := sess.LastN(2)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Content != "c" || got[1].Content != "d" {
		t.Errorf("got %q %q, want 'c' 'd'", got[0].Content, got[1].Content)
	}
}

func TestLastNMoreThanLength(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "a"})
	sess.Add(Message{Role: RoleAssistant, Content: "b"})

	got := sess.LastN(10)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestLastNZero(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "a"})

	got := sess.LastN(0)
	if got != nil {
		t.Errorf("LastN(0) should return nil, got %v", got)
	}
}

func TestLastNEmpty(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	got := sess.LastN(3)
	if got != nil {
		t.Errorf("LastN on empty should return nil, got %v", got)
	}
}
