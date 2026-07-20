package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/pkg/pipeline"
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
	ID              string               `json:"id"`
	Role            Role                 `json:"role"`
	Content         string               `json:"content"`
	ToolCallID      string               `json:"tool_call_id,omitempty"`
	ToolCalls       []ToolCall           `json:"tool_calls,omitempty"`
	ProviderContext *llm.ProviderContext `json:"provider_context,omitempty"`
	Status          string               `json:"status,omitempty"`
	CreatedAt       time.Time            `json:"created_at"`
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

	// Pipeline is assigned by a channel after it has assembled the session's
	// processing graph. It is intentionally not serialized with chat history.
	Pipeline pipeline.Pipeline `json:"-"`
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

// PopN removes up to n messages from the tail and returns them in original order.
// Returns nil if the session has no messages or n <= 0.
func (s *Session) PopN(n int) []Message {
	if n <= 0 || len(s.Messages) == 0 {
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

// ToLLMMessages converts session messages to LLM request format.
func (s *Session) ToLLMMessages() []llm.Message {
	result := make([]llm.Message, len(s.Messages))
	for i, msg := range s.Messages {
		result[i] = llm.Message{
			Role:            string(msg.Role),
			Content:         msg.Content,
			ToolCallID:      msg.ToolCallID,
			ProviderContext: cloneProviderContext(msg.ProviderContext),
		}
		if len(msg.ToolCalls) > 0 {
			result[i].ToolCalls = make([]llm.ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				result[i].ToolCalls[j] = llm.ToolCall{
					ID:        tc.ID,
					Name:      tc.Name,
					Arguments: tc.Arguments,
				}
			}
		}
	}
	return result
}

// LastN returns the last n messages without modifying the session.
// Returns nil if n <= 0 or the session is empty.
func (s *Session) LastN(n int) []Message {
	if n <= 0 || len(s.Messages) == 0 {
		return nil
	}
	if n > len(s.Messages) {
		n = len(s.Messages)
	}
	start := len(s.Messages) - n
	result := make([]Message, n)
	copy(result, s.Messages[start:])
	return result
}

// Clone returns a deep copy of the session. Modifying the clone does not
// affect the original.
func (s *Session) Clone() *Session {
	cloned := *s

	if s.Messages != nil {
		cloned.Messages = make([]Message, len(s.Messages))
		copy(cloned.Messages, s.Messages)
		for i := range cloned.Messages {
			if len(s.Messages[i].ToolCalls) > 0 {
				cloned.Messages[i].ToolCalls = make([]ToolCall, len(s.Messages[i].ToolCalls))
				copy(cloned.Messages[i].ToolCalls, s.Messages[i].ToolCalls)
			}
			cloned.Messages[i].ProviderContext = cloneProviderContext(s.Messages[i].ProviderContext)
		}
	}
	return &cloned
}

func cloneProviderContext(ctx *llm.ProviderContext) *llm.ProviderContext {
	if ctx == nil {
		return nil
	}
	cloned := *ctx
	cloned.Data = append([]byte(nil), ctx.Data...)
	return &cloned
}

func newID(prefix string) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf)
}
