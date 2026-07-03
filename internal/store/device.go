package store

import (
	"fmt"

	"gorm.io/gorm"
)

type DeviceStore struct{ db *gorm.DB }

func NewDeviceStore(db *gorm.DB) *DeviceStore { return &DeviceStore{db: db} }

func (s *DeviceStore) Create(id, voicebotID, name, creator string) (*Device, error) {
	d := &Device{
		ID:         id,
		VoicebotID: voicebotID,
		Name:       name,
		BaseModel:  BaseModel{Creator: creator},
	}
	if err := s.db.Create(d).Error; err != nil {
		return nil, fmt.Errorf("device store: create: %w", err)
	}
	return d, nil
}

func (s *DeviceStore) ListByVoicebot(voicebotID string) ([]Device, error) {
	var list []Device
	if err := s.db.Where("voicebot_id = ?", voicebotID).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("device store: list: %w", err)
	}
	return list, nil
}

func (s *DeviceStore) GetByID(id string) (*Device, error) {
	var d Device
	if err := s.db.First(&d, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *DeviceStore) Delete(id string) error {
	if err := s.db.Delete(&Device{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("device store: delete: %w", err)
	}
	return nil
}
