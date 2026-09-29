package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/channels/platform"
	"github.com/liuscraft/orion-x/internal/store"
)

// ChannelHandler 是通道平台的通用配置面：列平台与配置 Schema、按 Schema 配置/解绑。
// 新增平台只需要在 internal/channels/platform 加一条 descriptor，这里不用改。
type ChannelHandler struct {
	voicebots *store.VoicebotStore
	devices   *store.DeviceStore
	channels  *store.DeviceChannelStore
}

type deviceChannelStatus struct {
	Platform    string            `json:"platform"`
	DisplayName string            `json:"display_name"`
	Enabled     bool              `json:"enabled"`
	Config      map[string]string `json:"config,omitempty"` // 敏感字段已掩码
}

// NewChannelHandler 创建通道配置 handler。
func NewChannelHandler(voicebots *store.VoicebotStore, devices *store.DeviceStore, channels *store.DeviceChannelStore) *ChannelHandler {
	return &ChannelHandler{voicebots: voicebots, devices: devices, channels: channels}
}

// ListPlatforms GET /api/channels — 返回所有平台及其配置字段 Schema，前端据此渲染表单。
func (h *ChannelHandler) ListPlatforms(c *gin.Context) {
	c.JSON(http.StatusOK, platform.All())
}

type setChannelRequest struct {
	Config map[string]string `json:"config" binding:"required"`
}

// SetChannel PUT /api/voicebots/:id/devices/:did/channels/:platform
// 按平台 Schema 校验后覆盖写入；同值重复调用结果一致（幂等）。
func (h *ChannelHandler) SetChannel(c *gin.Context) {
	d, ok := deviceForVoicebot(c, h.voicebots, h.devices)
	if !ok {
		return
	}
	desc, ok := platform.Get(c.Param("platform"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown platform"})
		return
	}
	var req setChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "config is required"})
		return
	}
	config := normalizeConfig(req.Config)
	if err := desc.Validate(config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.channels.Upsert(d.ID, desc.Name, config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, deviceChannelStatus{
		Platform:    desc.Name,
		DisplayName: desc.DisplayName,
		Enabled:     true,
		Config:      desc.Mask(config),
	})
}

// DeleteChannel DELETE /api/voicebots/:id/devices/:did/channels/:platform
// 幂等：未配置的平台也返回 200。
func (h *ChannelHandler) DeleteChannel(c *gin.Context) {
	d, ok := deviceForVoicebot(c, h.voicebots, h.devices)
	if !ok {
		return
	}
	desc, ok := platform.Get(c.Param("platform"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown platform"})
		return
	}
	if err := h.channels.Delete(d.ID, desc.Name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, deviceChannelStatus{Platform: desc.Name, DisplayName: desc.DisplayName})
}

// newDeviceChannelStatuses 把设备的通道行转成回显状态；平台已下线的孤儿行忽略。
func newDeviceChannelStatuses(rows []store.DeviceChannel) []deviceChannelStatus {
	statuses := make([]deviceChannelStatus, 0, len(rows))
	for _, row := range rows {
		desc, ok := platform.Get(row.Platform)
		if !ok {
			continue
		}
		statuses = append(statuses, deviceChannelStatus{
			Platform:    row.Platform,
			DisplayName: desc.DisplayName,
			Enabled:     true,
			Config:      desc.Mask(row.ConfigStrings()),
		})
	}
	return statuses
}

// normalizeConfig 去空白并丢掉空值，避免把 " " 当成已填写。
func normalizeConfig(config map[string]string) map[string]string {
	out := make(map[string]string, len(config))
	for key, value := range config {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[key] = value
	}
	return out
}
