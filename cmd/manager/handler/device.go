package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/store"
)

type DeviceHandler struct {
	voicebots *store.VoicebotStore
	devices   *store.DeviceStore
	channels  *store.DeviceChannelStore
}

type deviceResponse struct {
	ID         string                `json:"id"`
	VoicebotID string                `json:"voicebot_id"`
	Name       string                `json:"name"`
	CreatedAt  time.Time             `json:"created_at"`
	UpdatedAt  time.Time             `json:"updated_at"`
	Creator    string                `json:"creator"`
	Channels   []deviceChannelStatus `json:"channels"`
}

func newDeviceResponse(d *store.Device, channelRows []store.DeviceChannel) deviceResponse {
	return deviceResponse{
		ID: d.ID, VoicebotID: d.VoicebotID, Name: d.Name,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, Creator: d.Creator,
		Channels: newDeviceChannelStatuses(channelRows),
	}
}

func NewDeviceHandler(voicebots *store.VoicebotStore, devices *store.DeviceStore, channels *store.DeviceChannelStore) *DeviceHandler {
	return &DeviceHandler{voicebots: voicebots, devices: devices, channels: channels}
}

// ownerVoicebot 校验当前用户是否拥有该 voicebot。
func ownerVoicebot(c *gin.Context, voicebots *store.VoicebotStore) (*store.Voicebot, bool) {
	v, err := voicebots.GetByID(c.Param("id"))
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

// deviceForVoicebot 取路径上的设备并校验它属于该 voicebot 的 owner。
func deviceForVoicebot(c *gin.Context, voicebots *store.VoicebotStore, devices *store.DeviceStore) (*store.Device, bool) {
	if _, ok := ownerVoicebot(c, voicebots); !ok {
		return nil, false
	}
	d, err := devices.GetByID(c.Param("did"))
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

// GET /api/voicebots/:id/devices
func (h *DeviceHandler) List(c *gin.Context) {
	if _, ok := ownerVoicebot(c, h.voicebots); !ok {
		return
	}
	list, err := h.devices.ListByVoicebot(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ids := make([]string, 0, len(list))
	for _, d := range list {
		ids = append(ids, d.ID)
	}
	grouped, err := h.channels.ListByDevices(ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	responses := make([]deviceResponse, 0, len(list))
	for i := range list {
		responses = append(responses, newDeviceResponse(&list[i], grouped[list[i].ID]))
	}
	c.JSON(http.StatusOK, responses)
}

type createDeviceRequest struct {
	ID   string `json:"id" binding:"required"`
	Name string `json:"name"`
}

// POST /api/voicebots/:id/devices
func (h *DeviceHandler) Create(c *gin.Context) {
	v, ok := ownerVoicebot(c, h.voicebots)
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
	c.JSON(http.StatusCreated, newDeviceResponse(d, nil))
}

// DELETE /api/voicebots/:id/devices/:did
func (h *DeviceHandler) Delete(c *gin.Context) {
	d, ok := deviceForVoicebot(c, h.voicebots, h.devices)
	if !ok {
		return
	}
	// 设备删除连带清掉它在各平台的通道配置（DeviceStore.Delete 内做）。
	if err := h.devices.Delete(d.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
