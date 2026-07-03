package store

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type VoicebotStore struct{ db *gorm.DB }

func NewVoicebotStore(db *gorm.DB) *VoicebotStore { return &VoicebotStore{db: db} }

func (s *VoicebotStore) Create(name, ownerID, configJSON, creator string) (*Voicebot, error) {
	v := &Voicebot{
		ID:         uuid.NewString(),
		Name:       name,
		OwnerID:    ownerID,
		ConfigJSON: configJSON,
		BaseModel:  BaseModel{Creator: creator},
	}
	if err := s.db.Create(v).Error; err != nil {
		return nil, fmt.Errorf("voicebot store: create: %w", err)
	}
	return v, nil
}

func (s *VoicebotStore) ListByOwner(ownerID string) ([]Voicebot, error) {
	var list []Voicebot
	if err := s.db.Where("owner_id = ?", ownerID).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("voicebot store: list: %w", err)
	}
	return list, nil
}

func (s *VoicebotStore) GetByID(id string) (*Voicebot, error) {
	var v Voicebot
	if err := s.db.First(&v, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *VoicebotStore) Update(id, name, configJSON string) (*Voicebot, error) {
	if err := s.db.Model(&Voicebot{}).Where("id = ?", id).
		Updates(map[string]any{"name": name, "config_json": configJSON}).Error; err != nil {
		return nil, fmt.Errorf("voicebot store: update: %w", err)
	}
	return s.GetByID(id)
}

func (s *VoicebotStore) Delete(id string) error {
	if err := s.db.Delete(&Voicebot{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("voicebot store: delete: %w", err)
	}
	return nil
}
