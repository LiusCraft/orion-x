package memory

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStoreSaveQueryPurge(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	expired := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	items := []MemoryItem{
		{
			UserID:     "u1",
			Content:    "user likes apples",
			Type:       "preference",
			Importance: 4,
			CreatedAt:  now,
			ExpiresAt:  &expired,
		},
		{
			UserID:     "u1",
			Content:    "user lives in shanghai",
			Type:       "fact",
			Importance: 3,
			CreatedAt:  now.Add(time.Second),
			ExpiresAt:  &future,
		},
	}

	if err := store.SaveItems(items); err != nil {
		t.Fatalf("save items: %v", err)
	}

	results, err := store.Query("u1", "shanghai", 10, 0)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	fallback, err := store.Query("u1", "no-match", 10, 0)
	if err != nil {
		t.Fatalf("fallback query: %v", err)
	}
	if len(fallback) == 0 {
		t.Fatalf("expected fallback results")
	}
	if fallback[0].Content != "user lives in shanghai" {
		t.Fatalf("unexpected fallback result: %s", fallback[0].Content)
	}

	if err := store.Purge(time.Now(), 1); err != nil {
		t.Fatalf("purge: %v", err)
	}

	var count int
	row := store.db.QueryRow("SELECT COUNT(*) FROM memories")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("scan count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 memory after purge, got %d", count)
	}
}
