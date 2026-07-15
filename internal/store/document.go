package store

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Document 知识库中的文档
type Document struct {
	ID              string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	KnowledgeBaseID string    `gorm:"not null;index;type:varchar(36)" json:"knowledge_base_id"`
	Name            string    `gorm:"not null;type:varchar(256)" json:"name"`
	Source          string    `gorm:"not null;type:varchar(16)" json:"source"`                 // "file" | "url"
	SourceURL       string    `gorm:"type:text" json:"source_url,omitempty"`                   // URL 来源（source=url 时）
	Status          string    `gorm:"not null;default:pending;type:varchar(16)" json:"status"` // pending/parsing/chunking/embedding/storing/ready/error
	ChunkCount      int       `gorm:"default:0" json:"chunk_count"`
	CharCount       int       `gorm:"default:0" json:"char_count"`
	ErrorMessage    string    `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt       time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Document) TableName() string { return "documents" }

// DocumentStore 文档持久化
type DocumentStore struct{ db *gorm.DB }

func NewDocumentStore(db *gorm.DB) *DocumentStore {
	return &DocumentStore{db: db}
}

func (s *DocumentStore) Create(doc *Document) error {
	if doc.ID == "" {
		doc.ID = uuid.New().String()
	}
	return s.db.Create(doc).Error
}

func (s *DocumentStore) GetByID(id string) (*Document, error) {
	var doc Document
	if err := s.db.First(&doc, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &doc, nil
}

func (s *DocumentStore) ListByKB(kbID string) ([]Document, error) {
	var docs []Document
	if err := s.db.Where("knowledge_base_id = ?", kbID).
		Order("created_at DESC").Find(&docs).Error; err != nil {
		return nil, err
	}
	return docs, nil
}

func (s *DocumentStore) UpdateStatus(id, status, errMsg string) error {
	return s.db.Model(&Document{}).Where("id = ?", id).Updates(map[string]any{
		"status":        status,
		"error_message": errMsg,
	}).Error
}

func (s *DocumentStore) UpdateChunkInfo(id string, chunkCount, charCount int) error {
	return s.db.Model(&Document{}).Where("id = ?", id).Updates(map[string]any{
		"chunk_count": chunkCount,
		"char_count":  charCount,
	}).Error
}

func (s *DocumentStore) DeleteByID(id string) error {
	return s.db.Delete(&Document{}, "id = ?", id).Error
}
