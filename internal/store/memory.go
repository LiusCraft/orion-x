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

func (s *MemoryEntryStore) GetByID(id string) (*MemoryEntry, error) {
	var e MemoryEntry
	if err := s.db.First(&e, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *MemoryEntryStore) DeleteByID(id string) error {
	return s.db.Delete(&MemoryEntry{}, "id = ?", id).Error
}

// baseQuery builds the common WHERE clause for multi-device queries.
func (s *MemoryEntryStore) baseQuery(deviceIDs []string, target, search string) *gorm.DB {
	query := s.db.Where("device_id IN ?", deviceIDs)
	if target != "" {
		query = query.Where("target = ?", target)
	}
	if search != "" {
		query = query.Where("content LIKE ?", "%"+search+"%")
	}
	return query
}

func (s *MemoryEntryStore) CountByDevices(deviceIDs []string, target, search string) (int64, error) {
	var count int64
	if err := s.baseQuery(deviceIDs, target, search).Model(&MemoryEntry{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *MemoryEntryStore) CountByDevice(deviceID, target, search string) (int64, error) {
	return s.CountByDevices([]string{deviceID}, target, search)
}

func (s *MemoryEntryStore) ListByDevices(deviceIDs []string, target, search string) ([]MemoryEntry, error) {
	var entries []MemoryEntry
	if err := s.baseQuery(deviceIDs, target, search).
		Order("created_at DESC").Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *MemoryEntryStore) ListByDevicePaginated(deviceID, target, search string, page, pageSize int) ([]MemoryEntry, int64, error) {
	count, err := s.CountByDevice(deviceID, target, search)
	if err != nil {
		return nil, 0, err
	}
	if count == 0 {
		return []MemoryEntry{}, 0, nil
	}
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}
	var entries []MemoryEntry
	if err := s.baseQuery([]string{deviceID}, target, search).
		Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&entries).Error; err != nil {
		return nil, 0, err
	}
	return entries, count, nil
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
