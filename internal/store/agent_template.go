package store

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type AgentTemplateStore struct {
	db *gorm.DB
}

func NewAgentTemplateStore(db *gorm.DB) *AgentTemplateStore {
	return &AgentTemplateStore{db: db}
}

// ListSystem 返回所有系统内置模板（智能体广场用），支持按分类和关键词过滤。
func (s *AgentTemplateStore) ListSystem(category, query string) ([]AgentTemplate, error) {
	q := s.db.Where("is_system = true")
	if category != "" {
		q = q.Where("category = ?", category)
	}
	if query != "" {
		q = q.Where("name ILIKE ? OR description ILIKE ?", "%"+query+"%", "%"+query+"%")
	}
	var list []AgentTemplate
	if err := q.Order("use_count DESC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("agent template store: list system: %w", err)
	}
	return list, nil
}

// GetByID 返回模板详情。
func (s *AgentTemplateStore) GetByID(id string) (*AgentTemplate, error) {
	var t AgentTemplate
	if err := s.db.First(&t, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// IncrementUse 递增使用次数并返回最新的模板（用于基于模板创建 voicebot）。
func (s *AgentTemplateStore) IncrementUse(id string) (*AgentTemplate, error) {
	if err := s.db.Model(&AgentTemplate{}).Where("id = ?", id).
		UpdateColumn("use_count", gorm.Expr("use_count + 1")).Error; err != nil {
		return nil, fmt.Errorf("agent template store: increment use: %w", err)
	}
	return s.GetByID(id)
}

type CreateTemplateParams struct {
	Name        string
	Description string
	Icon        string
	Color       string
	Category    string
	Tags        pq.StringArray
	ConfigJSON  string
	IsSystem    bool
	Creator     string
}

func (s *AgentTemplateStore) Create(p CreateTemplateParams) (*AgentTemplate, error) {
	t := &AgentTemplate{
		ID:          uuid.NewString(),
		Name:        p.Name,
		Description: p.Description,
		Icon:        p.Icon,
		Color:       p.Color,
		Category:    p.Category,
		Tags:        p.Tags,
		ConfigJSON:  p.ConfigJSON,
		IsSystem:    p.IsSystem,
		BaseModel:   BaseModel{Creator: p.Creator},
	}
	if err := s.db.Create(t).Error; err != nil {
		return nil, fmt.Errorf("agent template store: create: %w", err)
	}
	return t, nil
}

func (s *AgentTemplateStore) Update(id string, updates map[string]any) (*AgentTemplate, error) {
	if err := s.db.Model(&AgentTemplate{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("agent template store: update: %w", err)
	}
	return s.GetByID(id)
}

func (s *AgentTemplateStore) Delete(id string) error {
	var t AgentTemplate
	if err := s.db.First(&t, "id = ?", id).Error; err != nil {
		return err
	}
	if t.IsSystem {
		return ErrSystemRecord
	}
	if err := s.db.Delete(&AgentTemplate{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("agent template store: delete: %w", err)
	}
	return nil
}
