package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/store"
)

type VoiceHandler struct {
	voices *store.ModelVoiceStore
}

func NewVoiceHandler(voices *store.ModelVoiceStore) *VoiceHandler {
	return &VoiceHandler{voices: voices}
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

// PATCH /internal/voices/:id — 更新系统音色字段
func (h *VoiceHandler) AdminUpdate(c *gin.Context) {
	var updates map[string]any
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

type cloneVoiceRequest struct {
	VoiceID        string         `json:"voice_id" binding:"required"` // 厂商返回的复刻音色 ID
	Name           string         `json:"name" binding:"required"`
	SourceAudioURL string         `json:"source_audio_url" binding:"required"`
	Langs          pq.StringArray `json:"langs"`
}

// POST /api/models/:id/voices/clone
func (h *VoiceHandler) Clone(c *gin.Context) {
	var req cloneVoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	v, err := h.voices.CreateCloned(store.CloneVoiceParams{
		ModelID:        c.Param("id"),
		VoiceID:        req.VoiceID,
		Name:           req.Name,
		SourceAudioURL: req.SourceAudioURL,
		Langs:          req.Langs,
		Creator:        middleware.UserID(c),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, v)
}
