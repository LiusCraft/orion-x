package store

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ProviderStore struct{ db *gorm.DB }

func NewProviderStore(db *gorm.DB) *ProviderStore { return &ProviderStore{db: db} }

// List 返回系统内置 + 当前用户自建的厂商
func (s *ProviderStore) List(userID string) ([]Provider, error) {
	var list []Provider
	if err := s.db.Where("is_system = true OR creator = ?", userID).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("provider store: list: %w", err)
	}
	return list, nil
}

func (s *ProviderStore) GetByID(id string) (*Provider, error) {
	var p Provider
	if err := s.db.First(&p, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *ProviderStore) Create(name, slug, baseURL, apiKeyEnc, creator string, extra datatypes.JSONMap) (*Provider, error) {
	p := &Provider{
		ID:        uuid.NewString(),
		Name:      name,
		Slug:      slug,
		BaseURL:   baseURL,
		APIKeyEnc: apiKeyEnc,
		IsSystem:  false,
		Extra:     extra,
		BaseModel: BaseModel{Creator: creator},
	}
	if err := s.db.Create(p).Error; err != nil {
		return nil, fmt.Errorf("provider store: create: %w", err)
	}
	return p, nil
}

func (s *ProviderStore) Update(id string, updates map[string]any) (*Provider, error) {
	if err := s.db.Model(&Provider{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("provider store: update: %w", err)
	}
	return s.GetByID(id)
}

// Delete 拒绝删除系统内置厂商
func (s *ProviderStore) Delete(id string) error {
	var p Provider
	if err := s.db.First(&p, "id = ?", id).Error; err != nil {
		return err
	}
	if p.IsSystem {
		return ErrSystemRecord
	}
	if err := s.db.Delete(&Provider{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("provider store: delete: %w", err)
	}
	return nil
}
