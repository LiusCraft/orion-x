package chunker_test

import (
	"context"
	"testing"

	"github.com/liuscraft/orion-x/internal/knowledge/chunker"
)

func TestRecursiveChunker_Split_ShortText(t *testing.T) {
	c := chunker.NewRecursive(chunker.RecursiveConfig{
		ChunkSize:    20,
		ChunkOverlap: 4,
	})
	chunks, err := c.Split(context.Background(), "Hello world. This is a test.")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	for _, ch := range chunks {
		if len([]rune(ch.Content)) > 20 {
			t.Errorf("chunk too large: %d chars > 20: %q", len([]rune(ch.Content)), ch.Content)
		}
	}
}

func TestRecursiveChunker_Split_Empty(t *testing.T) {
	c := chunker.NewRecursive(chunker.RecursiveConfig{})
	chunks, err := c.Split(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks, got %d", len(chunks))
	}
	chunks2, err := c.Split(context.Background(), "   ")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks2) != 0 {
		t.Fatalf("expected 0 chunks for whitespace, got %d", len(chunks2))
	}
}

func TestRecursiveChunker_Split_ParagraphSeparator(t *testing.T) {
	c := chunker.NewRecursive(chunker.RecursiveConfig{
		ChunkSize:    200,
		ChunkOverlap: 20,
	})
	text := "第一段内容。\n\n第二段内容。\n\n第三段内容。"
	chunks, err := c.Split(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks from multi-paragraph text, got %d", len(chunks))
	}
	for i, ch := range chunks {
		if ch.Index != i {
			t.Errorf("chunk %d index mismatch: got %d", i, ch.Index)
		}
	}
}
