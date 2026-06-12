package memory

import (
	"context"
	"time"

	"github.com/liuscraft/orion-x/internal/llm"
)

type Mode string

const (
	ModeNone     Mode = "none"
	ModeSession  Mode = "session"
	ModeLongTerm Mode = "long_term"
)

type Config struct {
	Mode                 Mode
	SessionMaxTurns      int
	SessionSummaryEveryN int
	LongTermDBPath       string
	LongTermMaxResults   int
	RetentionDays        int
	FTSMinScore          float64
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
	StartedAt     time.Time
	EndedAt       time.Time
	Aborted       bool

	UserID    string
	SessionID string
	DeviceID  string
}

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

type Store interface {
	SaveTurn(turn Turn) error
	SaveItems(items []MemoryItem) error
	Query(userID, query string, limit int, minScore float64) ([]MemoryItem, error)
	Purge(now time.Time, retentionDays int) error
	Close() error
}

type Service interface {
	BuildContextMessages(ctx context.Context, userText string) ([]*llm.Message, error)
	RecordTurn(ctx context.Context, turn Turn) error
	Close() error
}
