package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/knowledge"
	"github.com/liuscraft/orion-x/internal/knowledge/retriever"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

// DataKnowledgeHandler exposes CRUD endpoints for knowledge bases and documents,
// plus an internal search endpoint for wsserver consumption.
type DataKnowledgeHandler struct {
	kbSvc       *knowledge.Service
	kbStore     *store.KnowledgeBaseStore
	docStore    *store.DocumentStore
	deviceStore *store.DeviceStore
	botStore    *store.VoicebotStore
}

// NewDataKnowledgeHandler creates a DataKnowledgeHandler.
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

// ListKBs GET /api/data/knowledge/bots/:bot_id/knowledge_bases
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
	if kbs == nil {
		kbs = []store.KnowledgeBase{}
	}
	c.JSON(http.StatusOK, gin.H{"knowledge_bases": kbs})
}

// CreateKB POST /api/data/knowledge/bots/:bot_id/knowledge_bases
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

// GetKB GET /api/data/knowledge/knowledge_bases/:kb_id
func (h *DataKnowledgeHandler) GetKB(c *gin.Context) {
	kbID := c.Param("kb_id")
	kb, err := h.kbSvc.GetKB(c.Request.Context(), kbID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "知识库不存在"})
		return
	}
	c.JSON(http.StatusOK, kb)
}

// DeleteKB DELETE /api/data/knowledge/knowledge_bases/:kb_id
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

// ListDocuments GET /api/data/knowledge/knowledge_bases/:kb_id/documents
func (h *DataKnowledgeHandler) ListDocuments(c *gin.Context) {
	kbID := c.Param("kb_id")
	docs, err := h.kbSvc.ListDocuments(c.Request.Context(), kbID)
	if err != nil {
		logging.Errorf("DataKnowledge ListDocuments kb=%s: %v", kbID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询文档失败"})
		return
	}
	if docs == nil {
		docs = []store.Document{}
	}
	c.JSON(http.StatusOK, gin.H{"documents": docs})
}

// UploadDocument POST /api/data/knowledge/knowledge_bases/:kb_id/documents
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
	defer func() { _ = f.Close() }()

	doc, err := h.kbSvc.IngestDocument(c.Request.Context(), kbID, f, file.Filename, "file")
	if err != nil {
		logging.Errorf("DataKnowledge UploadDocument: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "上传文档失败"})
		return
	}
	c.JSON(http.StatusCreated, doc)
}

// IngestURL POST /api/data/knowledge/knowledge_bases/:kb_id/documents/url
func (h *DataKnowledgeHandler) IngestURL(c *gin.Context) {
	kbID := c.Param("kb_id")
	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url 不能为空"})
		return
	}

	doc, err := h.kbSvc.IngestURL(c.Request.Context(), kbID, req.URL)
	if err != nil {
		logging.Errorf("DataKnowledge IngestURL: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "导入 URL 失败"})
		return
	}
	c.JSON(http.StatusCreated, doc)
}

// DeleteDocument DELETE /api/data/knowledge/documents/:doc_id
func (h *DataKnowledgeHandler) DeleteDocument(c *gin.Context) {
	docID := c.Param("doc_id")
	if err := h.kbSvc.DeleteDocument(c.Request.Context(), docID); err != nil {
		logging.Errorf("DataKnowledge DeleteDocument id=%s: %v", docID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}
	c.Status(http.StatusNoContent)
}

// GetDocumentStatus GET /api/data/knowledge/documents/:doc_id/status
func (h *DataKnowledgeHandler) GetDocumentStatus(c *gin.Context) {
	docID := c.Param("doc_id")
	doc, err := h.kbSvc.GetDocumentStatus(c.Request.Context(), docID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "文档不存在"})
		return
	}
	c.JSON(http.StatusOK, doc)
}

// ── 内部检索 API（wsserver 调用） ──

// Search GET /internal/knowledge/search?q=...&device_id=...&top_k=5
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

	// Resolve device_id → voicebot_id → knowledge bases
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
	if results == nil {
		results = []retriever.SearchResult{}
	}
	c.JSON(http.StatusOK, results)
}
