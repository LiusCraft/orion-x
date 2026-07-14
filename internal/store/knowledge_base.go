package store

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// KnowledgeBase 知识库 —— 一个智能体下可有多个
type KnowledgeBase struct {
	ID               string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	VoicebotID       string    `gorm:"not null;index;type:varchar(36)" json:"voicebot_id"`
	Name             string    `gorm:"not null;type:varchar(128)" json:"name"`
	Description      string    `gorm:"type:text" json:"description,omitempty"`
	EmbeddingModelID string    `gorm:"type:varchar(36)" json:"embedding_model_id,omitempty"` // AIModel.ID，空表示未配置
	EmbeddingDim     int       `gorm:"not null;default:0" json:"embedding_dim"`
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (KnowledgeBase) TableName() string { return "knowledge_bases" }

// KnowledgeBaseStore 知识库持久化
type KnowledgeBaseStore struct{ db *gorm.DB }

func NewKnowledgeBaseStore(db *gorm.DB) *KnowledgeBaseStore {
	return &KnowledgeBaseStore{db: db}
}

func (s *KnowledgeBaseStore) Create(kb *KnowledgeBase) error {
	if kb.ID == "" {
		kb.ID = uuid.New().String()
	}
	return s.db.Create(kb).Error
}

func (s *KnowledgeBaseStore) GetByID(id string) (*KnowledgeBase, error) {
	var kb KnowledgeBase
	if err := s.db.First(&kb, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &kb, nil
}

func (s *KnowledgeBaseStore) ListByVoicebot(voicebotID string) ([]KnowledgeBase, error) {
	var kbs []KnowledgeBase
	if err := s.db.Where("voicebot_id = ?", voicebotID).
		Order("created_at DESC").Find(&kbs).Error; err != nil {
		return nil, err
	}
	return kbs, nil
}

func (s *KnowledgeBaseStore) DeleteByID(id string) error {
	return s.db.Delete(&KnowledgeBase{}, "id = ?", id).Error
}
