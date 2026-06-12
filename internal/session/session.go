package session

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Message struct {
	ID         string     `json:"id"`
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Status     string     `json:"status,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type SessionMeta struct {
	UserID    string    `json:"user_id"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
}

type Session struct {
	ID        string      `json:"id"`
	Meta      SessionMeta `json:"meta"`
	Messages  []Message   `json:"messages"`
	UpdatedAt time.Time   `json:"updated_at"`
}

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

func (s *Session) Add(msg Message) {
	if msg.ID == "" {
		msg.ID = newID("msg")
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = msg.CreatedAt
}

func newID(prefix string) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return prefix + "_fallback"
	}
	return prefix + "_" + hex.EncodeToString(buf)
}
