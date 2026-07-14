package chunker

import (
	"context"
	"strings"
	"unicode/utf8"
)

var defaultSeparators = []string{"\n\n", "\n", "。", ".", " ", ""}

// RecursiveConfig configures the recursive character splitter.
type RecursiveConfig struct {
	ChunkSize    int
	ChunkOverlap int
	Separators   []string
}

func (c *RecursiveConfig) normalize() {
	if c.ChunkSize <= 0 {
		c.ChunkSize = 512
	}
	if c.ChunkOverlap <= 0 {
		c.ChunkOverlap = 80
	}
	if len(c.Separators) == 0 {
		c.Separators = defaultSeparators
	}
}

// RecursiveChunker splits text by recursively trying smaller separators,
// falling back to character-level splitting when no separator works.
type RecursiveChunker struct {
	cfg RecursiveConfig
}

// NewRecursive creates a RecursiveChunker with the given config.
func NewRecursive(cfg RecursiveConfig) *RecursiveChunker {
	cfg.normalize()
	return &RecursiveChunker{cfg: cfg}
}

func (c *RecursiveChunker) Split(_ context.Context, text string) ([]Chunk, error) {
	if len(strings.TrimSpace(text)) == 0 {
		return nil, nil
	}
	var chunks []Chunk
	c.splitRecursive(strings.TrimSpace(text), 0, &chunks)
	for i := range chunks {
		chunks[i].Index = i
	}
	return chunks, nil
}

func (c *RecursiveChunker) splitRecursive(text string, sepIdx int, chunks *[]Chunk) {
	if sepIdx >= len(c.cfg.Separators) {
		c.hardSplit(text, chunks)
		return
	}
	sep := c.cfg.Separators[sepIdx]
	if sep == "" {
		c.hardSplit(text, chunks)
		return
	}
	parts := strings.Split(text, sep)
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if utf8.RuneCountInString(trimmed) <= c.cfg.ChunkSize {
			*chunks = append(*chunks, Chunk{Content: trimmed})
		} else {
			c.splitRecursive(trimmed, sepIdx+1, chunks)
		}
	}
}

func (c *RecursiveChunker) hardSplit(text string, chunks *[]Chunk) {
	runes := []rune(text)
	step := c.cfg.ChunkSize - c.cfg.ChunkOverlap
	if step <= 0 {
		step = c.cfg.ChunkSize
	}
	for i := 0; i < len(runes); i += step {
		end := i + c.cfg.ChunkSize
		if end > len(runes) {
			end = len(runes)
		}
		content := strings.TrimSpace(string(runes[i:end]))
		if content != "" {
			*chunks = append(*chunks, Chunk{Content: content})
		}
		if end >= len(runes) {
			break
		}
	}
}
