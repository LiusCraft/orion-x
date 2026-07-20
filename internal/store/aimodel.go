package store

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type AIModelStore struct{ db *gorm.DB }

func NewAIModelStore(db *gorm.DB) *AIModelStore { return &AIModelStore{db: db} }

// List 返回系统内置 + 当前用户自建的模型，支持按 type 和 lang 过滤
func (s *AIModelStore) List(userID string, modelType ModelType, lang string) ([]AIModel, error) {
	q := s.db.Preload("Provider").Where("is_system = true OR creator = ?", userID)
	if modelType != "" {
		q = q.Where("type = ?", modelType)
	}
	if lang != "" {
		q = q.Where("langs @> ?", pq.StringArray{lang})
	}
	var list []AIModel
	if err := q.Find(&list).Error; err != nil {
		return nil, fmt.Errorf("ai_model store: list: %w", err)
	}
	return list, nil
}

func (s *AIModelStore) GetByID(id string) (*AIModel, error) {
	var m AIModel
	if err := s.db.Preload("Provider").First(&m, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *AIModelStore) Create(providerID, name string, modelType ModelType, baseURL, modelID, creator string, extra datatypes.JSONMap) (*AIModel, error) {
	m := &AIModel{
		ID:         uuid.NewString(),
		ProviderID: providerID,
		Name:       name,
		Type:       modelType,
		BaseURL:    baseURL,
		ModelID:    modelID,
		IsSystem:   false,
		Extra:      extra,
		BaseModel:  BaseModel{Creator: creator},
	}
	if err := s.db.Create(m).Error; err != nil {
		return nil, fmt.Errorf("ai_model store: create: %w", err)
	}
	return s.GetByID(m.ID)
}

func (s *AIModelStore) Update(id string, updates map[string]any) (*AIModel, error) {
	if err := s.db.Model(&AIModel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("ai_model store: update: %w", err)
	}
	return s.GetByID(id)
}

func (s *AIModelStore) Delete(id string, allowSystem bool) error {
	var m AIModel
	if err := s.db.First(&m, "id = ?", id).Error; err != nil {
		return err
	}
	if m.IsSystem && !allowSystem {
		return ErrSystemRecord
	}
	if err := s.db.Delete(&AIModel{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("ai_model store: delete: %w", err)
	}
	return nil
}
