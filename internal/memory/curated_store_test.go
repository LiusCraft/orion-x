package memory

import (
	"strings"
	"testing"
)

func TestCuratedStoreSnapshot(t *testing.T) {
	s := &CuratedStore{
		deviceID:        "test-device",
		memoryEntries:   []string{"fact one", "fact two"},
		userEntries:     []string{"prefers concise replies"},
		memoryCharLimit: 2200,
		userCharLimit:   1375,
	}
	s.buildSnapshot()
	memBlock := s.FormatForSystemPrompt("memory")
	if !strings.Contains(memBlock, "fact one") {
		t.Fatal("snapshot should contain fact one")
	}
	if !strings.Contains(memBlock, "2200") {
		t.Fatal("snapshot should show char usage")
	}
}

func TestCuratedStoreAdd(t *testing.T) {
	s := &CuratedStore{
		deviceID:        "test-device",
		memoryEntries:   []string{},
		userEntries:     []string{},
		memoryCharLimit: 100,
		userCharLimit:   100,
	}
	result := s.Add("memory", "hello world")
	if !result.Success {
		t.Fatal("add should succeed")
	}
	if len(s.memoryEntries) != 1 {
		t.Fatal("should have 1 entry")
	}
	long := strings.Repeat("x", 200)
	result = s.Add("memory", long)
	if result.Success {
		t.Fatal("add should fail when over limit")
	}
	if result.CurrentEntries == nil {
		t.Fatal("overflow response should include current entries")
	}
}

func TestCuratedStoreReplaceRemove(t *testing.T) {
	s := &CuratedStore{
		deviceID:        "test-device",
		memoryEntries:   []string{"original fact", "another fact"},
		userEntries:     []string{},
		memoryCharLimit: 2200,
		userCharLimit:   1375,
	}
	result := s.Replace("memory", "original", "updated fact")
	if !result.Success {
		t.Fatalf("replace failed: %s", result.Error)
	}
	if s.memoryEntries[0] != "updated fact" {
		t.Fatal("entry should be updated")
	}
	result = s.Remove("memory", "another")
	if !result.Success {
		t.Fatalf("remove failed: %s", result.Error)
	}
	if len(s.memoryEntries) != 1 {
		t.Fatal("should have 1 entry after remove")
	}
}
