# RAG 文档知识库 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 orion-x 中引入 RAG 文档知识库，支持用户上传文档到智能体知识库，Agent 通过 `knowledge_search` 工具进行向量检索获取文档内容。

**Architecture:** 服务式 + 进程内嵌。Manager 内新增 `internal/knowledge/` 包（Service + parser/chunker/embedder/retriever 子包），store 层新增 3 个 GORM 模型（零向量依赖）。wsserver 侧新增 `knowledge_search` 工具 + HTTP SearchClient。向量由 retriever/pgvector 自行管理私有表，与 store 层解耦。

**Tech Stack:** Go + GORM + PostgreSQL/pgvector + HNSW + OpenAI Embedding API + Gin

**Spec:** `docs/superpowers/specs/2025-07-15-knowledge-base-design.md`

## Global Constraints

- 所有 Go 代码遵循现有项目风格（zap 日志、GORM 模型、provider factory 注册模式）
- wsserver 不直接依赖 pgvector，只通过 HTTP 调用 Manager
- Chunk GORM 模型不含向量列
- 分块默认 recursive，chunk_size=512，overlap=80
- pgvector 用 halfvec 精度 + HNSW cosine 索引
- 入库异步 goroutine，前端轮询 status
- 写完后必须 `golangci-lint run ./...` 无报错

---

### Task 1: Store 层 — GORM 模型 + 基础 CRUD

**Files:**

- Create: `internal/store/knowledge_base.go`
- Create: `internal/store/document.go`
- Create: `internal/store/chunk.go`

**Interfaces:**

- Consumes: 现有 `internal/store/db.go` 的 `Open()` / `AutoMigrate` 模式
- Produces: `KnowledgeBase`, `Document`, `Chunk` 类型；`KnowledgeBaseStore`, `DocumentStore`, `ChunkStore`

- [ ] **Step 1: 创建 KnowledgeBase 模型 + Store**

```go
// internal/store/knowledge_base.go
package store

import "time"

type KnowledgeBase struct {
    ID             string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
    VoicebotID     string    `gorm:"not null;index;type:varchar(36)" json:"voicebot_id"`
    Name           string    `gorm:"not null;type:varchar(128)" json:"name"`
    Description    string    `gorm:"type:text" json:"description,omitempty"`
    EmbeddingModel string    `gorm:"not null;type:varchar(128);default:text-embedding-3-small" json:"embedding_model"`
    EmbeddingDim   int       `gorm:"not null;default:1536" json:"embedding_dim"`
    CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
    UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (KnowledgeBase) TableName() string { return "knowledge_bases" }

type KnowledgeBaseStore struct{ db *gorm.DB }

func NewKnowledgeBaseStore(db *gorm.DB) *KnowledgeBaseStore {
    return &KnowledgeBaseStore{db: db}
}

func (s *KnowledgeBaseStore) Create(kb *KnowledgeBase) error {
    if kb.ID == "" {
        kb.ID = uuid.New().String()
    }
    return s.db.Create(kb).Error
}

func (s *KnowledgeBaseStore) GetByID(id string) (*KnowledgeBase, error) {
    var kb KnowledgeBase
    if err := s.db.First(&kb, "id = ?", id).Error; err != nil {
        return nil, err
    }
    return &kb, nil
}

func (s *KnowledgeBaseStore) ListByVoicebot(voicebotID string) ([]KnowledgeBase, error) {
    var kbs []KnowledgeBase
    if err := s.db.Where("voicebot_id = ?", voicebotID).
        Order("created_at DESC").Find(&kbs).Error; err != nil {
        return nil, err
    }
    return kbs, nil
}

func (s *KnowledgeBaseStore) DeleteByID(id string) error {
    return s.db.Delete(&KnowledgeBase{}, "id = ?", id).Error
}
```

- [ ] **Step 2: 创建 Document 模型 + Store**

```go
// internal/store/document.go
package store

import (
    "time"
    "github.com/google/uuid"
    "gorm.io/gorm"
)

type Document struct {
    ID              string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
    KnowledgeBaseID string    `gorm:"not null;index;type:varchar(36)" json:"knowledge_base_id"`
    Name            string    `gorm:"not null;type:varchar(256)" json:"name"`
    Source          string    `gorm:"not null;type:varchar(16)" json:"source"`
    SourceURL       string    `gorm:"type:text" json:"source_url,omitempty"`
    Status          string    `gorm:"not null;default:pending;type:varchar(16)" json:"status"`
    ChunkCount      int       `gorm:"default:0" json:"chunk_count"`
    CharCount       int       `gorm:"default:0" json:"char_count"`
    ErrorMessage    string    `gorm:"type:text" json:"error_message,omitempty"`
    CreatedAt       time.Time `gorm:"autoCreateTime" json:"created_at"`
    UpdatedAt       time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Document) TableName() string { return "documents" }

type DocumentStore struct{ db *gorm.DB }

func NewDocumentStore(db *gorm.DB) *DocumentStore {
    return &DocumentStore{db: db}
}

func (s *DocumentStore) Create(doc *Document) error {
    if doc.ID == "" {
        doc.ID = uuid.New().String()
    }
    return s.db.Create(doc).Error
}

func (s *DocumentStore) GetByID(id string) (*Document, error) {
    var doc Document
    if err := s.db.First(&doc, "id = ?", id).Error; err != nil {
        return nil, err
    }
    return &doc, nil
}

func (s *DocumentStore) ListByKB(kbID string) ([]Document, error) {
    var docs []Document
    if err := s.db.Where("knowledge_base_id = ?", kbID).
        Order("created_at DESC").Find(&docs).Error; err != nil {
        return nil, err
    }
    return docs, nil
}

func (s *DocumentStore) UpdateStatus(id, status, errMsg string) error {
    return s.db.Model(&Document{}).Where("id = ?", id).Updates(map[string]interface{}{
        "status":        status,
        "error_message": errMsg,
    }).Error
}

func (s *DocumentStore) UpdateChunkInfo(id string, chunkCount, charCount int) error {
    return s.db.Model(&Document{}).Where("id = ?", id).Updates(map[string]interface{}{
        "chunk_count": chunkCount,
        "char_count":  charCount,
    }).Error
}

func (s *DocumentStore) DeleteByID(id string) error {
    return s.db.Delete(&Document{}, "id = ?", id).Error
}
```

- [ ] **Step 3: 创建 Chunk 模型 + Store（纯元数据，零向量依赖）**

```go
// internal/store/chunk.go
package store

import (
    "time"
    "github.com/google/uuid"
    "gorm.io/datatypes"
    "gorm.io/gorm"
)

type Chunk struct {
    ID              string         `gorm:"primaryKey;type:varchar(36)" json:"id"`
    DocumentID      string         `gorm:"not null;index;type:varchar(36)" json:"document_id"`
    KnowledgeBaseID string         `gorm:"not null;index;type:varchar(36)" json:"knowledge_base_id"`
    ChunkIndex      int            `gorm:"not null" json:"chunk_index"`
    Content         string         `gorm:"not null;type:text" json:"content"`
    Metadata        datatypes.JSONMap `gorm:"type:jsonb;default:'{}'" json:"metadata,omitempty"`
    CreatedAt       time.Time     `gorm:"autoCreateTime" json:"created_at"`
}

func (Chunk) TableName() string { return "chunks" }

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
```

- [ ] **Step 4: 注册 AutoMigrate**

在 `internal/store/db.go` 的 `AutoMigrate` 调用中追加 `&KnowledgeBase{}, &Document{}, &Chunk{}`。

- [ ] **Step 5: 验证**

```bash
GOTOOLCHAIN=$(go env GOTOOLCHAIN) go build ./internal/store/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/store/knowledge_base.go internal/store/document.go internal/store/chunk.go internal/store/db.go
git commit -m "feat: add knowledge store models (KnowledgeBase, Document, Chunk)"
```

---

### Task 2: 核心接口定义（parser/chunker/embedder/retriever）

**Files:**

- Create: `internal/knowledge/parser/parser.go`
- Create: `internal/knowledge/chunker/chunker.go`
- Create: `internal/knowledge/embedder/embedder.go`
- Create: `internal/knowledge/retriever/retriever.go`

**Interfaces:**

- Consumes: 无依赖
- Produces: `parser.Parser`, `chunker.Chunker`, `embedder.Embedder`, `retriever.Retriever` 接口；`retriever.SearchResult` 类型

- [ ] **Step 1: Parser 接口**

```go
// internal/knowledge/parser/parser.go
package parser

import (
    "context"
    "io"
    "strings"
)

type Parser interface {
    Parse(ctx context.Context, reader io.Reader, filename string) (string, error)
    SupportedExtensions() []string
}

type Registry struct {
    byExt map[string]Parser
}

func NewRegistry() *Registry {
    return &Registry{byExt: make(map[string]Parser)}
}

func (r *Registry) Register(p Parser) {
    for _, ext := range p.SupportedExtensions() {
        r.byExt[strings.ToLower(ext)] = p
    }
}

func (r *Registry) Find(filename string) (Parser, bool) {
    ext := strings.ToLower(filename[strings.LastIndex(filename, "."):])
    p, ok := r.byExt[ext]
    return p, ok
}
```

- [ ] **Step 2: Chunker 接口**

```go
// internal/knowledge/chunker/chunker.go
package chunker

import "context"

type Chunk struct {
    Index    int
    Content  string
    Metadata map[string]string
}

type Chunker interface {
    Split(ctx context.Context, text string) ([]Chunk, error)
}
```

- [ ] **Step 3: Embedder 接口 + factory 注册**

```go
// internal/knowledge/embedder/embedder.go
package embedder

import (
    "context"
    "fmt"
    "strings"
)

type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dimensions() int
}

// Factory pattern, consistent with provider/asr and provider/tts
type Config struct {
    Type    string // "openai"
    APIKey  string
    BaseURL string
    Model   string
}

type Factory func(Config) (Embedder, error)

var factories = map[string]Factory{}

func Register(etype string, f Factory) {
    factories[strings.ToLower(strings.TrimSpace(etype))] = f
}

func New(cfg Config) (Embedder, error) {
    t := strings.ToLower(strings.TrimSpace(cfg.Type))
    f, ok := factories[t]
    if !ok {
        return nil, fmt.Errorf("unknown embedder type: %q", t)
    }
    return f(cfg)
}
```

- [ ] **Step 4: Retriever 接口**

```go
// internal/knowledge/retriever/retriever.go
package retriever

import "context"

type SearchResult struct {
    ChunkID      string            `json:"chunk_id"`
    Content      string            `json:"content"`
    Score        float64           `json:"score"`
    DocumentID   string            `json:"document_id"`
    DocumentName string            `json:"document_name"`
    Metadata     map[string]string `json:"metadata,omitempty"`
}

type Retriever interface {
    Insert(ctx context.Context, kbID, docID string, chunks []Chunk, vectors [][]float32) error
    DeleteByKB(ctx context.Context, kbID string) error
    DeleteByDocument(ctx context.Context, docID string) error
    Search(ctx context.Context, kbIDs []string, vector []float32, topK int) ([]SearchResult, error)
}

// Chunk mirrors chunker.Chunk to avoid cross-package import for the retriever interface.
// In production, retriever uses its own type to keep the interface self-contained.
type Chunk struct {
    Index    int
    Content  string
    Metadata map[string]string
}
```

- [ ] **Step 5: 验证**

```bash
GOTOOLCHAIN=$(go env GOTOOLCHAIN) go build ./internal/knowledge/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/knowledge/parser/parser.go internal/knowledge/chunker/chunker.go internal/knowledge/embedder/embedder.go internal/knowledge/retriever/retriever.go
git commit -m "feat: add knowledge core interfaces (parser, chunker, embedder, retriever)"
```

---

### Task 3: RecursiveChunker 实现

**Files:**

- Create: `internal/knowledge/chunker/recursive.go`
- Create: `internal/knowledge/chunker/recursive_test.go`

**Interfaces:**

- Consumes: `chunker.Chunker` 接口（Task 2）
- Produces: `RecursiveChunker` 实现

- [ ] **Step 1: 写测试**

```go
// internal/knowledge/chunker/recursive_test.go
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
            t.Errorf("chunk too large: %d > 20", len([]rune(ch.Content)))
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
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
GOTOOLCHAIN=$(go env GOTOOLCHAIN) go test ./internal/knowledge/chunker/ -v -run TestRecursiveChunker
```

Expected: build error (NewRecursive 未定义)

- [ ] **Step 3: 实现 RecursiveChunker**

```go
// internal/knowledge/chunker/recursive.go
package chunker

import (
    "context"
    "strings"
    "unicode/utf8"
)

var defaultSeparators = []string{"\n\n", "\n", "。", ".", " ", ""}

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

type RecursiveChunker struct {
    cfg RecursiveConfig
}

func NewRecursive(cfg RecursiveConfig) *RecursiveChunker {
    cfg.normalize()
    return &RecursiveChunker{cfg: cfg}
}

func (c *RecursiveChunker) Split(ctx context.Context, text string) ([]Chunk, error) {
    if len(strings.TrimSpace(text)) == 0 {
        return nil, nil
    }
    var chunks []Chunk
    c.splitRecursive(text, 0, &chunks)
    // Assign sequential indices
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
        if utf8.RuneCountInString(part) <= c.cfg.ChunkSize {
            if len(strings.TrimSpace(part)) > 0 {
                *chunks = append(*chunks, Chunk{Content: strings.TrimSpace(part)})
            }
        } else {
            c.splitRecursive(part, sepIdx+1, chunks)
        }
    }
}

func (c *RecursiveChunker) hardSplit(text string, chunks *[]Chunk) {
    runes := []rune(text)
    for i := 0; i < len(runes); i += c.cfg.ChunkSize - c.cfg.ChunkOverlap {
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
```

- [ ] **Step 4: 运行测试验证通过**

```bash
GOTOOLCHAIN=$(go env GOTOOLCHAIN) go test ./internal/knowledge/chunker/ -v -run TestRecursiveChunker
```

- [ ] **Step 5: Commit**

```bash
git add internal/knowledge/chunker/recursive.go internal/knowledge/chunker/recursive_test.go
git commit -m "feat: add recursive chunker implementation"
```

---

### Task 4: OpenAI Embedder 实现

**Files:**

- Create: `internal/knowledge/embedder/openai.go`

**Interfaces:**

- Consumes: `embedder.Embedder` 接口 + `embedder.Config`（Task 2）
- Produces: OpenAI-compatible embedder

- [ ] **Step 1: 实现 OpenAI Embedder**

```go
// internal/knowledge/embedder/openai.go
package embedder

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"
)

type openaiEmbedder struct {
    apiKey     string
    baseURL    string
    model      string
    dimensions int
    client     *http.Client
}

func init() {
    Register("openai", NewOpenAI)
    Register("", NewOpenAI) // default
}

func NewOpenAI(cfg Config) (Embedder, error) {
    baseURL := strings.TrimRight(cfg.BaseURL, "/")
    if baseURL == "" {
        baseURL = "https://api.openai.com/v1"
    }
    model := cfg.Model
    if model == "" {
        model = "text-embedding-3-small"
    }
    dims := 1536
    if model == "text-embedding-3-large" {
        dims = 3072
    }
    return &openaiEmbedder{
        apiKey:     cfg.APIKey,
        baseURL:    baseURL,
        model:      model,
        dimensions: dims,
        client:     &http.Client{Timeout: 30 * time.Second},
    }, nil
}

func (e *openaiEmbedder) Dimensions() int { return e.dimensions }

func (e *openaiEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
    type reqBody struct {
        Input []string `json:"input"`
        Model string   `json:"model"`
    }
    type respData struct {
        Data []struct {
            Embedding []float32 `json:"embedding"`
        } `json:"data"`
    }

    body, _ := json.Marshal(reqBody{Input: texts, Model: e.model})
    url := fmt.Sprintf("%s/embeddings", e.baseURL)
    req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
    if err != nil {
        return nil, err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+e.apiKey)

    resp, err := e.client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("embed: request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        msg, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("embed: status %d: %s", resp.StatusCode, string(msg))
    }

    var data respData
    if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
        return nil, fmt.Errorf("embed: decode: %w", err)
    }

    result := make([][]float32, len(data.Data))
    for i, d := range data.Data {
        result[i] = d.Embedding
    }
    return result, nil
}
```

- [ ] **Step 2: 验证**

```bash
GOTOOLCHAIN=$(go env GOTOOLCHAIN) go build ./internal/knowledge/embedder/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/knowledge/embedder/openai.go
git commit -m "feat: add OpenAI embedder implementation"
```

---

### Task 5: PGVector Retriever 实现

**Files:**

- Create: `internal/knowledge/retriever/pgvector.go`

**Interfaces:**

- Consumes: `retriever.Retriever` 接口（Task 2）、`*gorm.DB`、`store.ChunkStore`
- Produces: `PGVectorRetriever` 实现

- [ ] **Step 1: 实现 PGVector**

```go
// internal/knowledge/retriever/pgvector.go
package retriever

import (
    "context"
    "fmt"

    "github.com/liuscraft/orion-x/internal/store"
    "gorm.io/gorm"
)

type PGVectorRetriever struct {
    db         *gorm.DB
    chunkStore *store.ChunkStore
    dimension  int
}

func NewPGVector(db *gorm.DB, dimension int) (*PGVectorRetriever, error) {
    if err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
        return nil, fmt.Errorf("pgvector: create extension: %w", err)
    }
    if err := db.Exec(`
        CREATE TABLE IF NOT EXISTS chunk_vectors (
            chunk_id   VARCHAR(36) PRIMARY KEY REFERENCES chunks(id) ON DELETE CASCADE,
            kb_id      VARCHAR(36) NOT NULL,
            embedding  halfvec(` + fmt.Sprintf("%d", dimension) + `),
            created_at TIMESTAMPTZ DEFAULT NOW()
        )
    `).Error; err != nil {
        return nil, fmt.Errorf("pgvector: create table: %w", err)
    }
    // Create indexes if not exist
    db.Exec("CREATE INDEX IF NOT EXISTS idx_chunk_vectors_kb ON chunk_vectors(kb_id)")
    db.Exec(fmt.Sprintf(
        "CREATE INDEX IF NOT EXISTS idx_chunk_vectors_embedding ON chunk_vectors USING hnsw (embedding halfvec_cosine_ops) WITH (m = 16, ef_construction = 200)",
    ))
    return &PGVectorRetriever{
        db:         db,
        chunkStore: store.NewChunkStore(db),
        dimension:  dimension,
    }, nil
}

func (r *PGVectorRetriever) Insert(ctx context.Context, kbID, docID string, chunks []Chunk, vectors [][]float32) error {
    if len(chunks) != len(vectors) {
        return fmt.Errorf("pgvector: chunks/vectors length mismatch: %d vs %d", len(chunks), len(vectors))
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
        // 2. Build vector INSERT
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

func (r *PGVectorRetriever) DeleteByKB(ctx context.Context, kbID string) error {
    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if err := tx.Exec("DELETE FROM chunk_vectors WHERE kb_id = ?", kbID).Error; err != nil {
            return err
        }
        return store.NewChunkStore(tx).DeleteByKB(kbID)
    })
}

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

func (r *PGVectorRetriever) Search(ctx context.Context, kbIDs []string, vector []float32, topK int) ([]SearchResult, error) {
    vecStr := float32ToHalfVec(vector)
    type row struct {
        ChunkID      string  `gorm:"column:chunk_id"`
        Content      string  `gorm:"column:content"`
        Score        float64 `gorm:"column:score"`
        DocumentID   string  `gorm:"column:document_id"`
        DocumentName string  `gorm:"column:document_name"`
    }
    var rows []row
    query := r.db.WithContext(ctx).Raw(`
        SELECT cv.chunk_id, c.content,
               1 - (cv.embedding <=> ?::halfvec) AS score,
               c.document_id, d.name AS document_name
        FROM chunk_vectors cv
        JOIN chunks c ON c.id = cv.chunk_id
        JOIN documents d ON d.id = c.document_id
        WHERE cv.kb_id IN ?
        ORDER BY cv.embedding <=> ?::halfvec
        LIMIT ?
    `, vecStr, kbIDs, vecStr, topK*2)
    if err := query.Scan(&rows).Error; err != nil {
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

func float32ToHalfVec(v []float32) string {
    b := &strings.Builder{}
    b.WriteString("[")
    for i, f := range v {
        if i > 0 {
            b.WriteString(",")
        }
        fmt.Fprintf(b, "%f", f)
    }
    b.WriteString("]")
    return b.String()
}
```

需要在文件顶部补充 `"strings"` import。

- [ ] **Step 2: 验证**

```bash
GOTOOLCHAIN=$(go env GOTOOLCHAIN) go build ./internal/knowledge/retriever/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/knowledge/retriever/pgvector.go
git commit -m "feat: add pgvector retriever implementation"
```

---

### Task 6: Parser 实现（TXT + URL）

**Files:**

- Create: `internal/knowledge/parser/text.go`
- Create: `internal/knowledge/parser/url.go`

**Interfaces:**

- Consumes: `parser.Parser` 接口（Task 2）
- Produces: TextParser, URLParser

- [ ] **Step 1: TextParser**

```go
// internal/knowledge/parser/text.go
package parser

import (
    "context"
    "io"
    "strings"
)

type TextParser struct{}

func NewTextParser() *TextParser { return &TextParser{} }

func (p *TextParser) SupportedExtensions() []string {
    return []string{".txt", ".md", ".markdown", ".go", ".py", ".ts", ".js", ".yaml", ".yml", ".json", ".xml", ".html", ".css", ".sh", ".sql", ".env", ".log"}
}

func (p *TextParser) Parse(_ context.Context, reader io.Reader, _ string) (string, error) {
    data, err := io.ReadAll(reader)
    if err != nil {
        return "", err
    }
    return strings.TrimSpace(string(data)), nil
}
```

- [ ] **Step 2: URLParser（简单 HTTP 抓取）**

```go
// internal/knowledge/parser/url.go
package parser

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"
)

type URLParser struct {
    client *http.Client
}

func NewURLParser() *URLParser {
    return &URLParser{client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *URLParser) SupportedExtensions() []string {
    return []string{".url"} // virtual extension, matched by source="url"
}

func (p *URLParser) Parse(ctx context.Context, _ io.Reader, _ string) (string, error) {
    // URL parsing is triggered by the service layer, not file extension.
    // This is a placeholder — the actual URL content is fetched in IngestDocument.
    return "", fmt.Errorf("URLParser: use Service.IngestURL instead")
}
```

- [ ] **Step 3: 构建 Parser Registry helper**

在 `parser/parser.go` 中添加 `DefaultRegistry()`:

```go
func DefaultRegistry() *Registry {
    r := NewRegistry()
    r.Register(NewTextParser())
    r.Register(NewURLParser())
    return r
}
```

- [ ] **Step 4: 验证**

```bash
GOTOOLCHAIN=$(go env GOTOOLCHAIN) go build ./internal/knowledge/parser/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/knowledge/parser/text.go internal/knowledge/parser/url.go internal/knowledge/parser/parser.go
git commit -m "feat: add text and url parsers"
```

---

### Task 7: knowledge.Service（Manager 侧入口）

**Files:**

- Create: `internal/knowledge/service.go`

**Interfaces:**

- Consumes: `store.KnowledgeBaseStore`, `store.DocumentStore`, `parser.Parser`, `chunker.Chunker`, `embedder.Embedder`, `retriever.Retriever`
- Produces: `Service` 类型

- [ ] **Step 1: 实现 Service**

```go
// internal/knowledge/service.go
package knowledge

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"

    "github.com/liuscraft/orion-x/internal/knowledge/chunker"
    "github.com/liuscraft/orion-x/internal/knowledge/embedder"
    "github.com/liuscraft/orion-x/internal/knowledge/parser"
    "github.com/liuscraft/orion-x/internal/knowledge/retriever"
    "github.com/liuscraft/orion-x/internal/logging"
    "github.com/liuscraft/orion-x/internal/store"
)

type Config struct {
    Embedding embedder.Config
    Chunk     chunker.RecursiveConfig
}

type Service struct {
    kbStore   *store.KnowledgeBaseStore
    docStore  *store.DocumentStore
    parserReg *parser.Registry
    chunker   chunker.Chunker
    embedder  embedder.Embedder
    retriever retriever.Retriever
}

func NewService(
    kbStore *store.KnowledgeBaseStore,
    docStore *store.DocumentStore,
    retriever retriever.Retriever,
    embedder embedder.Embedder,
) *Service {
    return &Service{
        kbStore:   kbStore,
        docStore:  docStore,
        parserReg: parser.DefaultRegistry(),
        chunker:   chunker.NewRecursive(chunker.RecursiveConfig{}),
        embedder:  embedder,
        retriever: retriever,
    }
}

func (s *Service) CreateKB(ctx context.Context, voicebotID, name, desc, embeddingModel string) (*store.KnowledgeBase, error) {
    if embeddingModel == "" {
        embeddingModel = "text-embedding-3-small"
    }
    kb := &store.KnowledgeBase{
        VoicebotID:     voicebotID,
        Name:           name,
        Description:    desc,
        EmbeddingModel: embeddingModel,
        EmbeddingDim:   s.embedder.Dimensions(),
    }
    if err := s.kbStore.Create(kb); err != nil {
        return nil, fmt.Errorf("create kb: %w", err)
    }
    return kb, nil
}

func (s *Service) DeleteKB(ctx context.Context, kbID string) error {
    // Delete vectors first, then chunks, then documents, then KB
    if err := s.retriever.DeleteByKB(ctx, kbID); err != nil {
        logging.Warnf("Knowledge: delete vectors for kb %s: %v", kbID, err)
    }
    return s.kbStore.DeleteByID(kbID)
}

func (s *Service) ListKBs(ctx context.Context, voicebotID string) ([]store.KnowledgeBase, error) {
    return s.kbStore.ListByVoicebot(voicebotID)
}

func (s *Service) GetKB(ctx context.Context, kbID string) (*store.KnowledgeBase, error) {
    return s.kbStore.GetByID(kbID)
}

func (s *Service) ListDocuments(ctx context.Context, kbID string) ([]store.Document, error) {
    return s.docStore.ListByKB(kbID)
}

func (s *Service) IngestDocument(ctx context.Context, kbID string, reader io.Reader, filename, source string) (*store.Document, error) {
    doc := &store.Document{
        KnowledgeBaseID: kbID,
        Name:            filename,
        Source:          source,
    }
    if err := s.docStore.Create(doc); err != nil {
        return nil, fmt.Errorf("create document: %w", err)
    }
    go s.ingestAsync(doc.ID, kbID, reader, filename, source)
    return doc, nil
}

func (s *Service) IngestURL(ctx context.Context, kbID, urlStr string) (*store.Document, error) {
    resp, err := http.Get(urlStr)
    if err != nil {
        return nil, fmt.Errorf("fetch url: %w", err)
    }
    defer resp.Body.Close()
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("read url body: %w", err)
    }
    name := urlStr
    if idx := strings.LastIndex(urlStr, "/"); idx >= 0 {
        name = urlStr[idx+1:]
    }
    if name == "" {
        name = urlStr
    }
    reader := strings.NewReader(string(body))
    return s.IngestDocument(ctx, kbID, reader, name, "url")
}

func (s *Service) DeleteDocument(ctx context.Context, docID string) error {
    if err := s.retriever.DeleteByDocument(ctx, docID); err != nil {
        logging.Warnf("Knowledge: delete vectors for doc %s: %v", docID, err)
    }
    return s.docStore.DeleteByID(docID)
}

func (s *Service) Search(ctx context.Context, kbIDs []string, query string, topK int) ([]retriever.SearchResult, error) {
    if topK <= 0 || topK > 10 {
        topK = 5
    }
    vectors, err := s.embedder.Embed(ctx, []string{query})
    if err != nil {
        return nil, fmt.Errorf("embed query: %w", err)
    }
    if len(vectors) == 0 {
        return nil, nil
    }
    return s.retriever.Search(ctx, kbIDs, vectors[0], topK)
}

func (s *Service) GetDocumentStatus(ctx context.Context, docID string) (*store.Document, error) {
    return s.docStore.GetByID(docID)
}

// ingestAsync runs the full ingestion pipeline in a background goroutine.
func (s *Service) ingestAsync(docID, kbID string, reader io.Reader, filename, source string) {
    ctx := context.Background()
    if err := s.docStore.UpdateStatus(docID, "parsing", ""); err != nil {
        logging.Errorf("Knowledge[%s]: update status: %v", docID, err)
        return
    }

    var text string
    if source == "url" {
        data, err := io.ReadAll(reader)
        if err != nil {
            s.failDoc(docID, fmt.Sprintf("read url content: %v", err))
            return
        }
        text = strings.TrimSpace(string(data))
    } else {
        p, ok := s.parserReg.Find(filename)
        if !ok {
            // Fallback to plain text reader
            data, err := io.ReadAll(reader)
            if err != nil {
                s.failDoc(docID, fmt.Sprintf("read file: %v", err))
                return
            }
            text = strings.TrimSpace(string(data))
        } else {
            var err error
            text, err = p.Parse(ctx, reader, filename)
            if err != nil {
                s.failDoc(docID, fmt.Sprintf("parse: %v", err))
                return
            }
        }
    }
    if text == "" {
        s.failDoc(docID, "empty content")
        return
    }

    if err := s.docStore.UpdateStatus(docID, "chunking", ""); err != nil {
        logging.Errorf("Knowledge[%s]: update status: %v", docID, err)
        return
    }
    chunks, err := s.chunker.Split(ctx, text)
    if err != nil {
        s.failDoc(docID, fmt.Sprintf("chunk: %v", err))
        return
    }

    if err := s.docStore.UpdateStatus(docID, "embedding", ""); err != nil {
        logging.Errorf("Knowledge[%s]: update status: %v", docID, err)
        return
    }
    contents := make([]string, len(chunks))
    for i, c := range chunks {
        contents[i] = c.Content
    }
    vectors, err := s.embedder.Embed(ctx, contents)
    if err != nil {
        s.failDoc(docID, fmt.Sprintf("embed: %v", err))
        return
    }

    if err := s.docStore.UpdateStatus(docID, "storing", ""); err != nil {
        logging.Errorf("Knowledge[%s]: update status: %v", docID, err)
        return
    }
    retChunks := make([]retriever.Chunk, len(chunks))
    for i, c := range chunks {
        retChunks[i] = retriever.Chunk{
            Index:    c.Index,
            Content:  c.Content,
            Metadata: c.Metadata,
        }
    }
    if err := s.retriever.Insert(ctx, kbID, docID, retChunks, vectors); err != nil {
        s.failDoc(docID, fmt.Sprintf("store vectors: %v", err))
        return
    }

    charCount := len([]rune(text))
    if err := s.docStore.UpdateChunkInfo(docID, len(chunks), charCount); err != nil {
        logging.Errorf("Knowledge[%s]: update chunk info: %v", docID, err)
    }
    if err := s.docStore.UpdateStatus(docID, "ready", ""); err != nil {
        logging.Errorf("Knowledge[%s]: update status: %v", docID, err)
    }
    logging.Infof("Knowledge[%s]: ingested %d chunks, %d chars", docID, len(chunks), charCount)
}

func (s *Service) failDoc(docID, errMsg string) {
    if err := s.docStore.UpdateStatus(docID, "error", errMsg); err != nil {
        logging.Errorf("Knowledge[%s]: fail doc: %v", docID, err)
    }
    logging.Errorf("Knowledge[%s]: %s", docID, errMsg)
}
```

- [ ] **Step 2: 验证**

```bash
GOTOOLCHAIN=$(go env GOTOOLCHAIN) go build ./internal/knowledge/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/knowledge/service.go
git commit -m "feat: add knowledge service (CRUD + ingestion + search)"
```

---

### Task 8: SearchClient（wsserver 侧 HTTP 客户端）

**Files:**

- Create: `internal/knowledge/search_client.go`

**Interfaces:**

- Consumes: 无（独立 HTTP 客户端）
- Produces: `SearchClient` 类型，`SearchResultItem` 类型

- [ ] **Step 1: 实现 SearchClient**

```go
// internal/knowledge/search_client.go
package knowledge

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "strings"
    "time"

    "github.com/liuscraft/orion-x/internal/logging"
)

type SearchResultItem struct {
    ChunkID      string  `json:"chunk_id"`
    Content      string  `json:"content"`
    Score        float64 `json:"score"`
    DocumentName string  `json:"document_name"`
}

type SearchClient struct {
    managerURL string
    deviceID   string
    client     *http.Client
}

func NewSearchClient(managerURL, deviceID string) *SearchClient {
    return &SearchClient{
        managerURL: strings.TrimRight(managerURL, "/"),
        deviceID:   deviceID,
        client:     &http.Client{Timeout: 15 * time.Second},
    }
}

func (c *SearchClient) Search(ctx context.Context, query string, topK int) ([]SearchResultItem, error) {
    if topK <= 0 || topK > 10 {
        topK = 5
    }
    addr := fmt.Sprintf("%s/internal/knowledge/search?q=%s&device_id=%s&top_k=%d",
        c.managerURL, url.QueryEscape(query), c.deviceID, topK)

    req, err := http.NewRequestWithContext(ctx, "GET", addr, nil)
    if err != nil {
        return nil, fmt.Errorf("search_client: create request: %w", err)
    }

    resp, err := c.client.Do(req)
    if err != nil {
        logging.Warnf("SearchClient: HTTP error: %v", err)
        return nil, fmt.Errorf("search_client: request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("search_client: status %d", resp.StatusCode)
    }

    var items []SearchResultItem
    if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
        return nil, fmt.Errorf("search_client: decode: %w", err)
    }
    return items, nil
}
```

- [ ] **Step 2: 验证**

```bash
GOTOOLCHAIN=$(go env GOTOOLCHAIN) go build ./internal/knowledge/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/knowledge/search_client.go
git commit -m "feat: add knowledge SearchClient (HTTP client)"
```

---

### Task 9: knowledge_search 工具（wsserver 侧）

**Files:**

- Create: `internal/tools/knowledge_search_tool.go`

**Interfaces:**

- Consumes: `knowledge.SearchClient`（Task 8）、`tools.Spec`
- Produces: `KnowledgeSearchToolName` 常量、`KnowledgeSearchToolSpec()` 函数

- [ ] **Step 1: 实现工具**

```go
// internal/tools/knowledge_search_tool.go
package tools

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/liuscraft/orion-x/internal/knowledge"
    "github.com/liuscraft/orion-x/internal/logging"
)

const KnowledgeSearchToolName = "knowledge_search"

func KnowledgeSearchToolSpec(client *knowledge.SearchClient) Spec {
    return Spec{
        Name: KnowledgeSearchToolName,
        Description: "搜索知识库文档。通过语义检索从上传的文档中查找相关内容，零额外 LLM 成本。\n\n" +
            "knowledge_search 和 memory / session_search 的区别：\n" +
            "  • memory = 你的笔记（偏好、事实、技巧），随时在上下文中可用\n" +
            "  • session_search = 按需回忆历史对话「我们上周讨论过 X 吗？」\n" +
            "  • knowledge_search = 检索用户上传的文档资料，如产品文档、技术手册",
        Parameters: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "q": map[string]any{
                    "type":        "string",
                    "description": "搜索关键词或问题",
                },
                "top_k": map[string]any{
                    "type":        "integer",
                    "description": "返回结果数（默认 5，最大 10）",
                },
            },
            "required": []any{"q"},
        },
        Execute: func(ctx context.Context, args json.RawMessage) (Result, error) {
            var a struct {
                Q     string `json:"q"`
                TopK  int    `json:"top_k,omitempty"`
            }
            if err := json.Unmarshal(args, &a); err != nil {
                return Result{}, fmt.Errorf("knowledge_search: parse args: %w", err)
            }
            if a.Q == "" {
                return Result{Output: mustJSON(map[string]any{
                    "success": false, "error": "q 不能为空",
                })}, nil
            }
            items, err := client.Search(ctx, a.Q, a.TopK)
            if err != nil {
                logging.Warnf("KnowledgeSearch: %v", err)
                return Result{Output: mustJSON(map[string]any{
                    "success": false, "error": "检索失败，请稍后重试",
                })}, nil
            }
            return Result{Output: mustJSON(map[string]any{
                "success": true,
                "results": items,
            })}, nil
        },
    }
}
```

- [ ] **Step 2: 验证**

```bash
GOTOOLCHAIN=$(go env GOTOOLCHAIN) go build ./internal/tools/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/tools/knowledge_search_tool.go
git commit -m "feat: add knowledge_search agent tool"
```

---

### Task 10: Manager HTTP API（handler + router）

**Files:**

- Create: `cmd/manager/handler/data_knowledge.go`
- Modify: `cmd/manager/server.go`（注册路由）
- Modify: `cmd/manager/main.go`（注入依赖）

**Interfaces:**

- Consumes: `knowledge.Service`（Task 7）、`store.DeviceStore`, `store.VoicebotStore`
- Produces: HTTP 端点

- [ ] **Step 1: 创建 handler**

```go
// cmd/manager/handler/data_knowledge.go
package handler

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    "github.com/liuscraft/orion-x/internal/knowledge"
    "github.com/liuscraft/orion-x/internal/logging"
    "github.com/liuscraft/orion-x/internal/store"
)

type DataKnowledgeHandler struct {
    kbSvc       *knowledge.Service
    kbStore     *store.KnowledgeBaseStore
    docStore    *store.DocumentStore
    deviceStore *store.DeviceStore
    botStore    *store.VoicebotStore
}

func NewDataKnowledgeHandler(
    kbSvc *knowledge.Service,
    kbStore *store.KnowledgeBaseStore,
    docStore *store.DocumentStore,
    deviceStore *store.DeviceStore,
    botStore *store.VoicebotStore,
) *DataKnowledgeHandler {
    return &DataKnowledgeHandler{
        kbSvc:       kbSvc,
        kbStore:     kbStore,
        docStore:    docStore,
        deviceStore: deviceStore,
        botStore:    botStore,
    }
}

// ── 知识库 CRUD ──

func (h *DataKnowledgeHandler) ListKBs(c *gin.Context) {
    userID := c.GetString("userID")
    botID := c.Param("bot_id")
    bot, err := h.botStore.GetByID(botID)
    if err != nil || bot.OwnerID != userID {
        c.JSON(http.StatusForbidden, gin.H{"error": "无权访问"})
        return
    }
    kbs, err := h.kbSvc.ListKBs(c.Request.Context(), botID)
    if err != nil {
        logging.Errorf("DataKnowledge ListKBs bot=%s: %v", botID, err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "查询知识库失败"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"knowledge_bases": kbs})
}

func (h *DataKnowledgeHandler) CreateKB(c *gin.Context) {
    userID := c.GetString("userID")
    botID := c.Param("bot_id")
    bot, err := h.botStore.GetByID(botID)
    if err != nil || bot.OwnerID != userID {
        c.JSON(http.StatusForbidden, gin.H{"error": "无权访问"})
        return
    }
    var req struct {
        Name           string `json:"name"`
        Description    string `json:"description"`
        EmbeddingModel string `json:"embedding_model"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
        return
    }
    if req.Name == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "名称不能为空"})
        return
    }
    kb, err := h.kbSvc.CreateKB(c.Request.Context(), botID, req.Name, req.Description, req.EmbeddingModel)
    if err != nil {
        logging.Errorf("DataKnowledge CreateKB: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "创建知识库失败"})
        return
    }
    c.JSON(http.StatusCreated, kb)
}

func (h *DataKnowledgeHandler) GetKB(c *gin.Context) {
    kbID := c.Param("kb_id")
    kb, err := h.kbSvc.GetKB(c.Request.Context(), kbID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "知识库不存在"})
        return
    }
    c.JSON(http.StatusOK, kb)
}

func (h *DataKnowledgeHandler) DeleteKB(c *gin.Context) {
    kbID := c.Param("kb_id")
    if err := h.kbSvc.DeleteKB(c.Request.Context(), kbID); err != nil {
        logging.Errorf("DataKnowledge DeleteKB id=%s: %v", kbID, err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
        return
    }
    c.Status(http.StatusNoContent)
}

// ── 文档管理 ──

func (h *DataKnowledgeHandler) ListDocuments(c *gin.Context) {
    kbID := c.Param("kb_id")
    docs, err := h.kbSvc.ListDocuments(c.Request.Context(), kbID)
    if err != nil {
        logging.Errorf("DataKnowledge ListDocuments kb=%s: %v", kbID, err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "查询文档失败"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"documents": docs})
}

func (h *DataKnowledgeHandler) UploadDocument(c *gin.Context) {
    kbID := c.Param("kb_id")
    file, err := c.FormFile("file")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "请上传文件"})
        return
    }
    f, err := file.Open()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件失败"})
        return
    }
    defer f.Close()
    doc, err := h.kbSvc.IngestDocument(c.Request.Context(), kbID, f, file.Filename, "file")
    if err != nil {
        logging.Errorf("DataKnowledge UploadDocument: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "上传文档失败"})
        return
    }
    c.JSON(http.StatusCreated, doc)
}

func (h *DataKnowledgeHandler) DeleteDocument(c *gin.Context) {
    docID := c.Param("doc_id")
    if err := h.kbSvc.DeleteDocument(c.Request.Context(), docID); err != nil {
        logging.Errorf("DataKnowledge DeleteDocument id=%s: %v", docID, err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
        return
    }
    c.Status(http.StatusNoContent)
}

func (h *DataKnowledgeHandler) GetDocumentStatus(c *gin.Context) {
    docID := c.Param("doc_id")
    doc, err := h.kbSvc.GetDocumentStatus(c.Request.Context(), docID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "文档不存在"})
        return
    }
    c.JSON(http.StatusOK, doc)
}

// ── 内部检索 API ──

func (h *DataKnowledgeHandler) Search(c *gin.Context) {
    deviceID := c.Query("device_id")
    if deviceID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "device_id is required"})
        return
    }
    query := c.Query("q")
    if query == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "q is required"})
        return
    }
    topK := 5
    if v := c.Query("top_k"); v != "" {
        if n, err := parseInt(v); err == nil && n > 0 && n <= 10 {
            topK = n
        }
    }

    // Resolve device_id → voicebot_id
    dev, err := h.deviceStore.GetByID(deviceID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "设备不存在"})
        return
    }
    kbs, err := h.kbStore.ListByVoicebot(dev.VoicebotID)
    if err != nil {
        logging.Errorf("DataKnowledge Search: list kbs for bot %s: %v", dev.VoicebotID, err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "检索失败"})
        return
    }
    if len(kbs) == 0 {
        c.JSON(http.StatusOK, []any{})
        return
    }
    kbIDs := make([]string, len(kbs))
    for i, kb := range kbs {
        kbIDs[i] = kb.ID
    }

    results, err := h.kbSvc.Search(c.Request.Context(), kbIDs, query, topK)
    if err != nil {
        logging.Errorf("DataKnowledge Search: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "检索失败"})
        return
    }
    c.JSON(http.StatusOK, results)
}
```

- [ ] **Step 2: 注册路由**

在 `cmd/manager/server.go` 的 `newRouter` 函数中：

1. 参数列表追加：`kbSvc *knowledge.Service, kbStore *store.KnowledgeBaseStore, docStore *store.DocumentStore`
2. 创建 handler：`dataKnowH := handler.NewDataKnowledgeHandler(kbSvc, kbStore, docStore, devices, voicebots)`
3. 注册管理端路由（在 jwtMw 下）：

```go
// 知识库管理
knowledgeAPI := data.Group("/knowledge")
knowledgeAPI.GET("/bots/:bot_id/knowledge_bases", dataKnowH.ListKBs)
knowledgeAPI.POST("/bots/:bot_id/knowledge_bases", dataKnowH.CreateKB)
knowledgeAPI.GET("/knowledge_bases/:kb_id", dataKnowH.GetKB)
knowledgeAPI.DELETE("/knowledge_bases/:kb_id", dataKnowH.DeleteKB)
knowledgeAPI.GET("/knowledge_bases/:kb_id/documents", dataKnowH.ListDocuments)
knowledgeAPI.POST("/knowledge_bases/:kb_id/documents", dataKnowH.UploadDocument)
knowledgeAPI.DELETE("/documents/:doc_id", dataKnowH.DeleteDocument)
knowledgeAPI.GET("/documents/:doc_id/status", dataKnowH.GetDocumentStatus)
```
1. 注册内部检索路由：

```go
internal.GET("/knowledge/search", dataKnowH.Search)
```

- [ ] **Step 3: 在 cmd/manager/main.go 中注入依赖**

在 `main.go` 中：

1. 创建 embedder：`emb, err := embedder.New(embedder.Config{...})`
2. 创建 retriever：`ret, err := retriever.NewPGVector(db, emb.Dimensions())`
3. 创建 knowledge service：`kbSvc := knowledge.NewService(kbStore, docStore, ret, emb)`
4. 传入 `newRouter(...)`

- [ ] **Step 4: 验证**

```bash
GOTOOLCHAIN=$(go env GOTOOLCHAIN) go build ./cmd/manager/...
```

- [ ] **Step 5: Commit**

```bash
git add cmd/manager/handler/data_knowledge.go cmd/manager/server.go cmd/manager/main.go
git commit -m "feat: add knowledge HTTP API and router integration"
```

---

### Task 11: wsserver 集 knowledge_search 工具注册

**Files:**

- Modify: `cmd/wsserver/connection.go`

**Interfaces:**

- Consumes: `tools.KnowledgeSearchToolSpec`（Task 9）、`knowledge.SearchClient`（Task 8）
- Produces: wsserver 侧 knowledge_search 工具注册

- [ ] **Step 1: 修改 connection.go**

在 `newConnection` 中，注册 builtin tools 的代码块里追加：

```go
// Register knowledge_search tool for this connection
knowClient := knowledge.NewSearchClient(s.deviceCfg.ManagerURL(), deviceID)
connAgent.RegisterBuiltinTool(tools.KnowledgeSearchToolSpec(knowClient))
logging.Infof("wsserver[%s]: registered knowledge search tool", sessionID)
```

需要添加 import：`"github.com/liuscraft/orion-x/internal/knowledge"`

- [ ] **Step 2: 验证**

```bash
GOTOOLCHAIN=$(go env GOTOOLCHAIN) go build ./cmd/wsserver/...
```

- [ ] **Step 3: Commit**

```bash
git add cmd/wsserver/connection.go
git commit -m "feat: register knowledge_search tool in wsserver"
```

---

### Task 12: 最终验证 + Lint

- [ ] **Step 1: 全量构建**

```bash
GOTOOLCHAIN=$(go env GOTOOLCHAIN) go build ./...
```

- [ ] **Step 2: Lint**

```bash
golangci-lint run ./...
```

- [ ] **Step 3: 修复所有 lint 问题后 Commit**

```bash
git add -A && git commit -m "chore: fix lint issues after knowledge base implementation"
```

---

## 依赖关系图

```
Task 1 (store models)
  └→ Task 2 (core interfaces)
       ├→ Task 3 (chunker) ──────────────┐
       ├→ Task 4 (embedder) ────────────┤
       ├→ Task 5 (pgvector) ────────────┤
       └→ Task 6 (parsers) ─────────────┤
                                         ▼
                                  Task 7 (service)
                                   ├→ Task 8 (SearchClient) ──┐
                                   │                           ▼
                                   │                    Task 9 (tool)
                                   │                         │
                                   ▼                         ▼
                              Task 10 (Manager API)   Task 11 (wsserver)
                                                         │
                                                         ▼
                                                  Task 12 (lint)
```
