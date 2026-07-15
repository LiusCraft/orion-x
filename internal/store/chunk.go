package store

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Chunk 文档分块 —— 纯元数据，不含向量列。
// 向量由 retriever 层各自管理（pgvector 自己建表 + 索引），store 层永远干净。
type Chunk struct {
	ID              string            `gorm:"primaryKey;type:varchar(36)" json:"id"`
	DocumentID      string            `gorm:"not null;index;type:varchar(36)" json:"document_id"`
	KnowledgeBaseID string            `gorm:"not null;index;type:varchar(36)" json:"knowledge_base_id"`
	ChunkIndex      int               `gorm:"not null" json:"chunk_index"`
	Content         string            `gorm:"not null;type:text" json:"content"`
	Metadata        datatypes.JSONMap `gorm:"type:jsonb;default:'{}'" json:"metadata,omitempty"`
	CreatedAt       time.Time         `gorm:"autoCreateTime" json:"created_at"`
}

func (Chunk) TableName() string { return "chunks" }

// ChunkStore 分块持久化
type ChunkStore struct{ db *gorm.DB }

func NewChunkStore(db *gorm.DB) *ChunkStore {
	return &ChunkStore{db: db}
}

func (s *ChunkStore) BatchCreate(chunks []*Chunk) error {
	for _, c := range chunks {
		if c.ID == "" {
			c.ID = uuid.New().String()
		}
	}
	return s.db.CreateInBatches(chunks, 100).Error
}

func (s *ChunkStore) DeleteByDocument(docID string) error {
	return s.db.Where("document_id = ?", docID).Delete(&Chunk{}).Error
}

func (s *ChunkStore) DeleteByKB(kbID string) error {
	return s.db.Where("knowledge_base_id = ?", kbID).Delete(&Chunk{}).Error
}

func (s *ChunkStore) GetByIDs(ids []string) ([]Chunk, error) {
	var chunks []Chunk
	if err := s.db.Where("id IN ?", ids).Find(&chunks).Error; err != nil {
		return nil, err
	}
	return chunks, nil
}
