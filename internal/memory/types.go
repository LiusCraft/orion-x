package memory

import (
	"context"
	"time"

	"github.com/cloudwego/eino/schema"
)

// Mode 表示记忆模式。
type Mode string

const (
	ModeNone     Mode = "none"
	ModeSession  Mode = "session"
	ModeLongTerm Mode = "long_term"
)

// Config 记忆配置。
type Config struct {
	Mode                 Mode
	SessionMaxTurns      int
	SessionSummaryEveryN int
	LongTermDBPath       string
	LongTermMaxResults   int
	RetentionDays        int
	FTSMinScore          float64
}

// Context 会话上下文。
type Context struct {
	UserID    string
	SessionID string
	DeviceID  string
}

// Turn 表示一次完整对话。
type Turn struct {
	TurnID        int64
	UserText      string
	AssistantText string
	StartedAt     time.Time
	EndedAt       time.Time
	Aborted       bool

	UserID    string
	SessionID string
	DeviceID  string
}

// MemoryItem 表示长期记忆条目。
type MemoryItem struct {
	ID         int64
	UserID     string
	Content    string
	Type       string
	Importance int
	CreatedAt  time.Time
	ExpiresAt  *time.Time
	Score      float64
}

// Store 持久化存储接口。
type Store interface {
	SaveTurn(turn Turn) error
	SaveItems(items []MemoryItem) error
	Query(userID, query string, limit int, minScore float64) ([]MemoryItem, error)
	Purge(now time.Time, retentionDays int) error
	Close() error
}

// Service 记忆服务接口。
type Service interface {
	BuildContextMessages(ctx context.Context, userText string) ([]*schema.Message, error)
	RecordTurn(ctx context.Context, turn Turn) error
	Close() error
}
