package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/assets"
	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/logging"
	ttsprovider "github.com/liuscraft/orion-x/internal/provider/tts"
	"github.com/liuscraft/orion-x/internal/store"
)

type VoiceHandler struct {
	voices       *store.ModelVoiceStore
	cloneService voiceCloneService
	assets       *assets.Service
}

// NewVoiceHandler 构造 VoiceHandler；assetSvc 为 nil 表示未配置对象存储，
// 此时用 source_asset_id 的复刻请求会返回 503。meter 为 nil 表示计费关闭（复刻
// 音色不计费）。
func NewVoiceHandler(voices *store.ModelVoiceStore, models *store.AIModelStore, assetSvc *assets.Service, meter billing.Meter) *VoiceHandler {
	// 避免把 nil 的 *assets.Service 装进接口（typed nil 不等于 nil）。
	var cloneAssets voiceAssetStore
	if assetSvc != nil {
		cloneAssets = assetSvc
	}
	return &VoiceHandler{
		voices:       voices,
		cloneService: newProviderVoiceCloneService(models, voices, cloneAssets, ttsprovider.NewProvider, meter),
		assets:       assetSvc,
	}
}

// GET /api/models/:id/voices?lang=zh
func (h *VoiceHandler) List(c *gin.Context) {
	list, err := h.voices.List(c.Param("id"), middleware.UserID(c), c.Query("lang"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// GET /api/voices/system — 获取所有系统内置音色（音色广场）
func (h *VoiceHandler) ListSystem(c *gin.Context) {
	list, err := h.voices.ListAllSystem(c.Query("lang"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// GET /api/voices/mine — 当前用户自建（含复刻）的音色，跨模型汇总
func (h *VoiceHandler) ListMine(c *gin.Context) {
	list, err := h.voices.ListByCreator(middleware.UserID(c), c.Query("lang"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 复刻音色用参考音频当试听：预签名 URL 只在响应时现算，不入库。
	var previews map[string]string
	if h.assets != nil {
		previews = voicePreviewURLs(c.Request.Context(), h.assets, middleware.UserID(c), list)
	}
	c.JSON(http.StatusOK, withPreviews(list, previews))
}

// GET /api/models/:id/voices/:vid
func (h *VoiceHandler) Get(c *gin.Context) {
	v, err := h.voices.GetByID(c.Param("vid"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, v)
}

type createVoiceRequest struct {
	VoiceID     string            `json:"voice_id" binding:"required"`
	Name        string            `json:"name" binding:"required"`
	Description string            `json:"description"`
	AvatarURL   string            `json:"avatar_url"`
	PreviewURL  string            `json:"preview_url"`
	Tags        pq.StringArray    `json:"tags"`
	Langs       pq.StringArray    `json:"langs"`
	Extra       datatypes.JSONMap `json:"extra"`
}

// POST /api/models/:id/voices
func (h *VoiceHandler) Create(c *gin.Context) {
	var req createVoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	v, err := h.voices.Create(store.CreateVoiceParams{
		ModelID:     c.Param("id"),
		VoiceID:     req.VoiceID,
		Name:        req.Name,
		Description: req.Description,
		AvatarURL:   req.AvatarURL,
		PreviewURL:  req.PreviewURL,
		Tags:        req.Tags,
		Langs:       req.Langs,
		Extra:       req.Extra,
		Creator:     middleware.UserID(c),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, v)
}

type updateVoiceRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	AvatarURL   string            `json:"avatar_url"`
	PreviewURL  string            `json:"preview_url"`
	Tags        pq.StringArray    `json:"tags"`
	Langs       pq.StringArray    `json:"langs"`
	Extra       datatypes.JSONMap `json:"extra"`
}

// PUT /api/models/:id/voices/:vid
func (h *VoiceHandler) Update(c *gin.Context) {
	v, err := h.voices.GetByID(c.Param("vid"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if v.IsSystem || v.Creator != middleware.UserID(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	var req updateVoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]any{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.AvatarURL != "" {
		updates["avatar_url"] = req.AvatarURL
	}
	if req.PreviewURL != "" {
		updates["preview_url"] = req.PreviewURL
	}
	if req.Tags != nil {
		updates["tags"] = req.Tags
	}
	if req.Langs != nil {
		updates["langs"] = req.Langs
	}
	if req.Extra != nil {
		updates["extra"] = req.Extra
	}

	updated, err := h.voices.Update(v.ID, updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

// DELETE /api/models/:id/voices/:vid
func (h *VoiceHandler) Delete(c *gin.Context) {
	v, err := h.voices.GetByID(c.Param("vid"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if v.Creator != middleware.UserID(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	// 参考音频随音色级联删除：先删资源（对象 + 行）再删音色，避免留下失配数据。
	if v.SourceAssetID != "" && h.assets != nil {
		if err := h.assets.DeleteCascade(c.Request.Context(), v.Creator, v.SourceAssetID, assets.PurposeVoiceSample); err != nil {
			writeVoiceAssetError(c, err)
			return
		}
	} else if v.SourceAssetID != "" {
		logging.Warnf("voice delete: skip sample asset %s cascade: object storage is not configured", v.SourceAssetID)
	}

	if err := h.voices.Delete(v.ID); err != nil {
		if errors.Is(err, store.ErrSystemRecord) {
			c.JSON(http.StatusForbidden, gin.H{"error": "cannot delete system voice"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// writeVoiceAssetError 把资源服务的错误映射为 HTTP 响应。
func writeVoiceAssetError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, assets.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	case errors.Is(err, assets.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "asset not found"})
	case errors.Is(err, assets.ErrStorage):
		logging.Errorf("Voice asset storage error: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "对象存储操作失败，请重试"})
	default:
		logging.Errorf("Voice asset error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
	}
}

// PATCH /internal/voices/:id — 更新系统音色字段
func (h *VoiceHandler) AdminUpdate(c *gin.Context) {
	updates := make(map[string]any)
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.voices.Update(c.Param("id"), updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

type adminCreateVoiceRequest struct {
	ModelID     string            `json:"model_id" binding:"required"`
	VoiceID     string            `json:"voice_id" binding:"required"`
	Name        string            `json:"name" binding:"required"`
	Description string            `json:"description"`
	Gender      store.VoiceGender `json:"gender"`
	AvatarURL   string            `json:"avatar_url"`
	PreviewURL  string            `json:"preview_url"`
	Tags        pq.StringArray    `json:"tags"`
	Langs       pq.StringArray    `json:"langs"`
	Emotions    datatypes.JSONMap `json:"emotions"`
	Extra       datatypes.JSONMap `json:"extra"`
}

// POST /internal/voices  — 添加系统内置音色，不走 JWT
func (h *VoiceHandler) AdminCreate(c *gin.Context) {
	var req adminCreateVoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	v, err := h.voices.CreateSystem(store.CreateVoiceParams{
		ModelID:     req.ModelID,
		VoiceID:     req.VoiceID,
		Name:        req.Name,
		Description: req.Description,
		Gender:      req.Gender,
		AvatarURL:   req.AvatarURL,
		PreviewURL:  req.PreviewURL,
		Tags:        req.Tags,
		Langs:       req.Langs,
		Emotions:    req.Emotions,
		Extra:       req.Extra,
		Creator:     "system",
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, v)
}
