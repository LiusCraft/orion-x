package store

import (
	"time"

	"gorm.io/gorm"
)

type SessionTurn struct {
	ID            int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	DeviceID      string    `gorm:"not null;index:idx_turns_device;type:varchar(128)" json:"device_id"`
	SessionID     string    `gorm:"not null;type:varchar(64)" json:"session_id"`
	TurnID        int64     `gorm:"not null" json:"turn_id"`
	UserText      string    `gorm:"type:text" json:"user_text"`
	AssistantText string    `gorm:"type:text" json:"assistant_text"`
	StartedAt     time.Time `json:"started_at"`
	EndedAt       time.Time `json:"ended_at"`
	Aborted       bool      `gorm:"not null;default:false" json:"aborted"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (SessionTurn) TableName() string { return "session_turns" }

type TurnStore struct{ db *gorm.DB }

func NewTurnStore(db *gorm.DB) *TurnStore { return &TurnStore{db: db} }

func (s *TurnStore) Create(turn *SessionTurn) error {
	return s.db.Create(turn).Error
}

// Search runs FTS via tsvector. Returns matching turns grouped by session.
func (s *TurnStore) Search(deviceID, query string, limit int) ([]SessionTurn, error) {
	var turns []SessionTurn
	err := s.db.Raw(`
        SELECT st.* FROM session_turns st
        WHERE st.device_id = ?
          AND to_tsvector('simple', coalesce(st.user_text,'') || ' ' || coalesce(st.assistant_text,'')) @@
              websearch_to_tsquery('simple', ?)
        ORDER BY st.created_at DESC
        LIMIT ?`, deviceID, query, limit).Scan(&turns).Error
	return turns, err
}

type SessionSummary struct {
	SessionID    string    `json:"session_id"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at"`
	MessageCount int64     `json:"message_count"`
	Preview      string    `json:"preview"`
}

func (s *TurnStore) ListSessions(deviceID string, limit int) ([]SessionSummary, error) {
	var sessions []SessionSummary
	err := s.db.Raw(`
        SELECT session_id,
               MIN(started_at) AS started_at,
               MAX(ended_at) AS ended_at,
               COUNT(*) AS message_count,
               MAX(user_text) AS preview
        FROM session_turns
        WHERE device_id = ?
        GROUP BY session_id
        ORDER BY MAX(created_at) DESC
        LIMIT ?`, deviceID, limit).Scan(&sessions).Error
	return sessions, err
}

func (s *TurnStore) ListBySession(deviceID, sessionID string, limit, offset int) ([]SessionTurn, error) {
	var turns []SessionTurn
	err := s.db.Where("device_id = ? AND session_id = ?", deviceID, sessionID).
		Order("turn_id ASC").Limit(limit).Offset(offset).Find(&turns).Error
	return turns, err
}
