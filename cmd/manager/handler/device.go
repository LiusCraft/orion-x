package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/store"
)

type DeviceHandler struct {
	voicebots *store.VoicebotStore
	devices   *store.DeviceStore
}

func NewDeviceHandler(voicebots *store.VoicebotStore, devices *store.DeviceStore) *DeviceHandler {
	return &DeviceHandler{voicebots: voicebots, devices: devices}
}

// 校验当前用户是否拥有该 voicebot
func (h *DeviceHandler) ownerVoicebot(c *gin.Context) (*store.Voicebot, bool) {
	v, err := h.voicebots.GetByID(c.Param("id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "voicebot not found"})
		return nil, false
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, false
	}
	if v.OwnerID != middleware.UserID(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return nil, false
	}
	return v, true
}

// GET /api/voicebots/:id/devices
func (h *DeviceHandler) List(c *gin.Context) {
	if _, ok := h.ownerVoicebot(c); !ok {
		return
	}
	list, err := h.devices.ListByVoicebot(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

type createDeviceRequest struct {
	ID   string `json:"id" binding:"required"`
	Name string `json:"name"`
}

// POST /api/voicebots/:id/devices
func (h *DeviceHandler) Create(c *gin.Context) {
	v, ok := h.ownerVoicebot(c)
	if !ok {
		return
	}
	var req createDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 检查 device_id 是否已被注册（跨 voicebot 唯一）
	if _, err := h.devices.GetByID(req.ID); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "device id already registered"})
		return
	}

	userID := middleware.UserID(c)
	d, err := h.devices.Create(req.ID, v.ID, req.Name, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, d)
}

// DELETE /api/voicebots/:id/devices/:did
func (h *DeviceHandler) Delete(c *gin.Context) {
	if _, ok := h.ownerVoicebot(c); !ok {
		return
	}
	d, err := h.devices.GetByID(c.Param("did"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 确认设备确实属于该 voicebot
	if d.VoicebotID != c.Param("id") {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	if err := h.devices.Delete(d.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
