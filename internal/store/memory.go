package store

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MemoryEntry struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	DeviceID  string    `gorm:"not null;index:idx_mem_device_target;type:varchar(128)" json:"device_id"`
	Target    string    `gorm:"not null;type:varchar(16)" json:"target"` // "memory" | "user"
	Content   string    `gorm:"not null;type:text" json:"content"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (MemoryEntry) TableName() string { return "memory_entries" }

type MemoryEntryStore struct{ db *gorm.DB }

func NewMemoryEntryStore(db *gorm.DB) *MemoryEntryStore {
	return &MemoryEntryStore{db: db}
}

func (s *MemoryEntryStore) ListByDevice(deviceID string) ([]MemoryEntry, error) {
	var entries []MemoryEntry
	if err := s.db.Where("device_id = ?", deviceID).
		Order("target, created_at").Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *MemoryEntryStore) ReplaceAll(deviceID string, entries []MemoryEntry) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("device_id = ?", deviceID).Delete(&MemoryEntry{}).Error; err != nil {
			return err
		}
		if len(entries) == 0 {
			return nil
		}
		for i := range entries {
			entries[i].ID = uuid.New().String()
			entries[i].DeviceID = deviceID
		}
		return tx.Create(entries).Error
	})
}
