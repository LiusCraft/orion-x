# RAG 文档知识库设计

## 1. 目标

在 orion-x 现有记忆系统（memory）之上，引入独立的 RAG 文档知识库（knowledge），支持用户上传文档/URL 到智能体知识库，Agent 通过向量检索获取文档内容回答问题。

与现有 memory 系统的关系：**独立、并列**。

| | memory | knowledge |
| --- | --- | --- |
| **定位** | Agent 对用户/环境的记忆（小笔记） | 用户上传的文档资料（长文本） |
| **数据来源** | Agent 自主记忆、用户画像 | 用户上传 PDF/Markdown/TXT/URL |
| **写入时机** | 每轮对话后异步 review | 文档上传时异步 pipeline |
| **检索方式** | 全量注入 System Prompt | 按 query 向量检索 topK |
| **工具** | `memory` 工具 | `knowledge_search` 工具 |
| **容量** | 2200/1375 字符硬限制 | 无硬限制，分 chunk 向量检索 |

## 2. 架构

### 2.1 部署模型

服务式 + 进程内嵌：知识库逻辑在 Manager 进程内，wsserver 通过 HTTP 调用检索。

```
┌── cmd/manager ────────────────────────────────────────┐
│                                                        │
│  handler/data_knowledge.go    ← HTTP API（写入+检索）    │
│         │                                              │
│         ▼                                              │
│  knowledge.Service                                     │
│  ├── parser/           PDF/MD/TXT/URL 解析              │
│  ├── chunker/          递归字符分割                      │
│  ├── embedder/         OpenAI 兼容 Embedding API        │
│  └── retriever/        向量存储+检索（可插拔）             │
│       └── pgvector/    pgvector 实现                   │
│       └── (qdrant/)    未来实现                          │
│         │                                              │
│         ▼                                              │
│  store/ (GORM)      KnowledgeBase, Document, Chunk     │
│  PostgreSQL         pgvector 向量索引                   │
└────────────────────────────────────────────────────────┘
                         ▲ HTTP
                         │
┌── cmd/wsserver ───────────────────────────────────────┐
│  tools/knowledge_search_tool.go                        │
│    → HTTP GET /internal/knowledge/search              │
│    → 纯 HTTP 客户端，零依赖                              │
│  knowledge/search_client.go                            │
│    → SearchClient 结构体（对标 memory.CuratedStore）     │
└────────────────────────────────────────────────────────┘
```

### 2.2 模块职责

```
internal/knowledge/
├── service.go            # Service 入口（Manager 侧）
├── search_client.go      # HTTP 客户端（wsserver 侧）
├── parser/
│   └── parser.go         # Parser 接口 + 注册表
├── chunker/
│   ├── chunker.go        # Chunker 接口
│   └── recursive.go      # 递归字符分割（默认）
├── embedder/
│   ├── embedder.go       # Embedder 接口 + factory
│   └── openai.go         # OpenAI 兼容实现
└── retriever/
    ├── retriever.go      # Service 接口（Index/Search/Delete）
    ├── pgvector.go       # pgvector 实现
    └── (qdrant.go)       # 未来

internal/store/            # GORM 模型，零向量依赖
├── knowledge_base.go     # KnowledgeBase
├── document.go           # Document
└── chunk.go              # Chunk（纯元数据，不含向量列）
```

## 3. 数据模型

### 3.1 GORM 模型（store 层）

```go
type KnowledgeBase struct {
    ID             string    `gorm:"primaryKey;type:varchar(36)"`
    VoicebotID     string    `gorm:"not null;index;type:varchar(36)"`
    Name           string    `gorm:"not null;type:varchar(128)"`
    Description    string    `gorm:"type:text"`
    EmbeddingModel string    `gorm:"not null;type:varchar(128);default:text-embedding-3-small"`
    EmbeddingDim   int       `gorm:"not null;default:1536"`
    CreatedAt      time.Time
    UpdatedAt      time.Time
}

type Document struct {
    ID              string    `gorm:"primaryKey;type:varchar(36)"`
    KnowledgeBaseID string    `gorm:"not null;index;type:varchar(36)"`
    Name            string    `gorm:"not null;type:varchar(256)"`
    Source          string    `gorm:"not null;type:varchar(16)"`   // "file" | "url"
    SourceURL       string    `gorm:"type:text"`
    Status          string    `gorm:"not null;default:pending;type:varchar(16)"`
    ChunkCount      int       `gorm:"default:0"`
    CharCount       int       `gorm:"default:0"`
    ErrorMessage    string    `gorm:"type:text"`
    CreatedAt       time.Time
    UpdatedAt       time.Time
}

type Chunk struct {
    ID              string         `gorm:"primaryKey;type:varchar(36)"`
    DocumentID      string         `gorm:"not null;index;type:varchar(36)"`
    KnowledgeBaseID string         `gorm:"not null;index;type:varchar(36)"`
    ChunkIndex      int            `gorm:"not null"`
    Content         string         `gorm:"not null;type:text"`
    Metadata        datatypes.JSONMap `gorm:"type:jsonb;default:'{}'"`
    CreatedAt       time.Time
}
```

关键设计：**Chunk 模型不含向量列**。向量由 retriever 层各自管理（pgvector 自己建表 + 索引，Qdrant 不碰 PG），store 层永远干净。

### 3.2 pgvector 表结构（retriever 层私有）

```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS chunk_vectors (
    chunk_id   VARCHAR(36) PRIMARY KEY REFERENCES chunks(id) ON DELETE CASCADE,
    kb_id      VARCHAR(36) NOT NULL,
    embedding  halfvec(1536),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chunk_vectors_kb ON chunk_vectors(kb_id);
CREATE INDEX IF NOT EXISTS idx_chunk_vectors_embedding ON chunk_vectors
    USING hnsw (embedding halfvec_cosine_ops)
    WITH (m = 16, ef_construction = 200);
```

## 4. 核心接口

### 4.1 可插拔接口（factory 模式，对标 provider/）

```go
// Parser: 文档 → 纯文本
type Parser interface {
    Parse(ctx context.Context, reader io.Reader, filename string) (string, error)
    SupportedExtensions() []string
}

// Chunker: 文本 → []Chunk
type ChunkType struct {
    Index    int
    Content  string
    Metadata map[string]string
}
type Chunker interface {
    Split(ctx context.Context, text string) ([]ChunkType, error)
}

// Embedder: 文本 → 向量
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dimensions() int
}

// Retriever: 向量存储 + 检索
type SearchResult struct {
    ChunkID      string
    Content      string
    Score        float64
    DocumentID   string
    DocumentName string
    Metadata     map[string]string
}
type Retriever interface {
    Insert(ctx context.Context, kbID string, chunks []ChunkType, vectors [][]float32) error
    DeleteByKB(ctx context.Context, kbID string) error
    DeleteByDocument(ctx context.Context, docID string) error
    Search(ctx context.Context, kbIDs []string, vector []float32, topK int) ([]SearchResult, error)
}
```

### 4.2 Service（Manager 侧入口）

```go
type Service struct {
    store     *store.KnowledgeStore
    retriever Retriever
    embedder  Embedder
    chunker   Chunker
    parser    *parser.Registry
}

func (s *Service) CreateKB(ctx, voicebotID, name, desc, model) (*store.KnowledgeBase, error)
func (s *Service) DeleteKB(ctx, kbID) error
func (s *Service) IngestDocument(ctx, kbID, reader, filename, source) (*store.Document, error)
func (s *Service) DeleteDocument(ctx, docID) error
func (s *Service) Search(ctx, kbIDs[], query, topK) ([]retriever.SearchResult, error)
func (s *Service) ListKBs(ctx, voicebotID) ([]store.KnowledgeBase, error)
func (s *Service) ListDocuments(ctx, kbID) ([]store.Document, error)
```

### 4.3 SearchClient（wsserver 侧）

```go
type SearchClient struct {
    managerURL string
    deviceID   string
    httpClient *http.Client
}

func NewSearchClient(managerURL, deviceID string) *SearchClient
func (c *SearchClient) Search(ctx, query, topK) ([]SearchResultItem, error)
```

## 5. 入库管线

异步处理，前端轮询 status：

```
POST /api/data/knowledge/:kb_id/documents
  │  创建 Document(status=pending)，返回 doc_id
  │
  ▼  goroutine 异步
  ┌─────────────────────────────────────────────────┐
  │ 1. status="parsing"                             │
  │    parser.Parse(reader, filename) → raw text    │
  │                                                 │
  │ 2. status="chunking"                            │
  │    chunker.Split(text) → []Chunk                │
  │    (recursive, chunk_size=512, overlap=80)      │
  │                                                 │
  │ 3. status="embedding"                           │
  │    embedder.Embed(chunks) → [][]float32         │
  │                                                 │
  │ 4. status="storing"                             │
  │    retriever.Insert(kbID, chunks, vectors)      │
  │    INSERT INTO chunks (id, doc_id, kb_id, ...)  │
  │                                                 │
  │ 5. status="ready"                               │
  │    UPDATE documents SET chunk_count, char_count │
  │                                                 │
  │ 出错 → status="error", error_message=errMsg     │
  └─────────────────────────────────────────────────┘
```

## 6. 检索流程

```
knowledge_search(q="如何配置", top_k=5)
  │
  ▼
  1. embedder.Embed([q]) → queryVector
  2. retriever.Search(kbIDs, queryVector, topK)
     → SELECT chunk_id, content, score
       FROM chunk_vectors cv
       JOIN chunks c ON c.id = cv.chunk_id
       WHERE cv.kb_id IN (...)
       ORDER BY embedding <=> queryVector
       LIMIT topK
  3. 返回 [{chunk_id, content, score, document_name}]
```

## 7. HTTP API

### 7.1 管理端（Manager，Web UI 调用）

| 方法 | 路径 | 说明 |
| ------ | ------ | ------ |
| GET | `/api/data/knowledge/bots/:bot_id/knowledge_bases` | 列出智能体的知识库 |
| POST | `/api/data/knowledge/bots/:bot_id/knowledge_bases` | 创建知识库 |
| GET | `/api/data/knowledge/knowledge_bases/:kb_id` | 查看知识库详情 |
| DELETE | `/api/data/knowledge/knowledge_bases/:kb_id` | 删除知识库 |
| GET | `/api/data/knowledge/knowledge_bases/:kb_id/documents` | 列出知识库文档 |
| POST | `/api/data/knowledge/knowledge_bases/:kb_id/documents` | 上传文档（multipart/form-data） |
| DELETE | `/api/data/knowledge/documents/:doc_id` | 删除文档 |
| GET | `/api/data/knowledge/documents/:doc_id/status` | 查询入库进度 |

### 7.2 内部检索（wsserver 调用）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/internal/knowledge/search?q=...&device_id=...&top_k=5` | 向量检索 |

检索端点内部流程：device_id → voicebot_id → 该智能体所有知识库 → retriever.Search(kbIDs, queryVector, topK)

### 7.3 权限校验

- 管理端点：通过 JWT userID + voicebot ownership 校验
- 检索端点：通过 device_id 定位归属，内部 API 不暴露 userID

## 8. 分块策略

默认使用递归字符分割（RecursiveCharacterTextSplitter）：

| 参数 | 默认值 | 说明 |
| ------ | -------- | ------ |
| chunk_size | 512 | 字符数 |
| chunk_overlap | 80（~15%） | 相邻 chunk 重叠字符数 |
| separators | `["\n\n", "\n", "。", ".", " ", ""]` | 分隔符优先级 |

策略：从大分隔符开始尝试切分，如果子块仍大于 chunk_size，递归尝试更小分隔符，兜底按字符硬切。

## 9. Agent 工具注册

三个工具的分工（注入到 Agent tool loop）：

| 工具 | 操作 | 成本 | 说明 |
| ------ | ------ | ------ | ------ |
| `memory` | 读写 | 0 | Agent 的笔记，全量注入 System Prompt |
| `session_search` | 只读 | 0 | 按需回忆历史对话，HTTP FTS |
| `knowledge_search` | 只读 | 1 次 embedding | 检索文档资料，HTTP 向量检索 |

## 10. 风险与后续迭代

- **pgvector HNSW 索引构建时间**：大量 chunk 时入库可能较慢（ef_construction=200），可后续调优
- **Embedding API 成本**：每次检索调用 1 次 embedding，可由用户配置是否启用知识库
- **URL 抓取**：先支持静态页面爬取，后续可接入浏览器渲染
- **未来扩展**：Qdrant 实现（注册 `retriever/qdrant.go`）、Rerank 重排序、混合检索（向量 + BM25）
