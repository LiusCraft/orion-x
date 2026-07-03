package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/store"
)

type VoicebotHandler struct {
	voicebots *store.VoicebotStore
}

func NewVoicebotHandler(voicebots *store.VoicebotStore) *VoicebotHandler {
	return &VoicebotHandler{voicebots: voicebots}
}

// GET /api/voicebots
func (h *VoicebotHandler) List(c *gin.Context) {
	list, err := h.voicebots.ListByOwner(middleware.UserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

type createVoicebotRequest struct {
	Name       string          `json:"name" binding:"required"`
	ConfigJSON json.RawMessage `json:"config_json"`
}

// POST /api/voicebots
func (h *VoicebotHandler) Create(c *gin.Context) {
	var req createVoicebotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cfgJSON := string(req.ConfigJSON)
	if cfgJSON == "" || cfgJSON == "null" {
		// 默认配置
		b, _ := json.Marshal(config.DefaultConfig())
		cfgJSON = string(b)
	}

	userID := middleware.UserID(c)
	v, err := h.voicebots.Create(req.Name, userID, cfgJSON, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, v)
}

// GET /api/voicebots/:id
func (h *VoicebotHandler) Get(c *gin.Context) {
	v, err := h.voicebots.GetByID(c.Param("id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if v.OwnerID != middleware.UserID(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	c.JSON(http.StatusOK, v)
}

type updateVoicebotRequest struct {
	Name       string          `json:"name" binding:"required"`
	ConfigJSON json.RawMessage `json:"config_json" binding:"required"`
}

// PUT /api/voicebots/:id
func (h *VoicebotHandler) Update(c *gin.Context) {
	v, err := h.voicebots.GetByID(c.Param("id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if v.OwnerID != middleware.UserID(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	var req updateVoicebotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 校验 JSON 格式合法
	var cfg config.AppConfig
	if err := json.Unmarshal(req.ConfigJSON, &cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config_json: " + err.Error()})
		return
	}

	updated, err := h.voicebots.Update(v.ID, req.Name, string(req.ConfigJSON))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

// DELETE /api/voicebots/:id
func (h *VoicebotHandler) Delete(c *gin.Context) {
	v, err := h.voicebots.GetByID(c.Param("id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if v.OwnerID != middleware.UserID(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	if err := h.voicebots.Delete(v.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
