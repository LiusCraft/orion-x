package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/store"
)

type DeviceHandler struct {
	voicebots *store.VoicebotStore
	devices   *store.DeviceStore
}

type telegramChannelStatus struct {
	Enabled   bool   `json:"enabled"`
	TokenHint string `json:"token_hint,omitempty"`
}

type deviceResponse struct {
	ID         string                `json:"id"`
	VoicebotID string                `json:"voicebot_id"`
	Name       string                `json:"name"`
	CreatedAt  time.Time             `json:"created_at"`
	UpdatedAt  time.Time             `json:"updated_at"`
	Creator    string                `json:"creator"`
	Telegram   telegramChannelStatus `json:"telegram"`
}

func maskTelegramToken(token string) string {
	if len(token) <= 8 {
		return "********"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

func newDeviceResponse(d *store.Device) deviceResponse {
	telegram := telegramChannelStatus{Enabled: d.TgBotToken != ""}
	if telegram.Enabled {
		telegram.TokenHint = maskTelegramToken(d.TgBotToken)
	}
	return deviceResponse{
		ID: d.ID, VoicebotID: d.VoicebotID, Name: d.Name,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, Creator: d.Creator, Telegram: telegram,
	}
}

func newDeviceResponses(devices []store.Device) []deviceResponse {
	responses := make([]deviceResponse, 0, len(devices))
	for i := range devices {
		responses = append(responses, newDeviceResponse(&devices[i]))
	}
	return responses
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
	c.JSON(http.StatusOK, newDeviceResponses(list))
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
	c.JSON(http.StatusCreated, newDeviceResponse(d))
}

func (h *DeviceHandler) deviceForVoicebot(c *gin.Context) (*store.Device, bool) {
	if _, ok := h.ownerVoicebot(c); !ok {
		return nil, false
	}
	d, err := h.devices.GetByID(c.Param("did"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return nil, false
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, false
	}
	if d.VoicebotID != c.Param("id") {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return nil, false
	}
	return d, true
}

type setTelegramChannelRequest struct {
	BotToken string `json:"bot_token" binding:"required"`
}

// SetTelegramChannel PUT /api/voicebots/:id/devices/:did/channels/telegram
func (h *DeviceHandler) SetTelegramChannel(c *gin.Context) {
	d, ok := h.deviceForVoicebot(c)
	if !ok {
		return
	}
	var req setTelegramChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bot_token is required"})
		return
	}
	token := strings.TrimSpace(req.BotToken)
	if token == "" || len(token) > 256 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Telegram bot token"})
		return
	}
	if err := h.devices.SetTgBotToken(d.ID, token); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	d.TgBotToken = token
	c.JSON(http.StatusOK, newDeviceResponse(d))
}

// DeleteTelegramChannel DELETE /api/voicebots/:id/devices/:did/channels/telegram
func (h *DeviceHandler) DeleteTelegramChannel(c *gin.Context) {
	d, ok := h.deviceForVoicebot(c)
	if !ok {
		return
	}
	if err := h.devices.SetTgBotToken(d.ID, ""); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	d.TgBotToken = ""
	c.JSON(http.StatusOK, newDeviceResponse(d))
}

// DELETE /api/voicebots/:id/devices/:did
func (h *DeviceHandler) Delete(c *gin.Context) {
	d, ok := h.deviceForVoicebot(c)
	if !ok {
		return
	}
	if err := h.devices.Delete(d.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
