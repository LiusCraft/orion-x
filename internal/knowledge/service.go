package knowledge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/knowledge/chunker"
	"github.com/liuscraft/orion-x/internal/knowledge/embedder"
	"github.com/liuscraft/orion-x/internal/knowledge/parser"
	"github.com/liuscraft/orion-x/internal/knowledge/retriever"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

// Service orchestrates document ingestion, retrieval, and knowledge base CRUD.
type Service struct {
	kbStore    *store.KnowledgeBaseStore
	docStore   *store.DocumentStore
	modelStore *store.AIModelStore
	parserReg  *parser.Registry
	chunker    chunker.Chunker
	retriever  retriever.Retriever

	mu       sync.Mutex
	embCache map[string]embedder.Embedder // keyed by model ID
}

// NewService creates a knowledge Service.
func NewService(
	kbStore *store.KnowledgeBaseStore,
	docStore *store.DocumentStore,
	modelStore *store.AIModelStore,
	ret retriever.Retriever,
) *Service {
	return &Service{
		kbStore:    kbStore,
		docStore:   docStore,
		modelStore: modelStore,
		parserReg:  parser.DefaultRegistry(),
		chunker:    chunker.NewRecursive(chunker.RecursiveConfig{}),
		retriever:  ret,
		embCache:   make(map[string]embedder.Embedder),
	}
}

// getEmbedder resolves an embedder for the given AIModel ID.
func (s *Service) getEmbedder(ctx context.Context, modelID string) (embedder.Embedder, error) {
	if modelID == "" {
		return nil, fmt.Errorf("未配置向量模型，请先在知识库设置中选择一个 Embedding 模型")
	}

	s.mu.Lock()
	emb, ok := s.embCache[modelID]
	s.mu.Unlock()
	if ok {
		return emb, nil
	}

	model, err := s.modelStore.GetByID(modelID)
	if err != nil {
		return nil, fmt.Errorf("向量模型 %s 不存在", modelID)
	}
	if model.Type != store.ModelTypeEmbedding {
		return nil, fmt.Errorf("模型 %s 不是 embedding 类型（当前类型: %s）", model.Name, model.Type)
	}
	if model.Provider == nil || model.Provider.APIKeyEnc == "" {
		return nil, fmt.Errorf("向量模型 %s 的厂商未配置 API Key", model.Name)
	}

	baseURL := model.BaseURL
	if baseURL == "" {
		baseURL = model.Provider.BaseURL
	}

	emb, err = embedder.New(embedder.Config{
		Type:    "openai",
		APIKey:  model.Provider.APIKeyEnc,
		BaseURL: baseURL,
		Model:   model.ModelID,
	})
	if err != nil {
		return nil, fmt.Errorf("创建向量模型 %s: %w", model.Name, err)
	}

	s.mu.Lock()
	s.embCache[modelID] = emb
	s.mu.Unlock()
	return emb, nil
}

// ── KnowledgeBase CRUD ──

// CreateKB creates a new knowledge base for a voicebot.
func (s *Service) CreateKB(ctx context.Context, voicebotID, name, desc, embeddingModelID string) (*store.KnowledgeBase, error) {
	if embeddingModelID == "" {
		return nil, fmt.Errorf("必须选择向量模型")
	}

	// Validate the model exists and is type=embedding
	emb, err := s.getEmbedder(ctx, embeddingModelID)
	if err != nil {
		return nil, fmt.Errorf("向量模型无效: %w", err)
	}

	kb := &store.KnowledgeBase{
		VoicebotID:       voicebotID,
		Name:             name,
		Description:      desc,
		EmbeddingModelID: embeddingModelID,
		EmbeddingDim:     emb.Dimensions(),
	}
	if err := s.kbStore.Create(kb); err != nil {
		return nil, fmt.Errorf("create kb: %w", err)
	}
	return kb, nil
}

// DeleteKB removes a knowledge base and all associated documents, chunks, and vectors.
func (s *Service) DeleteKB(ctx context.Context, kbID string) error {
	if err := s.retriever.DeleteByKB(ctx, kbID); err != nil {
		logging.Warnf("Knowledge: delete vectors for kb %s: %v", kbID, err)
	}
	return s.kbStore.DeleteByID(kbID)
}

// ListKBs returns all knowledge bases for a voicebot.
func (s *Service) ListKBs(ctx context.Context, voicebotID string) ([]store.KnowledgeBase, error) {
	return s.kbStore.ListByVoicebot(voicebotID)
}

// GetKB returns a single knowledge base by ID.
func (s *Service) GetKB(ctx context.Context, kbID string) (*store.KnowledgeBase, error) {
	return s.kbStore.GetByID(kbID)
}

// ── Document CRUD ──

// ListDocuments returns all documents in a knowledge base.
func (s *Service) ListDocuments(ctx context.Context, kbID string) ([]store.Document, error) {
	return s.docStore.ListByKB(kbID)
}

// IngestDocument starts asynchronous ingestion of a file.
func (s *Service) IngestDocument(ctx context.Context, kbID string, reader io.Reader, filename, source string) (*store.Document, error) {
	doc := &store.Document{
		KnowledgeBaseID: kbID,
		Name:            filename,
		Source:          source,
	}
	if err := s.docStore.Create(doc); err != nil {
		return nil, fmt.Errorf("create document: %w", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		s.failDoc(doc.ID, fmt.Sprintf("read input: %v", err))
		return doc, nil
	}
	go s.ingestAsync(doc.ID, kbID, data, filename, source)
	return doc, nil
}

// IngestURL fetches a URL and ingests its content asynchronously.
func (s *Service) IngestURL(ctx context.Context, kbID, urlStr string) (*store.Document, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("ingest url: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch url: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

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

	return s.IngestDocument(ctx, kbID, strings.NewReader(string(body)), name, "url")
}

// DeleteDocument removes a document and its chunks/vectors.
func (s *Service) DeleteDocument(ctx context.Context, docID string) error {
	if err := s.retriever.DeleteByDocument(ctx, docID); err != nil {
		logging.Warnf("Knowledge: delete vectors for doc %s: %v", docID, err)
	}
	return s.docStore.DeleteByID(docID)
}

// GetDocumentStatus returns the current ingestion status of a document.
func (s *Service) GetDocumentStatus(ctx context.Context, docID string) (*store.Document, error) {
	return s.docStore.GetByID(docID)
}

// RetryDocument resets a failed document and re-triggers ingestion.
// For file-type documents, the original content must be re-uploaded.
// For url-type documents, it re-fetches the URL.
func (s *Service) RetryDocument(ctx context.Context, docID string) error {
	doc, err := s.docStore.GetByID(docID)
	if err != nil {
		return fmt.Errorf("document not found: %w", err)
	}
	if doc.Status != "error" {
		return fmt.Errorf("only failed documents can be retried, current status: %s", doc.Status)
	}

	// Clear old chunks/vectors
	if err := s.retriever.DeleteByDocument(ctx, docID); err != nil {
		logging.Warnf("Knowledge: retry delete old vectors for doc %s: %v", docID, err)
	}

	if doc.Source == "url" && doc.SourceURL != "" {
		// Re-fetch URL
		if err := s.docStore.UpdateStatus(docID, "pending", ""); err != nil {
			return err
		}
		go s.ingestURLAsync(docID, doc.KnowledgeBaseID, doc.SourceURL)
	} else {
		return fmt.Errorf("文件类型文档请重新上传，URL 类型文档支持自动重试")
	}
	return nil
}

func (s *Service) ingestURLAsync(docID, kbID, urlStr string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		s.failDoc(docID, fmt.Sprintf("fetch url: %v", err))
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.failDoc(docID, fmt.Sprintf("fetch url: %v", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.failDoc(docID, fmt.Sprintf("read url body: %v", err))
		return
	}

	name := urlStr
	if idx := strings.LastIndex(urlStr, "/"); idx >= 0 {
		name = urlStr[idx+1:]
	}

	s.ingestAsync(docID, kbID, body, name, "url")
}

// ── Search ──

// Search performs vector similarity search across the given knowledge bases.
// It resolves the embedder from each KB's configured model.
func (s *Service) Search(ctx context.Context, kbIDs []string, query string, topK int) ([]retriever.SearchResult, error) {
	if topK <= 0 || topK > 10 {
		topK = 5
	}
	if len(kbIDs) == 0 {
		return nil, nil
	}

	// Resolve embedder from the first KB's model (all KBs for a voicebot share the same model)
	kb, err := s.kbStore.GetByID(kbIDs[0])
	if err != nil {
		return nil, fmt.Errorf("get kb %s: %w", kbIDs[0], err)
	}
	emb, err := s.getEmbedder(ctx, kb.EmbeddingModelID)
	if err != nil {
		return nil, err
	}

	vectors, err := emb.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) == 0 {
		return nil, nil
	}
	return s.retriever.Search(ctx, kbIDs, vectors[0], topK)
}

// ── Internal: async ingestion pipeline ──

func (s *Service) ingestAsync(docID, kbID string, data []byte, filename, source string) {
	defer func() {
		if rec := recover(); rec != nil {
			s.failDoc(docID, fmt.Sprintf("panic: %v", rec))
		}
	}()

	ctx := context.Background()

	setStatus := func(status string) {
		if err := s.docStore.UpdateStatus(docID, status, ""); err != nil {
			logging.Errorf("Knowledge[%s]: update status to %s: %v", docID, status, err)
		}
	}

	// Resolve embedder from KB
	kb, err := s.kbStore.GetByID(kbID)
	if err != nil {
		s.failDoc(docID, fmt.Sprintf("get kb: %v", err))
		return
	}
	emb, err := s.getEmbedder(ctx, kb.EmbeddingModelID)
	if err != nil {
		s.failDoc(docID, fmt.Sprintf("embedder: %v", err))
		return
	}

	// 1. Parse
	setStatus("parsing")
	var text string
	p, ok := s.parserReg.Find(filename)
	if !ok {
		text = strings.TrimSpace(string(data))
	} else {
		var err error
		text, err = p.Parse(ctx, strings.NewReader(string(data)), filename)
		if err != nil {
			s.failDoc(docID, fmt.Sprintf("parse: %v", err))
			return
		}
	}
	if text == "" {
		s.failDoc(docID, "empty content after parsing")
		return
	}

	// 2. Chunk
	setStatus("chunking")
	chunks, err := s.chunker.Split(ctx, text)
	if err != nil {
		s.failDoc(docID, fmt.Sprintf("chunk: %v", err))
		return
	}
	if len(chunks) == 0 {
		s.failDoc(docID, "no chunks produced")
		return
	}

	// 3. Embed (batch by 10 for DashScope compatibility)
	setStatus("embedding")
	contents := make([]string, len(chunks))
	for i, c := range chunks {
		contents[i] = c.Content
	}
	const batchSize = 10
	var vectors [][]float32
	for i := 0; i < len(contents); i += batchSize {
		end := i + batchSize
		if end > len(contents) {
			end = len(contents)
		}
		batch, err := emb.Embed(ctx, contents[i:end])
		if err != nil {
			s.failDoc(docID, fmt.Sprintf("embed: %v", err))
			return
		}
		vectors = append(vectors, batch...)
	}

	// 4. Store
	setStatus("storing")
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

	// 5. Done
	charCount := len([]rune(text))
	if err := s.docStore.UpdateChunkInfo(docID, len(chunks), charCount); err != nil {
		logging.Errorf("Knowledge[%s]: update chunk info: %v", docID, err)
	}
	setStatus("ready")
	logging.Infof("Knowledge[%s]: ingested %d chunks, %d chars", docID, len(chunks), charCount)
}

func (s *Service) failDoc(docID, errMsg string) {
	if err := s.docStore.UpdateStatus(docID, "error", errMsg); err != nil {
		logging.Errorf("Knowledge[%s]: fail doc: %v", docID, err)
	}
	logging.Errorf("Knowledge[%s]: %s", docID, errMsg)
}
