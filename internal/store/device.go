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

// ListWithTGBot 返回所有配置了 tg_bot_token 的设备（tg_bot_token 非空）。
func (s *DeviceStore) ListWithTGBot() ([]Device, error) {
	var list []Device
	if err := s.db.Where("tg_bot_token IS NOT NULL AND tg_bot_token != ''").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("device store: list with tg bot: %w", err)
	}
	return list, nil
}

// SetTgBotToken 设置/清除设备的 tg_bot_token。
func (s *DeviceStore) SetTgBotToken(deviceID, token string) error {
	if err := s.db.Model(&Device{}).Where("id = ?", deviceID).Update("tg_bot_token", token).Error; err != nil {
		return fmt.Errorf("device store: set tg bot token: %w", err)
	}
	return nil
}
