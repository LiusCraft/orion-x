package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// Role represents the role of a chat message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall represents a tool call request from the assistant.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Message represents a single chat message in a session.
type Message struct {
	ID         string     `json:"id"`
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Status     string     `json:"status,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// SessionMeta holds metadata about a session.
type SessionMeta struct {
	UserID    string    `json:"user_id"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
}

// Session represents a complete chat conversation history.
type Session struct {
	ID        string      `json:"id"`
	Meta      SessionMeta `json:"meta"`
	Messages  []Message   `json:"messages"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// New creates a new session with auto-generated ID and timestamp defaults.
func New(meta SessionMeta) *Session {
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = time.Now().UTC()
	}
	now := time.Now().UTC()
	return &Session{
		ID:        newID("sess"),
		Meta:      meta,
		Messages:  make([]Message, 0, 16),
		UpdatedAt: now,
	}
}

// Add appends a message to the session, auto-generating ID and timestamp if needed.
func (s *Session) Add(msg Message) {
	if msg.ID == "" {
		msg.ID = newID("msg")
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now().UTC()
}

// Pop removes and returns the last message. Returns false if the session is empty.
func (s *Session) Pop() (Message, bool) {
	if len(s.Messages) == 0 {
		return Message{}, false
	}
	last := s.Messages[len(s.Messages)-1]
	s.Messages = s.Messages[:len(s.Messages)-1]
	s.UpdatedAt = time.Now().UTC()
	return last, true
}

// PopN removes the last n messages and returns them in original order.
func (s *Session) PopN(n int) []Message {
	if n <= 0 {
		return nil
	}
	if n > len(s.Messages) {
		n = len(s.Messages)
	}
	idx := len(s.Messages) - n
	removed := make([]Message, n)
	copy(removed, s.Messages[idx:])
	s.Messages = s.Messages[:idx]
	s.UpdatedAt = time.Now().UTC()
	return removed
}

func newID(prefix string) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf)
}
