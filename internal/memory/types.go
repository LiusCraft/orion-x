package memory

import (
	"context"
	"time"
)

type Config struct {
	MemoryCharLimit int
	UserCharLimit   int
}

type Context struct {
	UserID    string
	SessionID string
	DeviceID  string
}

type Turn struct {
	TurnID        int64
	UserText      string
	AssistantText string
	ToolsJSON     string `json:"tools_json,omitempty"` // serialized tool call info (name+arguments+result)
	StartedAt     time.Time
	EndedAt       time.Time
	Aborted       bool
	UserID        string
	SessionID     string
	DeviceID      string
}

type contextKey struct{}

// WithContext 将记忆上下文写入 context。
func WithContext(ctx context.Context, memCtx Context) context.Context {
	return context.WithValue(ctx, contextKey{}, memCtx)
}

// FromContext 从 context 读取记忆上下文。
func FromContext(ctx context.Context) (Context, bool) {
	value := ctx.Value(contextKey{})
	if value == nil {
		return Context{}, false
	}
	memCtx, ok := value.(Context)
	return memCtx, ok
}
