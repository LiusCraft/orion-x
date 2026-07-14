// Package retriever defines the vector store and retrieval interface.
// Implementations store chunk embeddings and perform similarity search.
package retriever

import "context"

// SearchResult is a single retrieved chunk with score and metadata.
type SearchResult struct {
	ChunkID      string            `json:"chunk_id"`
	Content      string            `json:"content"`
	Score        float64           `json:"score"`
	DocumentID   string            `json:"document_id"`
	DocumentName string            `json:"document_name"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Chunk carries the content and metadata of a text segment to be indexed.
// It mirrors chunker.Chunk to keep the retriever interface self-contained.
type Chunk struct {
	Index    int
	Content  string
	Metadata map[string]string
}

// Retriever stores chunk embeddings and performs similarity search.
type Retriever interface {
	// Insert saves chunks and their embeddings, associating them with a document in a knowledge base.
	Insert(ctx context.Context, kbID, docID string, chunks []Chunk, vectors [][]float32) error

	// DeleteByKB removes all chunks and embeddings for a knowledge base.
	DeleteByKB(ctx context.Context, kbID string) error

	// DeleteByDocument removes all chunks and embeddings for a specific document.
	DeleteByDocument(ctx context.Context, docID string) error

	// Search performs cosine similarity search across the given knowledge bases.
	Search(ctx context.Context, kbIDs []string, vector []float32, topK int) ([]SearchResult, error)
}
