package store

import (
	"fmt"

	"gorm.io/gorm"
)

type LanguageStore struct{ db *gorm.DB }

func NewLanguageStore(db *gorm.DB) *LanguageStore { return &LanguageStore{db: db} }

func (s *LanguageStore) List(parentCode string) ([]Language, error) {
	q := s.db.Order("code ASC")
	if parentCode == "null" {
		q = q.Where("parent_code IS NULL")
	} else if parentCode != "" {
		q = q.Where("parent_code = ?", parentCode)
	}
	var list []Language
	if err := q.Find(&list).Error; err != nil {
		return nil, fmt.Errorf("language store: list: %w", err)
	}
	return list, nil
}

func (s *LanguageStore) GetByCode(code string) (*Language, error) {
	var lang Language
	if err := s.db.Preload("Children").First(&lang, "code = ?", code).Error; err != nil {
		return nil, err
	}
	return &lang, nil
}

func (s *LanguageStore) Exists(code string) (bool, error) {
	var count int64
	if err := s.db.Model(&Language{}).Where("code = ?", code).Count(&count).Error; err != nil {
		return false, fmt.Errorf("language store: exists: %w", err)
	}
	return count > 0, nil
}

type CreateLanguageParams struct {
	Code       string
	Name       string
	ParentCode *string
}

func (s *LanguageStore) Create(p CreateLanguageParams) (*Language, error) {
	lang := &Language{
		Code:       p.Code,
		Name:       p.Name,
		ParentCode: p.ParentCode,
	}
	if err := s.db.Create(lang).Error; err != nil {
		return nil, fmt.Errorf("language store: create: %w", err)
	}
	return lang, nil
}

func (s *LanguageStore) Update(code string, updates map[string]any) (*Language, error) {
	if err := s.db.Model(&Language{}).Where("code = ?", code).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("language store: update: %w", err)
	}
	return s.GetByCode(code)
}

func (s *LanguageStore) Delete(code string) error {
	var lang Language
	if err := s.db.First(&lang, "code = ?", code).Error; err != nil {
		return err
	}
	if err := s.db.Where("parent_code = ?", code).Delete(&Language{}).Error; err != nil {
		return fmt.Errorf("language store: delete children: %w", err)
	}
	if err := s.db.Delete(&Language{}, "code = ?", code).Error; err != nil {
		return fmt.Errorf("language store: delete: %w", err)
	}
	return nil
}
