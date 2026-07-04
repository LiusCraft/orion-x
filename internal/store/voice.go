package store

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ModelVoiceStore struct {
	db            *gorm.DB
	languageStore *LanguageStore
}

func NewModelVoiceStore(db *gorm.DB, languageStore *LanguageStore) *ModelVoiceStore {
	return &ModelVoiceStore{db: db, languageStore: languageStore}
}

// List 返回指定模型下系统内置 + 当前用户的音色，支持按 lang 过滤
func (s *ModelVoiceStore) List(modelID, userID, lang string) ([]ModelVoice, error) {
	q := s.db.Where("model_id = ? AND (is_system = true OR creator = ?)", modelID, userID)
	if lang != "" {
		q = q.Where("langs @> ?", pq.StringArray{lang})
	}
	var list []ModelVoice
	if err := q.Find(&list).Error; err != nil {
		return nil, fmt.Errorf("voice store: list: %w", err)
	}
	return list, nil
}

// ListAllSystem 返回所有模型下的系统内置音色（音色广场用）
func (s *ModelVoiceStore) ListAllSystem(lang string) ([]ModelVoice, error) {
	q := s.db.Where("is_system = true")
	if lang != "" {
		q = q.Where("langs @> ?", pq.StringArray{lang})
	}
	var list []ModelVoice
	if err := q.Find(&list).Error; err != nil {
		return nil, fmt.Errorf("voice store: list all system: %w", err)
	}
	return list, nil
}

func (s *ModelVoiceStore) GetByID(id string) (*ModelVoice, error) {
	var v ModelVoice
	if err := s.db.First(&v, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *ModelVoiceStore) validateLangs(langs pq.StringArray) error {
	for _, lang := range langs {
		ok, err := s.languageStore.Exists(lang)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("voice store: unknown language: %s", lang)
		}
	}
	return nil
}

type CreateVoiceParams struct {
	ModelID     string
	VoiceID     string
	Name        string
	Description string
	Gender      VoiceGender
	AvatarURL   string
	PreviewURL  string
	Tags        pq.StringArray
	Langs       pq.StringArray
	Emotions    datatypes.JSONMap
	Extra       datatypes.JSONMap
	Creator     string
}

func (s *ModelVoiceStore) Create(p CreateVoiceParams) (*ModelVoice, error) {
	if err := s.validateLangs(p.Langs); err != nil {
		return nil, err
	}
	v := &ModelVoice{
		ID:          uuid.NewString(),
		ModelID:     p.ModelID,
		VoiceID:     p.VoiceID,
		Name:        p.Name,
		Description: p.Description,
		Gender:      p.Gender,
		AvatarURL:   p.AvatarURL,
		PreviewURL:  p.PreviewURL,
		Tags:        p.Tags,
		Langs:       p.Langs,
		Emotions:    p.Emotions,
		IsSystem:    false,
		IsCloned:    false,
		Extra:       p.Extra,
		BaseModel:   BaseModel{Creator: p.Creator},
	}
	if err := s.db.Create(v).Error; err != nil {
		return nil, fmt.Errorf("voice store: create: %w", err)
	}
	return v, nil
}

func (s *ModelVoiceStore) CreateSystem(p CreateVoiceParams) (*ModelVoice, error) {
	if err := s.validateLangs(p.Langs); err != nil {
		return nil, err
	}
	v := &ModelVoice{
		ID:          uuid.NewString(),
		ModelID:     p.ModelID,
		VoiceID:     p.VoiceID,
		Name:        p.Name,
		Description: p.Description,
		Gender:      p.Gender,
		AvatarURL:   p.AvatarURL,
		PreviewURL:  p.PreviewURL,
		Tags:        p.Tags,
		Langs:       p.Langs,
		Emotions:    p.Emotions,
		IsSystem:    true,
		IsCloned:    false,
		Extra:       p.Extra,
		BaseModel:   BaseModel{Creator: p.Creator},
	}
	if err := s.db.Create(v).Error; err != nil {
		return nil, fmt.Errorf("voice store: create system: %w", err)
	}
	return v, nil
}

type CloneVoiceParams struct {
	ModelID        string
	VoiceID        string // 复刻后厂商返回的音色 ID
	Name           string
	SourceAudioURL string
	Langs          pq.StringArray
	Creator        string
}

func (s *ModelVoiceStore) CreateCloned(p CloneVoiceParams) (*ModelVoice, error) {
	if err := s.validateLangs(p.Langs); err != nil {
		return nil, err
	}
	v := &ModelVoice{
		ID:             uuid.NewString(),
		ModelID:        p.ModelID,
		VoiceID:        p.VoiceID,
		Name:           p.Name,
		SourceAudioURL: p.SourceAudioURL,
		Langs:          p.Langs,
		IsSystem:       false,
		IsCloned:       true,
		BaseModel:      BaseModel{Creator: p.Creator},
	}
	if err := s.db.Create(v).Error; err != nil {
		return nil, fmt.Errorf("voice store: create cloned: %w", err)
	}
	return v, nil
}

func (s *ModelVoiceStore) Update(id string, updates map[string]any) (*ModelVoice, error) {
	if langs, ok := updates["langs"]; ok {
		if arr, ok := langs.(pq.StringArray); ok {
			if err := s.validateLangs(arr); err != nil {
				return nil, err
			}
		}
	}
	if err := s.db.Model(&ModelVoice{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("voice store: update: %w", err)
	}
	return s.GetByID(id)
}

func (s *ModelVoiceStore) Delete(id string) error {
	var v ModelVoice
	if err := s.db.First(&v, "id = ?", id).Error; err != nil {
		return err
	}
	if v.IsSystem {
		return ErrSystemRecord
	}
	if err := s.db.Delete(&ModelVoice{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("voice store: delete: %w", err)
	}
	return nil
}
