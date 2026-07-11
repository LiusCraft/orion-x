# 记忆系统 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Hermes-aligned memory system in Orion-X with per-device curated memory, Agent-managed memory tool, background review, structured context compression, and session search.

**Architecture:** Manager (PostgreSQL) stores memory entries + conversation history. wsserver connects via RESTful HTTP API. Each connection gets an in-memory CuratedStore with frozen snapshot for system prompt injection. Post-turn BackgroundReview goroutine extracts facts. ContextCompressor triggers when context window exceeds threshold.

**Tech Stack:** Go, Gin/GORM, PostgreSQL (tsvector FTS), HTTP client

## Global Constraints

- All Manager HTTP APIs use RESTful style under `/internal/devices/{device_id}/...`
- Curated memory char limits: memory = 2200, user = 1375 (Hermes 1:1)
- Memory tool responses: success with `done:true`, overflow with `current_entries`
- Memory snapshot is frozen at session start, never mutated mid-session
- Background review runs async goroutine, non-blocking
- No new external dependencies beyond existing (gorm, gin, gin-swagger etc.)
- Every new Go file must pass `golangci-lint run ./...`

---

## File Structure

```
internal/store/
    memory.go                  ← MemoryEntry model + MemoryEntryStore
    turn.go                    ← SessionTurn model + TurnStore + FTS
    db.go                      ← AutoMigrate: add memory_entries + session_turns

internal/memory/
    curated_store.go           ← CuratedStore: cache, snapshot, HTTP sync
    compressor.go              ← ContextCompressor: structured summary
    background_review.go       ← BackgroundReview: post-turn review
    service.go                 ← Rewrite: CuratedStore + Review + Compressor facade
    types.go                   ← Keep Turn, MemoryItem, drop Store interface

internal/tools/
    memory_tool.go             ← MemoryTool: schema + handler
    session_search_tool.go     ← SessionSearchTool: schema + handler

internal/config/
    config.go                  ← Add ReviewConfig, CompressionConfig, MemoryCharLimit

internal/agent/
    agent.go                   ← Add RegisterBuiltinTool method

cmd/manager/handler/
    memory.go                  ← Memory API handler
    turn.go                    ← Turn API handler
    internal.go                ← No change (but new file added)

cmd/manager/
    server.go                  ← Register /internal/devices/:id/memory etc.

cmd/wsserver/
    main.go                    ← Init HTTP client, pass to connection
    connection.go              ← Per-connection CuratedStore, tool registration
```

---

### Task 1: Manager Data Model — MemoryEntry + SessionTurn

**Files:**

- Create: `internal/store/memory.go`
- Create: `internal/store/turn.go`
- Modify: `internal/store/db.go` (add AutoMigrate models)

**Interfaces:**

- Consumes: `gorm.DB` from existing Open func
- Produces: `store.MemoryEntry{,Store}`, `store.SessionTurn{,Store}` with FTS index

- [ ] **Step 1: Create `internal/store/memory.go`**

```go
package store

import (
    "time"
    "github.com/google/uuid"
    "gorm.io/gorm"
)

type MemoryEntry struct {
    ID        string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
    DeviceID  string    `gorm:"not null;index:idx_mem_device_target;type:varchar(128)" json:"device_id"`
    Target    string    `gorm:"not null;type:varchar(16)" json:"target"` // "memory" | "user"
    Content   string    `gorm:"not null;type:text" json:"content"`
    CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
    UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (MemoryEntry) TableName() string { return "memory_entries" }

type MemoryEntryStore struct{ db *gorm.DB }

func NewMemoryEntryStore(db *gorm.DB) *MemoryEntryStore {
    return &MemoryEntryStore{db: db}
}

func (s *MemoryEntryStore) ListByDevice(deviceID string) ([]MemoryEntry, error) {
    var entries []MemoryEntry
    if err := s.db.Where("device_id = ?", deviceID).
        Order("target, created_at").Find(&entries).Error; err != nil {
        return nil, err
    }
    return entries, nil
}

func (s *MemoryEntryStore) ReplaceAll(deviceID string, entries []MemoryEntry) error {
    return s.db.Transaction(func(tx *gorm.DB) error {
        if err := tx.Where("device_id = ?", deviceID).Delete(&MemoryEntry{}).Error; err != nil {
            return err
        }
        if len(entries) == 0 {
            return nil
        }
        for i := range entries {
            entries[i].ID = uuid.New().String()
            entries[i].DeviceID = deviceID
        }
        return tx.Create(entries).Error
    })
}
```

- [ ] **Step 2: Create `internal/store/turn.go`**

```go
package store

import (
    "time"
    "gorm.io/gorm"
)

type SessionTurn struct {
    ID            int64     `gorm:"primaryKey;autoIncrement" json:"id"`
    DeviceID      string    `gorm:"not null;index:idx_turns_device;type:varchar(128)" json:"device_id"`
    SessionID     string    `gorm:"not null;type:varchar(64)" json:"session_id"`
    TurnID        int64     `gorm:"not null" json:"turn_id"`
    UserText      string    `gorm:"type:text" json:"user_text"`
    AssistantText string    `gorm:"type:text" json:"assistant_text"`
    StartedAt     time.Time `json:"started_at"`
    EndedAt       time.Time `json:"ended_at"`
    Aborted       bool      `gorm:"not null;default:false" json:"aborted"`
    CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (SessionTurn) TableName() string { return "session_turns" }

type TurnStore struct{ db *gorm.DB }

func NewTurnStore(db *gorm.DB) *TurnStore { return &TurnStore{db: db} }

func (s *TurnStore) Create(turn *SessionTurn) error {
    return s.db.Create(turn).Error
}

// Search runs FTS via tsvector. Returns matching turns grouped by session.
func (s *TurnStore) Search(deviceID, query string, limit int) ([]SessionTurn, error) {
    var turns []SessionTurn
    // Use websearch_to_tsquery for flexible query syntax (supports "quoted phrases")
    // Fallback to plainto_tsquery when websearch unavailable
    err := s.db.Raw(`
        SELECT st.* FROM session_turns st
        WHERE st.device_id = ?
          AND to_tsvector('simple', coalesce(st.user_text,'') || ' ' || coalesce(st.assistant_text,'')) @@
              websearch_to_tsquery('simple', ?)
        ORDER BY st.created_at DESC
        LIMIT ?`, deviceID, query, limit).Scan(&turns).Error
    return turns, err
}

type SessionSummary struct {
    SessionID    string    `json:"session_id"`
    StartedAt    time.Time `json:"started_at"`
    EndedAt      time.Time `json:"ended_at"`
    MessageCount int64     `json:"message_count"`
    Preview      string    `json:"preview"`
}

func (s *TurnStore) ListSessions(deviceID string, limit int) ([]SessionSummary, error) {
    var sessions []SessionSummary
    err := s.db.Raw(`
        SELECT session_id,
               MIN(started_at) AS started_at,
               MAX(ended_at) AS ended_at,
               COUNT(*) AS message_count,
               MAX(user_text) AS preview
        FROM session_turns
        WHERE device_id = ?
        GROUP BY session_id
        ORDER BY MAX(created_at) DESC
        LIMIT ?`, deviceID, limit).Scan(&sessions).Error
    return sessions, err
}

func (s *TurnStore) ListBySession(deviceID, sessionID string, limit, offset int) ([]SessionTurn, error) {
    var turns []SessionTurn
    err := s.db.Where("device_id = ? AND session_id = ?", deviceID, sessionID).
        Order("turn_id ASC").Limit(limit).Offset(offset).Find(&turns).Error
    return turns, err
}
```

- [ ] **Step 3: Update `internal/store/db.go` — AutoMigrate new models**

Find `db.AutoMigrate(` line and edit to include `&MemoryEntry{}, &SessionTurn{},`.

```go
// Before:
if err := db.AutoMigrate(&User{}, &Voicebot{}, &Device{}, &Provider{}, &AIModel{}, &ModelVoice{}, &Language{}, &MCPMarketEntry{}, &MCPServer{}, &VoicebotMCPBinding{}); err != nil {

// After:
if err := db.AutoMigrate(&User{}, &Voicebot{}, &Device{}, &Provider{}, &AIModel{}, &ModelVoice{}, &Language{}, &MCPMarketEntry{}, &MCPServer{}, &VoicebotMCPBinding{}, &MemoryEntry{}, &SessionTurn{}); err != nil {
```

Add FTS index creation after migrations:

```go
// After the seedLanguages block, add:
if err := ensureTurnFTSIndex(db); err != nil {
    return fmt.Errorf("store: fts index: %w", err)
}
```

```go
func ensureTurnFTSIndex(db *gorm.DB) error {
    return db.Exec(`CREATE INDEX IF NOT EXISTS idx_turns_fts ON session_turns
        USING gin(to_tsvector('simple', coalesce(user_text,'') || ' ' || coalesce(assistant_text,'')))`).Error
}
```

- [ ] **Step 4: Run tests**

```bash
cd /path/to/orion-x && go build ./internal/store/...
```

Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
git add internal/store/memory.go internal/store/turn.go internal/store/db.go
git commit -m "feat(store): add MemoryEntry and SessionTurn models with FTS"
```

---

### Task 2: Manager HTTP Handlers — Memory + Turn APIs

**Files:**

- Create: `cmd/manager/handler/memory.go`
- Create: `cmd/manager/handler/turn.go`
- Modify: `cmd/manager/server.go`

**Interfaces:**

- Consumes: `store.MemoryEntryStore`, `store.TurnStore`
- Produces: REST endpoints under `/internal/devices/{device_id}/...`

- [ ] **Step 1: Create `cmd/manager/handler/memory.go`**

```go
package handler

import (
    "net/http"
    "github.com/gin-gonic/gin"
    "github.com/liuscraft/orion-x/internal/store"
)

const (
    defaultMemoryCharLimit = 2200
    defaultUserCharLimit   = 1375
)

type MemoryHandler struct {
    store *store.MemoryEntryStore
}

func NewMemoryHandler(s *store.MemoryEntryStore) *MemoryHandler {
    return &MemoryHandler{store: s}
}

// GetMemory GET /internal/devices/:device_id/memory
func (h *MemoryHandler) GetMemory(c *gin.Context) {
    deviceID := c.Param("device_id")
    entries, err := h.store.ListByDevice(deviceID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    var memEntries, userEntries []gin.H
    var memUsed, userUsed int
    for _, e := range entries {
        if e.Target == "user" {
            userUsed += len(e.Content)
            userEntries = append(userEntries, gin.H{"content": e.Content, "created_at": e.CreatedAt})
        } else {
            memUsed += len(e.Content)
            memEntries = append(memEntries, gin.H{"content": e.Content, "created_at": e.CreatedAt})
        }
    }

    c.JSON(http.StatusOK, gin.H{
        "entries": gin.H{
            "memory": memEntries,
            "user":   userEntries,
        },
        "usage": gin.H{
            "memory": gin.H{"used": memUsed, "limit": defaultMemoryCharLimit},
            "user":   gin.H{"used": userUsed, "limit": defaultUserCharLimit},
        },
    })
}

type memoryPutBody struct {
    Entries []struct {
        Target  string `json:"target" binding:"required"`
        Content string `json:"content" binding:"required"`
    } `json:"entries" binding:"required"`
}

// PutMemory PUT /internal/devices/:device_id/memory
func (h *MemoryHandler) PutMemory(c *gin.Context) {
    deviceID := c.Param("device_id")
    var body memoryPutBody
    if err := c.ShouldBindJSON(&body); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    entries := make([]store.MemoryEntry, 0, len(body.Entries))
    for _, e := range body.Entries {
        entries = append(entries, store.MemoryEntry{
            Target:  e.Target,
            Content: e.Content,
        })
    }

    if err := h.store.ReplaceAll(deviceID, entries); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.Status(http.StatusNoContent)
}
```

- [ ] **Step 2: Create `cmd/manager/handler/turn.go`**

```go
package handler

import (
    "net/http"
    "strconv"
    "github.com/gin-gonic/gin"
    "github.com/liuscraft/orion-x/internal/store"
)

type TurnHandler struct {
    store *store.TurnStore
}

func NewTurnHandler(s *store.TurnStore) *TurnHandler {
    return &TurnHandler{store: s}
}

// CreateTurn POST /internal/devices/:device_id/turns
func (h *TurnHandler) CreateTurn(c *gin.Context) {
    deviceID := c.Param("device_id")
    var turn store.SessionTurn
    if err := c.ShouldBindJSON(&turn); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    turn.DeviceID = deviceID
    if err := h.store.Create(&turn); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusCreated, gin.H{"id": turn.ID})
}

// SearchTurns GET /internal/devices/:device_id/turns?q=xxx&limit=3
func (h *TurnHandler) SearchTurns(c *gin.Context) {
    deviceID := c.Param("device_id")
    query := c.Query("q")
    if query == "" {
        // browse mode: return sessions summary
        h.listSessions(c, deviceID)
        return
    }
    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "3"))
    if limit <= 0 || limit > 10 {
        limit = 3
    }
    turns, err := h.store.Search(deviceID, query, limit)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    // Group results by session_id, return first match per session
    seen := make(map[string]bool)
    results := make([]gin.H, 0, limit)
    for _, t := range turns {
        if seen[t.SessionID] {
            continue
        }
        seen[t.SessionID] = true
        results = append(results, gin.H{
            "session_id": t.SessionID,
            "snippet":    truncate(t.UserText + " " + t.AssistantText, 200),
            "matched_role": "user",
            "messages": []gin.H{
                {"id": t.ID, "role": "user", "content": t.UserText, "timestamp": t.CreatedAt},
                {"id": t.ID + 1, "role": "assistant", "content": t.AssistantText, "timestamp": t.CreatedAt},
            },
        })
        if len(results) >= limit {
            break
        }
    }
    c.JSON(http.StatusOK, gin.H{"results": results})
}

// GetSessionMessages GET /internal/devices/:device_id/sessions/:session_id
func (h *TurnHandler) GetSessionMessages(c *gin.Context) {
    deviceID := c.Param("device_id")
    sessionID := c.Param("session_id")
    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
    offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
    turns, err := h.store.ListBySession(deviceID, sessionID, limit, offset)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    messages := make([]gin.H, 0, len(turns))
    for _, t := range turns {
        messages = append(messages, gin.H{
            "id": t.ID, "role": "user", "content": t.UserText, "timestamp": t.CreatedAt,
        })
        // Only show non-empty assistant text as a separate message
        if t.AssistantText != "" {
            messages = append(messages, gin.H{
                "id": t.ID + 1, "role": "assistant", "content": t.AssistantText, "timestamp": t.EndedAt,
            })
        }
    }
    c.JSON(http.StatusOK, gin.H{"messages": messages, "count": len(messages)})
}

func (h *TurnHandler) listSessions(c *gin.Context, deviceID string) {
    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
    if limit <= 0 || limit > 50 {
        limit = 10
    }
    sessions, err := h.store.ListSessions(deviceID, limit)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

func truncate(s string, max int) string {
    runes := []rune(s)
    if len(runes) <= max {
        return s
    }
    return string(runes[:max]) + "..."
}
```

- [ ] **Step 3: Register routes in `cmd/manager/server.go`**

In the `newRouter` function, add MemoryHandler and TurnHandler creation, plus internal routes:

```go
// After existing handler creation:
memH := NewMemoryHandler(store.NewMemoryEntryStore(db))
turnH := NewTurnHandler(store.NewTurnStore(db))

// After existing internal routes:
internal := r.Group("/internal")
{
    internal.GET("/device-config", internalH.DeviceConfig)
    // Add new memory/turn routes:
    internal.GET("/devices/:device_id/memory", memH.GetMemory)
    internal.PUT("/devices/:device_id/memory", memH.PutMemory)
    internal.POST("/devices/:device_id/turns", turnH.CreateTurn)
    internal.GET("/devices/:device_id/turns", turnH.SearchTurns)
    internal.GET("/devices/:device_id/sessions/:session_id", turnH.GetSessionMessages)
}
```

Note: `:device_id` in Gin captures the path segment as `c.Param("device_id")`.

- [ ] **Step 4: Build**

```bash
cd /path/to/orion-x && go build ./cmd/manager/
```

Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
git add cmd/manager/handler/memory.go cmd/manager/handler/turn.go cmd/manager/server.go
git commit -m "feat(manager): add memory and turn REST API handlers"
```

---

### Task 3: CuratedStore — In-Memory Cache + Frozen Snapshot + HTTP Sync

**Files:**

- Create: `internal/memory/curated_store.go`

**Interfaces:**

- Consumes: Manager HTTP API at `managerURL`
- Produces: `*CuratedStore` with `Load()`, `FormatForSystemPrompt()`, `Add/Replace/Remove/Batch()`

- [ ] **Step 1: Write the test**

```go
// internal/memory/curated_store_test.go (inline)
func TestCuratedStoreSnapshot(t *testing.T) {
    // Create store without HTTP (inject entries directly)
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
    if !strings.Contains(memBlock, "2/2,200") {
        t.Fatal("snapshot should show char usage")
    }
}

func TestCuratedStoreAdd(t *testing.T) {
    s := &CuratedStore{
        deviceID:        "test-device",
        memoryEntries:   []string{},
        userEntries:     []string{},
        memoryCharLimit: 100, // small for test
        userCharLimit:   100,
    }
    // Add within limit
    result := s.Add("memory", "hello world")
    if !result.Success {
        t.Fatal("add should succeed")
    }
    if len(s.memoryEntries) != 1 {
        t.Fatal("should have 1 entry")
    }
    // Add overflow
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
    // Replace by substring
    result := s.Replace("memory", "original", "updated fact")
    if !result.Success {
        t.Fatalf("replace failed: %s", result.Error)
    }
    if s.memoryEntries[0] != "updated fact" {
        t.Fatal("entry should be updated")
    }
    // Remove by substring
    result = s.Remove("memory", "another")
    if !result.Success {
        t.Fatalf("remove failed: %s", result.Error)
    }
    if len(s.memoryEntries) != 1 {
        t.Fatal("should have 1 entry after remove")
    }
}
```

- [ ] **Step 2: Create `internal/memory/curated_store.go`**

```go
package memory

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "sync"
)

const (
    entryDelimiter     = "\n§\n"
    DefaultMemoryLimit = 2200
    DefaultUserLimit   = 1375
)

type ToolResult struct {
    Success       bool     `json:"success"`
    Done          bool     `json:"done,omitempty"`
    Error         string   `json:"error,omitempty"`
    CurrentEntries []string `json:"current_entries,omitempty"`
    Usage         string   `json:"usage,omitempty"`
    Target        string   `json:"target,omitempty"`
    EntryCount    int      `json:"entry_count,omitempty"`
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
    deviceID  string
    managerURL string
    httpClient *http.Client

    mu             sync.RWMutex
    memoryEntries  []string
    userEntries    []string
    memorySnapshot string // frozen at Load(), never changes
    userSnapshot   string

    memoryCharLimit int
    userCharLimit   int
}

func NewCuratedStore(managerURL, deviceID string, memoryLimit, userLimit int) (*CuratedStore) {
    if memoryLimit <= 0 { memoryLimit = DefaultMemoryLimit }
    if userLimit <= 0 { userLimit = DefaultUserLimit }
    return &CuratedStore{
        deviceID:     deviceID,
        managerURL:   strings.TrimRight(managerURL, "/"),
        httpClient:   &http.Client{},
        memoryCharLimit: memoryLimit,
        userCharLimit:   userLimit,
    }
}

// Load fetches memory from Manager and builds frozen snapshot.
func (c *CuratedStore) Load(ctx context.Context) error {
    url := fmt.Sprintf("%s/internal/devices/%s/memory", c.managerURL, c.deviceID)
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil { return err }
    resp, err := c.httpClient.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()

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
        if pct > 100 { pct = 100 }
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

// --- Tool operations (add/replace/remove/batch) ---

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
            Success: false,
            Error:   fmt.Sprintf("已达到 %d/%d 字符。添加这条(%d字符)会超限。请在同一轮中使用 replace 合并相关条目，或 remove 删除过期条目，然后重试。", current, limit, len([]rune(content))),
            CurrentEntries: entries,
            Usage:  fmt.Sprintf("%d/%d", current, limit),
        }
    }

    c.setEntries(target, newEntries)
    c.goSync() // async flush to manager

    c.consolidationFailures = 0 // reset on success
    return &ToolResult{Success: true, Done: true, Target: target,
        EntryCount: len(newEntries), Usage: c.usageStr(target, newEntries, limit),
        Error: "写入完成。不要重复此操作。"}
}

func (c *CuratedStore) Replace(target, oldText, newContent string) *ToolResult {
    oldText = strings.TrimSpace(oldText)
    newContent = strings.TrimSpace(newContent)
    if oldText == "" { return &ToolResult{Success: false, Error: "old_text 不能为空"} }
    if newContent == "" { return &ToolResult{Success: false, Error: "new_content 不能为空。要删除请使用 remove"} }

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
    if oldText == "" { return &ToolResult{Success: false, Error: "old_text 不能为空"} }

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

// goSync async-flushes to manager. Best-effort, logs errors.
func (c *CuratedStore) goSync() {
    go func() {
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

        body, _ := json.Marshal(payload)
        url := fmt.Sprintf("%s/internal/devices/%s/memory", c.managerURL, c.deviceID)
        req, _ := http.NewRequest("PUT", url, bytes.NewReader(body))
        req.Header.Set("Content-Type", "application/json")
        resp, err := c.httpClient.Do(req)
        if err != nil {
            // TODO: log error
            return
        }
        resp.Body.Close()
    }()
}

// --- Utility funcs ---

func charCount(entries []string) int {
    if len(entries) == 0 { return 0 }
    return len([]rune(strings.Join(entries, entryDelimiter)))
}

func contains(entries []string, s string) bool {
    for _, e := range entries { if e == s { return true } }
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
        if !seen[v] { seen[v] = true; r = append(r, v) }
    }
    return r
}

func indexOf(entries []string, s string) int {
    for i, e := range entries { if e == s { return i } }
    return -1
}

func copySlice(s []string) []string {
    r := make([]string, len(s))
    copy(r, s)
    return r
}
```

- [ ] **Step 3: Run unit tests**

```bash
go test ./internal/memory/ -run TestCuratedStore -v
```

Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add internal/memory/curated_store.go
git commit -m "feat(memory): add CuratedStore with frozen snapshot and HTTP sync"
```

---

### Task 4: Memory Tool — Schema + Handler

**Files:**

- Create: `internal/tools/memory_tool.go`
- Modify: `internal/agent/agent.go` (add `RegisterBuiltinTool`)

**Interfaces:**

- Consumes: `*memory.CuratedStore`
- Produces: Tool schema registered in Agent

- [ ] **Step 1: Create `internal/tools/memory_tool.go`**

```go
package tools

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/liuscraft/orion-x/internal/memory"
)

const MemoryToolName = "memory"

var MemorySchema = Schema{
    Name: MemoryToolName,
    Description: "管理长期记忆。你有两个存储空间：\n" +
        "memory（你的笔记：环境事实、项目约定、工具技巧、学到的东西）\n" +
        "user（用户画像：偏好、风格、习惯、期待）\n\n" +
        "何时保存（主动保存，不需要用户要求）：\n" +
        "  用户偏好：「我更喜欢 TypeScript」→ 存到 user\n" +
        "  环境事实：「这台机器装的 Debian 12」→ 存到 memory\n" +
        "  纠正：「不要用 sudo」→ 存到 memory\n" +
        "  约定：「项目用 tab 缩进」→ 存到 memory\n" +
        "  完成的工作：「已将数据库从 MySQL 迁到 PostgreSQL」→ 存到 memory\n" +
        "  明确要求：「记一下我的 API key 每月轮换」→ 存到 memory\n\n" +
        "不要保存：\n" +
        "  显而易见的琐事、易重新发现的事实、原始数据转储、临时会话上下文\n\n" +
        "容量管理：memory 上限 2200 字符，user 上限 1375 字符。\n" +
        "  当使用率超过 80% 时，在追加新条目之前合并或删除。\n" +
        "  合并时用 replace 将相关条目合并为更短的版本。\n\n" +
        "子串匹配：replace/remove 的 old_text 只需要能唯一匹配一条条目的子串。",
    Parameters: json.RawMessage(`{
        "type": "object",
        "properties": {
            "action": {
                "type": "string",
                "enum": ["add", "replace", "remove"],
                "description": "操作类型"
            },
            "target": {
                "type": "string",
                "enum": ["memory", "user"],
                "description": "memory = 你的笔记（环境/项目/技巧）；user = 用户画像"
            },
            "content": {
                "type": "string",
                "description": "add 或 replace 时的新内容。简洁、信息密集的句子效果最好。"
            },
            "old_text": {
                "type": "string",
                "description": "replace 或 remove 时的子串匹配。只需要唯一的子串，不必是完整条目。"
            }
        },
        "required": ["action", "target"]
    }`),
}

type memoryArgs struct {
    Action  string `json:"action"`
    Target  string `json:"target"`
    Content string `json:"content,omitempty"`
    OldText string `json:"old_text,omitempty"`
}

type MemoryToolHandler struct {
    store *memory.CuratedStore
}

func NewMemoryToolHandler(store *memory.CuratedStore) *MemoryToolHandler {
    return &MemoryToolHandler{store: store}
}

func (h *MemoryToolHandler) Handle(ctx context.Context, args json.RawMessage) (string, error) {
    var a memoryArgs
    if err := json.Unmarshal(args, &a); err != nil {
        return "", fmt.Errorf("memory: parse args: %w", err)
    }

    if a.Target != "memory" && a.Target != "user" {
        return errorJSON("target 必须是 memory 或 user"), nil
    }

    var result *memory.ToolResult
    switch a.Action {
    case "add":
        result = h.store.Add(a.Target, a.Content)
    case "replace":
        result = h.store.Replace(a.Target, a.OldText, a.Content)
    case "remove":
        result = h.store.Remove(a.Target, a.OldText)
    default:
        return errorJSON("action 必须是 add/replace/remove"), nil
    }

    return toJSON(result), nil
}

func errorJSON(msg string) string {
    b, _ := json.Marshal(map[string]interface{}{
        "success": false, "error": msg,
    })
    return string(b)
}

func toJSON(v interface{}) string {
    b, _ := json.Marshal(v)
    return string(b)
}
```

- [ ] **Step 2: Add `RegisterBuiltinTool` to Agent**

In `internal/agent/agent.go`, add the method and call it:

```go
// Before: struct definition
type RegisteredTool struct {
    Schema  tools.Schema
    Handler func(ctx context.Context, args json.RawMessage) (string, error)
}

type Agent struct {
    // ...existing fields...
    builtinTools map[string]*RegisteredTool
}

// In New:
func New(ctx context.Context, cfg Config, mgr *tools.Manager, memorySvc memory.Service) (*Agent, error) {
    // ...existing code...
    return &Agent{
        client:       client,
        registry:     mgr.Registry(),
        model:        normalized.Model,
        memorySvc:    memorySvc,
        maxSteps:     10,
        systemPrompt: systemPrompt,
        builtinTools: make(map[string]*RegisteredTool),
    }, nil
}

// AddRegisterBuiltinTool method:
func (a *Agent) RegisterBuiltinTool(schema tools.Schema, handler func(ctx context.Context, args json.RawMessage) (string, error)) {
    if a.builtinTools == nil {
        a.builtinTools = make(map[string]*RegisteredTool)
    }
    a.builtinTools[schema.Name] = &RegisteredTool{Schema: schema, Handler: handler}
}
```

Add the import for `encoding/json`.

- [ ] **Step 3: Build**

```bash
go build ./internal/tools/ ./internal/agent/
```

Expected: builds clean.

- [ ] **Step 4: Commit**

```bash
git add internal/tools/memory_tool.go internal/agent/agent.go
git commit -m "feat(tools): add MemoryTool schema and Agent RegisterBuiltinTool"
```

---

### Task 5: BackgroundReview — Post-Turn Self-Improvement

**Files:**

- Create: `internal/memory/background_review.go`
- Modify: `internal/config/config.go` (add review config)

**Interfaces:**

- Consumes: `llm.Client` for review LLM, `*memory.CuratedStore` for writes
- Produces: async goroutine per turn

- [ ] **Step 1: Create `internal/memory/background_review.go`**

```go
package memory

import (
    "context"
    "encoding/json"
    "strings"

    "github.com/liuscraft/orion-x/internal/llm"
    "github.com/liuscraft/orion-x/internal/logging"
)

const reviewPrompt = `回顾上面的对话，考虑是否需要保存记忆。

重点关注：
1. 用户是否透露了关于他们自己的信息——他们的性格、愿望、偏好、或个人细节值得记住？
2. 用户是否表达了对你的行为方式、工作风格、交流方式的期望？

如果有什么值得记住的，使用 memory tool 保存。
如果没有值得保存的，就说"无需保存"。
不要重复用户已经明确要求删除或不要记住的内容。`

type ReviewConfig struct {
    Enabled bool   // 是否启用
    Model   string // 自省模型 ID（空=用主模型）
    APIKey  string
    BaseURL string
}

type BackgroundReview struct {
    curated *CuratedStore
    config  ReviewConfig
}

func NewBackgroundReview(curated *CuratedStore, config ReviewConfig) *BackgroundReview {
    return &BackgroundReview{curated: curated, config: config}
}

// Spawn fires a non-blocking review. snapshot contains recent turns + existing memory.
func (r *BackgroundReview) Spawn(ctx context.Context, recentTurns []Turn, existingSnapshot string) {
    if !r.config.Enabled {
        return
    }

    go func() {
        // Build review context from recent turns
        var b strings.Builder
        b.WriteString("当前记忆：\n")
        if existingSnapshot != "" {
            b.WriteString(existingSnapshot)
            b.WriteString("\n\n")
        }
        b.WriteString("最近对话：\n")
        for i, t := range recentTurns {
            if i > 10 {
                b.WriteString("...（更多历史已省略）\n")
                break
            }
            b.WriteString("用户: " + strings.TrimSpace(t.UserText) + "\n")
            b.WriteString("助手: " + strings.TrimSpace(t.AssistantText) + "\n")
        }

        reviewCtx := []llm.Message{
            {Role: "system", Content: reviewPrompt},
            {Role: "user", Content: b.String()},
        }

        // TODO: Call review LLM. For now, this is a placeholder that logs.
        // In a real implementation, this would call the LLM with the review model,
        // parse the response, and call r.curated.Add/Replace/Remove accordingly.
        logging.Infof("BackgroundReview: would review %d turns (memory: %s)",
            len(recentTurns), existingSnapshot[:min(len(existingSnapshot), 50)])
    }()
}
```

- [ ] **Step 2: Add config fields**

In `internal/config/config.go`, add to `MemoryConfig`:

```go
type MemoryConfig struct {
    // ...existing fields...

    MemoryCharLimit int `json:"memory_char_limit"` // default 2200
    UserCharLimit   int `json:"user_char_limit"`   // default 1375

    Review struct {
        Enabled bool   `json:"enabled"` // default true
        Model   string `json:"model"`   // empty = use main model
    } `json:"review"`
}
```

Set defaults in `DefaultConfig()`:

```go
Memory: MemoryConfig{
    // ...existing defaults...
    MemoryCharLimit: 2200,
    UserCharLimit:   1375,
    Review: struct {
        Enabled bool   `json:"enabled"`
        Model   string `json:"model"`
    }{
        Enabled: true,
    },
},
```

- [ ] **Step 3: Commit**

```bash
git add internal/memory/background_review.go internal/config/config.go
git commit -m "feat(memory): add BackgroundReview skeleton and config"
```

---

### Task 6: SessionSearch Tool — FTS Search for Agent

**Files:**

- Create: `internal/tools/session_search_tool.go`

**Interfaces:**

- Consumes: Manager HTTP API
- Produces: Tool registered in Agent

- [ ] **Step 1: Create `internal/tools/session_search_tool.go`**

```go
package tools

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"

    "github.com/liuscraft/orion-x/internal/logging"
)

const SessionSearchToolName = "session_search"

var SessionSearchSchema = Schema{
    Name: SessionSearchToolName,
    Description: "搜索历史会话记录。使用 FTS5 全文检索，零 LLM 成本。\n\n" +
        "根据传参自动推断模式：\n" +
        "  • 传 q= → 发现模式（搜索匹配的会话）\n" +
        "  • 传 session_id → 查看模式（获取指定会话详情）\n" +
        "  • 什么都不传 → 浏览模式（最近会话列表）\n\n" +
        "session_search 和 memory 的区别：\n" +
        "  • memory = 关键事实，随时在上下文中可用\n" +
        "  • session_search = 按需回忆「我们上周讨论过 X 吗？」",
    Parameters: json.RawMessage(`{
        "type": "object",
        "properties": {
            "q": {
                "type": "string",
                "description": "发现模式：搜索关键词。FTS 全文检索，支持短语匹配"
            },
            "session_id": {
                "type": "string",
                "description": "查看模式：要查看的会话 ID"
            },
            "limit": {
                "type": "integer",
                "description": "最大返回结果数（默认 3，最大 10）"
            }
        }
    }`),
}

type SessionSearchArgs struct {
    Q         string `json:"q,omitempty"`
    SessionID string `json:"session_id,omitempty"`
    Limit     int    `json:"limit,omitempty"`
}

type SessionSearchToolHandler struct {
    managerURL string
    deviceID   string
    client     *http.Client
}

func NewSessionSearchToolHandler(managerURL, deviceID string) *SessionSearchToolHandler {
    return &SessionSearchToolHandler{
        managerURL: strings.TrimRight(managerURL, "/"),
        deviceID:   deviceID,
        client:     &http.Client{},
    }
}

func (h *SessionSearchToolHandler) Handle(ctx context.Context, args json.RawMessage) (string, error) {
    var a SessionSearchArgs
    if err := json.Unmarshal(args, &a); err != nil {
        return "", fmt.Errorf("session_search: parse args: %w", err)
    }

    base := fmt.Sprintf("%s/internal/devices/%s", h.managerURL, h.deviceID)

    var url string
    if a.SessionID != "" {
        // View mode: get specific session
        url = fmt.Sprintf("%s/sessions/%s", base, a.SessionID)
    } else if a.Q != "" {
        // Discover mode: search
        limit := a.Limit
        if limit <= 0 || limit > 10 { limit = 3 }
        url = fmt.Sprintf("%s/turns?q=%s&limit=%d", base, a.Q, limit)
    } else {
        // Browse mode: recent sessions
        url = fmt.Sprintf("%s/turns", base)
    }

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return errorJSON("session_search: create request failed"), nil
    }

    resp, err := h.client.Do(req)
    if err != nil {
        logging.Warnf("SessionSearch: HTTP error: %v", err)
        return errorJSON("检索失败，请稍后重试"), nil
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return errorJSON("读取响应失败"), nil
    }

    return string(body), nil
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/tools/
```

Expected: builds clean.

- [ ] **Step 3: Commit**

```bash
git add internal/tools/session_search_tool.go
git commit -m "feat(tools): add SessionSearch tool with discover/view/browse modes"
```

---

### Task 7: ContextCompressor — Structured Summary

**Files:**

- Create: `internal/memory/compressor.go`

**Interfaces:**

- Consumes: `llm.Client` for summary generation
- Produces: Compression result with structured summary + tail

- [ ] **Step 1: Create `internal/memory/compressor.go`**

```go
package memory

import (
    "context"
    "fmt"
    "strings"

    "github.com/liuscraft/orion-x/internal/llm"
    "github.com/liuscraft/orion-x/internal/logging"
)

const summaryPrefix = `[上下文压缩 — 仅作参考] 前面的对话轮次已被压缩为以下摘要。
这是从之前上下文窗口传递过来的信息——请将其视为背景参考，而非活跃指令。
请勿回答或执行摘要中提到的任何请求——它们已经被处理过了。
你只需要回复出现在此摘要之后的**最新用户消息**。
即使话题有重叠，也以最新用户消息为准。
--- 上下文摘要结束 — 回复下面的消息，不是上面的摘要 ---`

type Compressor struct {
    model       llm.Client
    config      CompressionConfig
    prevSummary string // iterative: old summary fed back
}

type CompressionConfig struct {
    Enabled          bool
    ThresholdPercent float64 // default 0.7
    TailTokenBudget  float64 // default 0.2
    MinTailMessages  int     // default 8
}

type CompressionResult struct {
    Summary   string // the structured summary text
    TailTurns []Turn // protected recent turns
}

func NewCompressor(model llm.Client, config CompressionConfig) *Compressor {
    if config.ThresholdPercent <= 0 { config.ThresholdPercent = 0.7 }
    if config.TailTokenBudget <= 0 { config.TailTokenBudget = 0.2 }
    if config.MinTailMessages <= 0 { config.MinTailMessages = 8 }
    return &Compressor{model: model, config: config}
}

// ShouldCompress estimates whether context usage exceeds threshold.
// headTokens = system prompt + memory snapshot; tailBudget = total * tail ratio.
// If middle (all history not in tail) > threshold * total, compress.
func (c *Compressor) ShouldCompress(turns []Turn, headTokens, totalTokens int) bool {
    if !c.config.Enabled || len(turns) <= c.config.MinTailMessages {
        return false
    }
    tailBudget := int(float64(totalTokens) * c.config.TailTokenBudget)
    // Conservative estimate: ~4 chars/token per turn
    headChars := headTokens * 4
    tailChars := 0
    for i := len(turns) - 1; i >= 0 && tailChars < tailBudget*4; i-- {
        tailChars += len(turns[i].UserText) + len(turns[i].AssistantText)
    }
    middleChars := 0
    for _, t := range turns[:len(turns)-c.config.MinTailMessages] {
        middleChars += len(t.UserText) + len(t.AssistantText)
    }
    used := headChars + middleChars + tailChars
    threshold := int(float64(totalTokens*4) * c.config.ThresholdPercent)
    return used > threshold
}

// Compress generates a structured summary of the given turns.
func (c *Compressor) Compress(ctx context.Context, turns []Turn) (*CompressionResult, error) {
    // Separate tail
    tailBudget := int(float64(len(turns)) * c.config.TailTokenBudget)
    if tailBudget < c.config.MinTailMessages {
        tailBudget = c.config.MinTailMessages
    }
    if tailBudget >= len(turns) {
        tailBudget = len(turns) / 2
    }
    tailStart := len(turns) - tailBudget
    tail := turns[tailStart:]
    middle := turns[:tailStart]

    // Build summarizer prompt
    var b strings.Builder
    b.WriteString("请将以下对话历史压缩为结构化摘要（中文）。\n")
    b.WriteString("输出格式：\n")
    b.WriteString("## Historical Task Snapshot\n（已完成的任务概述）\n\n")
    b.WriteString("## Historical In-Progress State\n（进行中的状态）\n\n")
    b.WriteString("## Historical Pending User Asks\n（用户提过但未完成的需求）\n\n")
    b.WriteString("## Historical Remaining Work\n（剩余工作）\n\n")
    if c.prevSummary != "" {
        b.WriteString("以下是之前已有摘要，请在它的基础上更新和补充：\n")
        b.WriteString(c.prevSummary + "\n\n")
    }
    b.WriteString("需要压缩的对话：\n")
    for _, t := range middle {
        b.WriteString(fmt.Sprintf("用户: %s\n助手: %s\n\n", strings.TrimSpace(t.UserText), strings.TrimSpace(t.AssistantText)))
    }

    req := llm.Request{
        Messages: []llm.Message{
            {Role: "system", Content: "你是一个对话摘要助手，用简洁的中文输出结构化摘要。"},
            {Role: "user", Content: b.String()},
        },
    }

    resp, err := c.model.ChatSync(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("compressor: chat: %w", err)
    }

    summary := strings.TrimSpace(resp.Content)
    if summary == "" {
        return nil, fmt.Errorf("compressor: empty summary")
    }

    // Wrap with restrictive prefix
    fullSummary := summaryPrefix + "\n\n" + summary
    c.prevSummary = summary // store for iterative update

    return &CompressionResult{
        Summary:   fullSummary,
        TailTurns: tail,
    }, nil
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/memory/
```

Expected: builds clean.

- [ ] **Step 3: Commit**

```bash
git add internal/memory/compressor.go
git commit -m "feat(memory): add ContextCompressor with structured summary"
```

---

### Task 8: wsserver Integration — Wire Everything Together

**Files:**

- Modify: `internal/memory/service.go` (rewrite as facade)
- Modify: `internal/memory/types.go` (remove Store interface)
- Modify: `cmd/wsserver/main.go` (HTTP client mode)
- Modify: `cmd/wsserver/connection.go` (per-connection CuratedStore)
- Modify: `internal/agent/agent.go` (wire builtin tools into Run)

**Interfaces:**

- Consumes: All previous tasks' outputs
- Produces: Working memory system in wsserver

- [ ] **Step 1: Rewrite `internal/memory/types.go` — remove Store, keep Turn**

```go
package memory

import "time"

type Mode string

const (
    ModeNone     Mode = "none"
    ModeSession  Mode = "session"
    ModeLongTerm Mode = "long_term"
)

type Turn struct {
    TurnID        int64
    UserText      string
    AssistantText string
    StartedAt     time.Time
    EndedAt       time.Time
    Aborted       bool
    UserID        string
    SessionID     string
    DeviceID      string
}
```

- [ ] **Step 2: Rewrite `internal/memory/service.go` — facade over CuratedStore + Review + Compressor**

```go
package memory

import (
    "context"
    "strings"
    "time"

    "github.com/liuscraft/orion-x/internal/llm"
    "github.com/liuscraft/orion-x/internal/logging"
)

type Options struct {
    SystemPrompt string
    LLM          llm.Client
    ManagerURL   string
    DeviceID     string
    ReviewConfig ReviewConfig
    CompressCfg  CompressionConfig
    Now          func() time.Time
}

type Service struct {
    store         *CuratedStore
    review        *BackgroundReview
    compressor    *Compressor
    sessionBuffer []Turn // local buffer for recent turns
    systemPrompt  string
    now           func() time.Time
}

func NewService(cfg Config, opts Options) (*Service, error) {
    if opts.Now == nil {
        opts.Now = time.Now
    }

    store := NewCuratedStore(opts.ManagerURL, opts.DeviceID, cfg.MemoryCharLimit, cfg.UserCharLimit)
    review := NewBackgroundReview(store, opts.ReviewConfig)

    var compressor *Compressor
    if opts.LLM != nil {
        compressor = NewCompressor(opts.LLM, opts.CompressCfg)
    }

    svc := &Service{
        store:        store,
        review:       review,
        compressor:   compressor,
        systemPrompt: strings.TrimSpace(opts.SystemPrompt),
        now:          opts.Now,
    }

    // Load memory from Manager
    if err := store.Load(context.Background()); err != nil {
        logging.Warnf("Memory: failed to load curated store: %v", err)
    }

    return svc, nil
}

// BuildContextMessages assembles messages for the LLM: system prompt + frozen snapshot + history.
func (s *Service) BuildContextMessages(ctx context.Context, history []*llm.Message) []*llm.Message {
    messages := make([]*llm.Message, 0, 16)
    if s.systemPrompt != "" {
        messages = append(messages, &llm.Message{Role: "system", Content: s.systemPrompt})
    }

    memoryBlock := s.store.FormatForSystemPrompt("memory")
    userBlock := s.store.FormatForSystemPrompt("user")
    if memoryBlock != "" || userBlock != "" {
        var b strings.Builder
        if memoryBlock != "" {
            b.WriteString(memoryBlock)
        }
        if userBlock != "" {
            if b.Len() > 0 {
                b.WriteString("\n\n")
            }
            b.WriteString(userBlock)
        }
        messages = append(messages, &llm.Message{Role: "system", Content: b.String()})
    }

    for _, m := range history {
        messages = append(messages, m)
    }
    return messages
}

// RecordTurn saves a turn and triggers background review.
func (s *Service) RecordTurn(ctx context.Context, turn Turn) error {
    if turn.Aborted {
        return nil
    }
    s.sessionBuffer = append(s.sessionBuffer, turn)
    if len(s.sessionBuffer) > 50 {
        s.sessionBuffer = s.sessionBuffer[len(s.sessionBuffer)-50:]
    }

    // Async: save turn to Manager via HTTP (TODO: wire up)
    // POST /internal/devices/{device_id}/turns

    // Trigger background review
    snapshot := s.store.FormatForSystemPrompt("memory")
    s.review.Spawn(ctx, s.sessionBuffer, snapshot)

    // Check compression (best effort)
    if s.compressor != nil {
        // Estimate: ~4 chars per token, head ~500 tokens for system+memory
        if s.compressor.ShouldCompress(s.sessionBuffer, 500, 8192) {
            result, err := s.compressor.Compress(ctx, s.sessionBuffer)
            if err != nil {
                logging.Warnf("Memory: compression failed: %v", err)
            } else {
                // Flatten: replace session buffer with summary + tail
                compressedTurns := []Turn{{
                    TurnID:        -1,
                    UserText:      "[摘要]",
                    AssistantText: result.Summary,
                    StartedAt:     turn.StartedAt,
                    EndedAt:       turn.EndedAt,
                }}
                compressedTurns = append(compressedTurns, result.TailTurns...)
                s.sessionBuffer = compressedTurns
                logging.Infof("Memory: compression done, %d turns → %d turns + summary",
                    len(s.sessionBuffer), len(result.TailTurns))
            }
        }
    }

    return nil
}

func (s *Service) Close() error {
    // TODO: flush pending writes
    return nil
}

// CuratedStore exposes the underlying store for tool handlers.
func (s *Service) CuratedStore() *CuratedStore {
    return s.store
}
```

- [ ] **Step 3: Update `cmd/wsserver/connection.go` — per-connection memory**

In `handleConnection`, after getting the device config from manager, create per-connection memory service:

```go
// After loading connCfg from manager:
deviceID := userID // or hello.DeviceID

memSvc, err := memory.NewService(memory.Config{
    MemoryCharLimit: connCfg.Memory.MemoryCharLimit,
    UserCharLimit:   connCfg.Memory.UserCharLimit,
}, memory.Options{
    SystemPrompt: agent.DefaultSystemPrompt(),
    ManagerURL:   s.deviceCfg.ManagerURL(), // need to store managerURL in Server
    DeviceID:     deviceID,
    ReviewConfig: memory.ReviewConfig{Enabled: true},
})
if err != nil {
    logging.Warnf("Memory init: %v", err)
    memSvc = nil
}

// Wire tools
if memSvc != nil && memSvc.CuratedStore() != nil {
    store := memSvc.CuratedStore()
    memoryHandler := tools.NewMemoryToolHandler(store)
    connAgent.RegisterBuiltinTool(tools.MemorySchema, memoryHandler.Handle)

    searchHandler := tools.NewSessionSearchToolHandler(managerURL, deviceID)
    connAgent.RegisterBuiltinTool(tools.SessionSearchSchema, searchHandler.Handle)
}
```

- [ ] **Step 4: Add `ManagerURL()` to `DeviceConfigLoader` interface**

In `cmd/wsserver/device_config.go`:

```go
type DeviceConfigLoader interface {
    LoadConfig(deviceID string) (*config.AppConfig, error)
    ManagerURL() string
}
```

Implement on `httpDeviceConfigLoader`:

```go
func (l *httpDeviceConfigLoader) ManagerURL() string {
    return l.managerURL
}
```

- [ ] **Step 5: Pass tools to Agent.Run or equivalent**

In the agent's Run loop (or wherever it processes tool calls), check `builtinTools` before delegating to MCP:

```go
// When processing tool calls in the LLM loop:
if a.builtinTools != nil {
    if tool, ok := a.builtinTools[toolCall.Name]; ok {
        result, err := tool.Handler(ctx, toolCall.Arguments)
        // handle result
        continue // skip MCP dispatch
    }
}
// fall through to MCP registry
```

- [ ] **Step 6: Build**

```bash
go build ./cmd/wsserver/
```

Expected: builds clean.

- [ ] **Step 7: Commit**

```bash
git add internal/memory/service.go internal/memory/types.go cmd/wsserver/main.go cmd/wsserver/connection.go cmd/wsserver/device_config.go internal/agent/agent.go
git commit -m "feat(wsserver): integrate per-connection memory system"
```

---

### Task 9: Cleanup — Remove Old Files

**Files:**

- Delete: `internal/memory/sqlite_store.go`
- Delete: `internal/memory/sqlite_store_test.go`
- Delete: `internal/memory/llm.go`
- Delete: `internal/memory/session_buffer.go`
- Delete: `internal/memory/session_buffer_test.go`
- Delete: `internal/memory/context.go`

- [ ] **Step 1: Remove old files**

```bash
git rm internal/memory/sqlite_store.go internal/memory/sqlite_store_test.go
git rm internal/memory/llm.go
git rm internal/memory/session_buffer.go internal/memory/session_buffer_test.go
git rm internal/memory/context.go
```

- [ ] **Step 2: Remove references in other files**

Search for imports of removed packages:

```bash
grep -r "memory/sqlite" --include="*.go" .
grep -r "memory/llm" --include="*.go" .
```

Fix any remaining references.

- [ ] **Step 3: Build**

```bash
go build ./...
golangci-lint run ./...
```

Expected: builds and lints clean.

- [ ] **Step 4: Commit**

```bash
git commit -m "refactor(memory): remove old sqlite_store, llm, session_buffer"
```

---

### Task 10: Run Full Test Suite

- [ ] **Step 1: Run all tests**

```bash
go test ./... 2>&1
```

- [ ] **Step 2: Run linter**

```bash
golangci-lint run ./... 2>&1
```

- [ ] **Step 3: Fix any issues**

- [ ] **Step 4: Final commit if needed**

```bash
git commit -m "test: fix tests after memory system refactor"
```
