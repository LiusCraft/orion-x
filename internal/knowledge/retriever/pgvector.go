package retriever

import (
	"context"
	"fmt"
	"strings"

	"github.com/liuscraft/orion-x/internal/store"
	"gorm.io/gorm"
)

// PGVectorRetriever implements Retriever using PostgreSQL with pgvector + halfvec + HNSW.
// It manages its own chunk_vectors table, separate from the store-layer Chunk model.
type PGVectorRetriever struct {
	db         *gorm.DB
	chunkStore *store.ChunkStore
	dimension  int
}

// NewPGVector creates a new pgvector retriever with the given embedding dimension.
// It ensures the vector extension and chunk_vectors table exist.
func NewPGVector(db *gorm.DB, dimension int) (*PGVectorRetriever, error) {
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		return nil, fmt.Errorf("pgvector: create extension: %w", err)
	}
	_ = dimension
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS chunk_vectors (
			chunk_id   VARCHAR(36) PRIMARY KEY REFERENCES chunks(id) ON DELETE CASCADE,
			kb_id      VARCHAR(36) NOT NULL,
			embedding  halfvec,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)
	`).Error; err != nil {
		return nil, fmt.Errorf("pgvector: create table: %w", err)
	}
	// Migrate existing tables that may have been created with a fixed dimension
	_ = db.Exec(`ALTER TABLE chunk_vectors ALTER COLUMN embedding TYPE halfvec`).Error
	db.Exec("CREATE INDEX IF NOT EXISTS idx_chunk_vectors_kb ON chunk_vectors(kb_id)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_chunk_vectors_embedding ON chunk_vectors " +
		"USING hnsw (embedding halfvec_cosine_ops) WITH (m = 16, ef_construction = 200)")

	return &PGVectorRetriever{
		db:         db,
		chunkStore: store.NewChunkStore(db),
		dimension:  dimension,
	}, nil
}

// Insert saves chunks and their vectors in a single transaction.
func (r *PGVectorRetriever) Insert(ctx context.Context, kbID, docID string, chunks []Chunk, vectors [][]float32) error {
	if len(chunks) != len(vectors) {
		return fmt.Errorf("pgvector: chunks/vectors length mismatch: %d vs %d", len(chunks), len(vectors))
	}
	if len(chunks) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Insert chunk metadata
		storeChunks := make([]*store.Chunk, len(chunks))
		for i, c := range chunks {
			storeChunks[i] = &store.Chunk{
				DocumentID:      docID,
				KnowledgeBaseID: kbID,
				ChunkIndex:      c.Index,
				Content:         c.Content,
			}
		}
		if err := store.NewChunkStore(tx).BatchCreate(storeChunks); err != nil {
			return fmt.Errorf("pgvector: insert chunks: %w", err)
		}

		// 2. Insert vectors
		for i, c := range storeChunks {
			vecStr := float32ToHalfVec(vectors[i])
			if err := tx.Exec(
				"INSERT INTO chunk_vectors (chunk_id, kb_id, embedding) VALUES (?, ?, ?::halfvec)",
				c.ID, kbID, vecStr,
			).Error; err != nil {
				return fmt.Errorf("pgvector: insert vector: %w", err)
			}
		}
		return nil
	})
}

// DeleteByKB removes all chunks and vectors for a knowledge base.
func (r *PGVectorRetriever) DeleteByKB(ctx context.Context, kbID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM chunk_vectors WHERE kb_id = ?", kbID).Error; err != nil {
			return err
		}
		return store.NewChunkStore(tx).DeleteByKB(kbID)
	})
}

// DeleteByDocument removes all chunks and vectors for a specific document.
func (r *PGVectorRetriever) DeleteByDocument(ctx context.Context, docID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var chunkIDs []string
		if err := tx.Raw("SELECT id FROM chunks WHERE document_id = ?", docID).Scan(&chunkIDs).Error; err != nil {
			return err
		}
		if len(chunkIDs) > 0 {
			if err := tx.Exec("DELETE FROM chunk_vectors WHERE chunk_id IN ?", chunkIDs).Error; err != nil {
				return err
			}
		}
		return store.NewChunkStore(tx).DeleteByDocument(docID)
	})
}

// Search performs cosine similarity search with HNSW acceleration.
func (r *PGVectorRetriever) Search(ctx context.Context, kbIDs []string, vector []float32, topK int) ([]SearchResult, error) {
	if len(kbIDs) == 0 {
		return nil, nil
	}
	if topK <= 0 {
		topK = 5
	}

	vecStr := float32ToHalfVec(vector)

	type row struct {
		ChunkID      string  `gorm:"column:chunk_id"`
		Content      string  `gorm:"column:content"`
		Score        float64 `gorm:"column:score"`
		DocumentID   string  `gorm:"column:document_id"`
		DocumentName string  `gorm:"column:document_name"`
	}
	var rows []row

	sql := `
		SELECT cv.chunk_id, c.content,
		       1 - (cv.embedding <=> ?::halfvec) AS score,
		       c.document_id, d.name AS document_name
		FROM chunk_vectors cv
		JOIN chunks c ON c.id = cv.chunk_id
		JOIN documents d ON d.id = c.document_id
		WHERE cv.kb_id IN ?
		ORDER BY cv.embedding <=> ?::halfvec
		LIMIT ?
	`
	if err := r.db.WithContext(ctx).Raw(sql, vecStr, kbIDs, vecStr, topK*2).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("pgvector: search: %w", err)
	}

	results := make([]SearchResult, len(rows))
	for i, r := range rows {
		results[i] = SearchResult{
			ChunkID:      r.ChunkID,
			Content:      r.Content,
			Score:        r.Score,
			DocumentID:   r.DocumentID,
			DocumentName: r.DocumentName,
		}
	}
	return results, nil
}

// float32ToHalfVec formats a float32 slice as a PostgreSQL halfvec literal: [1,2,3,...]
func float32ToHalfVec(v []float32) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%f", f)
	}
	sb.WriteByte(']')
	return sb.String()
}
