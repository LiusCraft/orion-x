package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/logging"
)

const (
	entryDelimiter     = "\n§\n"
	DefaultMemoryLimit = 2200
	DefaultUserLimit   = 1375
)

type ToolResult struct {
	Success        bool     `json:"success"`
	Done           bool     `json:"done,omitempty"`
	Error          string   `json:"error,omitempty"`
	CurrentEntries []string `json:"current_entries,omitempty"`
	Usage          string   `json:"usage,omitempty"`
	Target         string   `json:"target,omitempty"`
	EntryCount     int      `json:"entry_count,omitempty"`
}

type managerMemoryEntry struct {
	Target  string `json:"target"`
	Content string `json:"content"`
}

type memoryGetResponse struct {
	Entries struct {
		Memory []struct {
			Content   string `json:"content"`
			CreatedAt string `json:"created_at"`
		} `json:"memory"`
		User []struct {
			Content   string `json:"content"`
			CreatedAt string `json:"created_at"`
		} `json:"user"`
	} `json:"entries"`
	Usage struct {
		Memory struct{ Used, Limit int } `json:"memory"`
		User   struct{ Used, Limit int } `json:"user"`
	} `json:"usage"`
}

// CuratedStore holds per-device curated memory. Each connection gets one.
type CuratedStore struct {
	deviceID   string
	managerURL string
	localPath  string // local JSON file path (only set when managerURL is empty)
	httpClient *http.Client

	mu             sync.RWMutex
	wg             sync.WaitGroup
	memoryEntries  []string
	userEntries    []string
	memorySnapshot string // frozen at Load(), never changes
	userSnapshot   string

	memoryCharLimit int
	userCharLimit   int
}

func NewCuratedStore(managerURL, deviceID string, memoryLimit, userLimit int) *CuratedStore {
	if memoryLimit <= 0 {
		memoryLimit = DefaultMemoryLimit
	}
	if userLimit <= 0 {
		userLimit = DefaultUserLimit
	}
	localPath := ""
	if strings.TrimRight(managerURL, "/") == "" {
		localPath = "data/memory_local.json"
	}
	return &CuratedStore{
		deviceID:        deviceID,
		managerURL:      strings.TrimRight(managerURL, "/"),
		localPath:       localPath,
		httpClient:      &http.Client{},
		memoryCharLimit: memoryLimit,
		userCharLimit:   userLimit,
	}
}

// Load fetches memory from Manager and builds frozen snapshot.
// When managerURL is empty (local mode), reads from local JSON file.
func (c *CuratedStore) Load(ctx context.Context) error {
	if c.managerURL == "" || c.deviceID == "" {
		if c.localPath != "" {
			if data, err := os.ReadFile(c.localPath); err == nil {
				var local struct {
					Memory []string `json:"memory"`
					User   []string `json:"user"`
				}
				if err := json.Unmarshal(data, &local); err == nil {
					c.mu.Lock()
					c.memoryEntries = local.Memory
					c.userEntries = local.User
					c.buildSnapshot()
					c.mu.Unlock()
					return nil
				}
			}
		}
		c.buildSnapshot()
		return nil
	}
	url := fmt.Sprintf("%s/internal/devices/%s/memory", c.managerURL, c.deviceID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("load memory: status %d", resp.StatusCode)
	}

	var data memoryGetResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.memoryEntries = make([]string, len(data.Entries.Memory))
	for i, e := range data.Entries.Memory {
		c.memoryEntries[i] = strings.TrimSpace(e.Content)
	}
	c.userEntries = make([]string, len(data.Entries.User))
	for i, e := range data.Entries.User {
		c.userEntries[i] = strings.TrimSpace(e.Content)
	}

	c.buildSnapshot()
	return nil
}

// buildSnapshot generates frozen system-prompt blocks. Must be called under lock.
func (c *CuratedStore) buildSnapshot() {
	c.memorySnapshot = c.renderBlock("memory", c.memoryEntries, c.memoryCharLimit)
	c.userSnapshot = c.renderBlock("user", c.userEntries, c.userCharLimit)
}

// renderBlock creates a formatted block like Hermes' frozen snapshot.
func (c *CuratedStore) renderBlock(target string, entries []string, limit int) string {
	if len(entries) == 0 {
		return ""
	}

	content := strings.Join(entries, entryDelimiter)
	used := len([]rune(content))
	pct := 0
	if limit > 0 {
		pct = used * 100 / limit
		if pct > 100 {
			pct = 100
		}
	}

	var header string
	if target == "user" {
		header = fmt.Sprintf("═══════════════════ 用户画像 [%d%% — %d/%d] ═══════════════════", pct, used, limit)
	} else {
		header = fmt.Sprintf("═══════════════════ 记忆(环境/项目笔记) [%d%% — %d/%d] ═══════════════════", pct, used, limit)
	}

	return header + "\n" + content
}

// FormatForSystemPrompt returns the frozen snapshot block. Thread-safe.
func (c *CuratedStore) FormatForSystemPrompt(target string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if target == "user" {
		return c.userSnapshot
	}
	return c.memorySnapshot
}

// --- Tool operations (add/replace/remove) ---

func (c *CuratedStore) Add(target, content string) *ToolResult {
	content = strings.TrimSpace(content)
	if content == "" {
		return &ToolResult{Success: false, Error: "内容不能为空"}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entries, limit := c.entriesFor(target)
	if contains(entries, content) {
		return &ToolResult{Success: true, Done: true, EntryCount: len(entries),
			Target: target, Usage: c.usageStr(target, entries, limit),
			Error: "条目已存在（不重复添加）"}
	}

	newEntries := append(entries, content)
	newTotal := charCount(newEntries)
	if newTotal > limit {
		current := charCount(entries)
		return &ToolResult{
			Success:        false,
			Error:          fmt.Sprintf("已达到 %d/%d 字符。添加这条(%d字符)会超限。请在同一轮中使用 replace 合并相关条目，或 remove 删除过期条目，然后重试。", current, limit, len([]rune(content))),
			CurrentEntries: entries,
			Usage:          fmt.Sprintf("%d/%d", current, limit),
		}
	}

	c.setEntries(target, newEntries)
	c.goSync() // async flush to manager

	return &ToolResult{Success: true, Done: true, Target: target,
		EntryCount: len(newEntries), Usage: c.usageStr(target, newEntries, limit),
		Error: "写入完成。不要重复此操作。"}
}

func (c *CuratedStore) Replace(target, oldText, newContent string) *ToolResult {
	oldText = strings.TrimSpace(oldText)
	newContent = strings.TrimSpace(newContent)
	if oldText == "" {
		return &ToolResult{Success: false, Error: "old_text 不能为空"}
	}
	if newContent == "" {
		return &ToolResult{Success: false, Error: "new_content 不能为空。要删除请使用 remove"}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entries, limit := c.entriesFor(target)
	matches := findMatches(entries, oldText)
	if len(matches) == 0 {
		return &ToolResult{Success: false, Error: fmt.Sprintf("未找到匹配 '%s' 的条目", oldText),
			CurrentEntries: entries}
	}
	if len(matches) > 1 {
		unique := uniqueStrings(matches)
		if len(unique) > 1 {
			return &ToolResult{Success: false, Error: fmt.Sprintf("'%s' 匹配了多条不同的条目，请使用更精确的匹配文本", oldText),
				CurrentEntries: entries}
		}
	}

	idx := indexOf(entries, matches[0])
	testEntries := copySlice(entries)
	testEntries[idx] = newContent
	if charCount(testEntries) > limit {
		return &ToolResult{Success: false, Error: "替换后超出字符限额，请缩短内容或先删除其他条目",
			CurrentEntries: entries}
	}

	entries[idx] = newContent
	c.setEntries(target, entries)
	c.goSync()

	return &ToolResult{Success: true, Done: true, Target: target,
		EntryCount: len(entries), Usage: c.usageStr(target, entries, limit),
		Error: "条目已替换。不要重复此操作。"}
}

func (c *CuratedStore) Remove(target, oldText string) *ToolResult {
	oldText = strings.TrimSpace(oldText)
	if oldText == "" {
		return &ToolResult{Success: false, Error: "old_text 不能为空"}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entries, limit := c.entriesFor(target)
	matches := findMatches(entries, oldText)
	if len(matches) == 0 {
		return &ToolResult{Success: false, Error: fmt.Sprintf("未找到匹配 '%s' 的条目", oldText),
			CurrentEntries: entries}
	}
	if len(matches) > 1 {
		unique := uniqueStrings(matches)
		if len(unique) > 1 {
			return &ToolResult{Success: false, Error: fmt.Sprintf("'%s' 匹配了多条不同的条目，请使用更精确的匹配文本", oldText),
				CurrentEntries: entries}
		}
	}

	idx := indexOf(entries, matches[0])
	entries = append(entries[:idx], entries[idx+1:]...)
	c.setEntries(target, entries)
	c.goSync()

	return &ToolResult{Success: true, Done: true, Target: target,
		EntryCount: len(entries), Usage: c.usageStr(target, entries, limit),
		Error: "条目已删除。不要重复此操作。"}
}

// --- Internal helpers ---

func (c *CuratedStore) entriesFor(target string) ([]string, int) {
	if target == "user" {
		return c.userEntries, c.userCharLimit
	}
	return c.memoryEntries, c.memoryCharLimit
}

func (c *CuratedStore) setEntries(target string, entries []string) {
	if target == "user" {
		c.userEntries = entries
	} else {
		c.memoryEntries = entries
	}
}

func (c *CuratedStore) usageStr(target string, entries []string, limit int) string {
	return fmt.Sprintf("%d/%d", charCount(entries), limit)
}

// goSync async-flushes to manager or persists locally.
func (c *CuratedStore) goSync() {
	if c.managerURL == "" && c.localPath != "" {
		c.localPersist()
		return
	}
	if c.managerURL == "" {
		return
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		c.mu.RLock()
		memEntries := copySlice(c.memoryEntries)
		userEntries := copySlice(c.userEntries)
		c.mu.RUnlock()

		payload := struct {
			Entries []managerMemoryEntry `json:"entries"`
		}{}
		for _, e := range memEntries {
			payload.Entries = append(payload.Entries, managerMemoryEntry{Target: "memory", Content: e})
		}
		for _, e := range userEntries {
			payload.Entries = append(payload.Entries, managerMemoryEntry{Target: "user", Content: e})
		}

		body, err := json.Marshal(payload)
		if err != nil {
			logging.Errorf("CuratedStore[%s]: goSync marshal payload: %v", c.deviceID, err)
			return
		}
		url := fmt.Sprintf("%s/internal/devices/%s/memory", c.managerURL, c.deviceID)
		req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(body))
		if err != nil {
			logging.Errorf("CuratedStore[%s]: goSync create request: %v", c.deviceID, err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			logging.Errorf("CuratedStore[%s]: goSync HTTP PUT: %v", c.deviceID, err)
			return
		}
		_ = resp.Body.Close()
	}()
}

// localPersist writes entries to a local JSON file.
func (c *CuratedStore) localPersist() {
	c.mu.RLock()
	data, err := json.Marshal(map[string][]string{
		"memory": c.memoryEntries,
		"user":   c.userEntries,
	})
	c.mu.RUnlock()
	if err != nil {
		logging.Errorf("CuratedStore[%s]: localPersist marshal: %v", c.deviceID, err)
		return
	}
	if err := os.MkdirAll("data", 0755); err != nil {
		logging.Errorf("CuratedStore[%s]: localPersist mkdir: %v", c.deviceID, err)
		return
	}
	if err := os.WriteFile(c.localPath, data, 0644); err != nil {
		logging.Errorf("CuratedStore[%s]: localPersist write: %v", c.deviceID, err)
	}
}

// WaitForSync blocks until all pending goSync goroutines complete.
func (c *CuratedStore) WaitForSync() {
	c.wg.Wait()
}

// --- Utility funcs ---

func charCount(entries []string) int {
	if len(entries) == 0 {
		return 0
	}
	return len([]rune(strings.Join(entries, entryDelimiter)))
}

func contains(entries []string, s string) bool {
	for _, e := range entries {
		if e == s {
			return true
		}
	}
	return false
}

func findMatches(entries []string, sub string) []string {
	var matches []string
	for _, e := range entries {
		if strings.Contains(e, sub) {
			matches = append(matches, e)
		}
	}
	return matches
}

func uniqueStrings(s []string) []string {
	seen := make(map[string]bool)
	var r []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			r = append(r, v)
		}
	}
	return r
}

func indexOf(entries []string, s string) int {
	for i, e := range entries {
		if e == s {
			return i
		}
	}
	return -1
}

func copySlice(s []string) []string {
	r := make([]string, len(s))
	copy(r, s)
	return r
}
