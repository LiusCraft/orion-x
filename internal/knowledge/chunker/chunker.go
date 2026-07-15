// Package chunker defines the text chunking interface.
// Chunkers split long documents into overlapping text segments suitable
// for embedding and vector retrieval.
package chunker

import "context"

// Chunk is a text segment with positional metadata.
type Chunk struct {
	Index    int
	Content  string
	Metadata map[string]string
}

// Chunker splits a document's text into overlapping chunks.
type Chunker interface {
	Split(ctx context.Context, text string) ([]Chunk, error)
}
