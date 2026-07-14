// Package knowledge provides the document knowledge base service.
// It orchestrates document ingestion (parse → chunk → embed → store) and retrieval.
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

// Service orchestrates document ingestion, retrieval, and knowledge base CRUD.
type Service struct {
	kbStore   *store.KnowledgeBaseStore
	docStore  *store.DocumentStore
	parserReg *parser.Registry
	chunker   chunker.Chunker
	embedder  embedder.Embedder
	retriever retriever.Retriever
}

// NewService creates a knowledge Service.
func NewService(
	kbStore *store.KnowledgeBaseStore,
	docStore *store.DocumentStore,
	ret retriever.Retriever,
	emb embedder.Embedder,
) *Service {
	return &Service{
		kbStore:   kbStore,
		docStore:  docStore,
		parserReg: parser.DefaultRegistry(),
		chunker:   chunker.NewRecursive(chunker.RecursiveConfig{}),
		embedder:  emb,
		retriever: ret,
	}
}

// ── KnowledgeBase CRUD ──

// CreateKB creates a new knowledge base for a voicebot.
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
// It creates a pending Document and returns immediately; the caller can poll
// GetDocumentStatus for completion.
func (s *Service) IngestDocument(ctx context.Context, kbID string, reader io.Reader, filename, source string) (*store.Document, error) {
	doc := &store.Document{
		KnowledgeBaseID: kbID,
		Name:            filename,
		Source:          source,
	}
	if err := s.docStore.Create(doc); err != nil {
		return nil, fmt.Errorf("create document: %w", err)
	}
	// Read all data now since the caller may close the reader.
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

// ── Search ──

// Search performs vector similarity search across the given knowledge bases.
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

// ── Internal: async ingestion pipeline ──

func (s *Service) ingestAsync(docID, kbID string, data []byte, filename, source string) {
	ctx := context.Background()

	setStatus := func(status string) {
		if err := s.docStore.UpdateStatus(docID, status, ""); err != nil {
			logging.Errorf("Knowledge[%s]: update status to %s: %v", docID, status, err)
		}
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

	// 3. Embed
	setStatus("embedding")
	contents := make([]string, len(chunks))
	for i, c := range chunks {
		contents[i] = c.Content
	}
	vectors, err := s.embedder.Embed(ctx, contents)
	if err != nil {
		s.failDoc(docID, fmt.Sprintf("embed: %v", err))
		return
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
