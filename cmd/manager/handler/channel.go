package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/channels/platform"
	"github.com/liuscraft/orion-x/internal/channels/qrbind"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

// ChannelStore 是通道配置的读写面；`*store.DeviceChannelStore` 实现它。
// 抽成接口是为了让扫码端点的单测不必连库（仓库里 DeviceOwner 同此做法）。
type ChannelStore interface {
	// Upsert 写入/覆盖设备在某个平台上的配置（幂等）。
	Upsert(deviceID, platformName string, config map[string]string) error
	// Delete 解绑，幂等：不存在也返回 nil。
	Delete(deviceID, platformName string) error
}

// ChannelHandler 是通道平台的通用配置面：列平台与配置 Schema、按 Schema 配置/解绑，
// 以及把支持扫码的平台代理到对应的 qrbind.Binder。
// 新增平台只需要在 internal/channels/platform 加一条 descriptor，这里不用改。
type ChannelHandler struct {
	voicebots *store.VoicebotStore
	devices   *store.DeviceStore
	channels  ChannelStore
	// qrBinders 是「扫码开通」的平台实现，键为 platform.Descriptor.Name；
	// 空 map = 扫码关闭（channels.qr_binding.enabled: false）。
	qrBinders map[string]qrbind.Binder
}

type deviceChannelStatus struct {
	Platform    string            `json:"platform"`
	DisplayName string            `json:"display_name"`
	Enabled     bool              `json:"enabled"`
	Config      map[string]string `json:"config,omitempty"` // 敏感字段已掩码
}

// channelQRSession 是一次扫码开通会话的响应。
type channelQRSession struct {
	SessionID string    `json:"session_id"`
	Platform  string    `json:"platform"`
	Status    string    `json:"status"`
	QRContent string    `json:"qr_content,omitempty"` // 二维码内容（企微 auth_url）
	ExpiresAt time.Time `json:"expires_at"`
	Error     string    `json:"error,omitempty"`
	// Channel 仅在本会话成功并写库后出现，语义同设备响应里的通道项。
	Channel *deviceChannelStatus `json:"channel,omitempty"`
}

// NewChannelHandler 创建通道配置 handler。qrBinders 可以为 nil。
func NewChannelHandler(voicebots *store.VoicebotStore, devices *store.DeviceStore, channels ChannelStore, qrBinders map[string]qrbind.Binder) *ChannelHandler {
	if qrBinders == nil {
		qrBinders = make(map[string]qrbind.Binder)
	}
	return &ChannelHandler{voicebots: voicebots, devices: devices, channels: channels, qrBinders: qrBinders}
}

// ListPlatforms GET /api/channels — 返回所有平台及其配置字段 Schema，前端据此渲染表单。
// 扫码能力是注册表声明 + 运行时装配的交集：没装配 Binder 的平台不显示扫码入口。
func (h *ChannelHandler) ListPlatforms(c *gin.Context) {
	descs := platform.All()
	for i := range descs {
		if descs[i].QRBinding && h.qrBinders[descs[i].Name] == nil {
			descs[i].QRBinding = false
		}
	}
	c.JSON(http.StatusOK, descs)
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

// StartChannelQR POST /api/voicebots/:id/devices/:did/channels/:platform/qr
// 生成二维码并开始一次扫码开通会话；重复调用替换旧会话（旧的二维码随即作废）。
func (h *ChannelHandler) StartChannelQR(c *gin.Context) {
	d, ok := deviceForVoicebot(c, h.voicebots, h.devices)
	if !ok {
		return
	}
	desc, binder, ok := h.qrBinder(c)
	if !ok {
		return
	}
	sess, err := binder.Start(d.ID)
	if err != nil {
		// 出网失败：控制台据此引导用户改用手动配置。
		logging.Errorf("channel qr: start session for device %s platform %s: %v", d.ID, desc.Name, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, newChannelQRSession(desc, sess))
}

// GetChannelQR GET /api/voicebots/:id/devices/:did/channels/:platform/qr/:sid
// 返回会话快照；成功后把凭证按平台 Schema 校验写入 device_channels，并带上通道状态。
func (h *ChannelHandler) GetChannelQR(c *gin.Context) {
	d, ok := deviceForVoicebot(c, h.voicebots, h.devices)
	if !ok {
		return
	}
	desc, binder, ok := h.qrBinder(c)
	if !ok {
		return
	}
	sess, err := binder.Get(d.ID, c.Param("sid"))
	if errors.Is(err, qrbind.ErrSessionNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "qr session not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := newChannelQRSession(desc, sess)
	if sess.Status == qrbind.StatusSuccess {
		channel, err := h.bindQRSuccess(d, desc, sess)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		resp.Channel = channel
	}
	c.JSON(http.StatusOK, resp)
}

// bindQRSuccess 把扫码拿到的凭证按平台 Schema 校验后写入 device_channels。
// Schema 是字段的权威：Binder 给的键必须过校验，否则不写库（防两端漂移）。
func (h *ChannelHandler) bindQRSuccess(d *store.Device, desc platform.Descriptor, sess qrbind.Session) (*deviceChannelStatus, error) {
	config := normalizeConfig(sess.Config)
	if err := desc.Validate(config); err != nil {
		logging.Errorf("channel qr: platform %s returned invalid config: %v", desc.Name, err)
		return nil, err
	}
	if err := h.channels.Upsert(d.ID, desc.Name, config); err != nil {
		return nil, err
	}
	return &deviceChannelStatus{
		Platform:    desc.Name,
		DisplayName: desc.DisplayName,
		Enabled:     true,
		Config:      desc.Mask(config),
	}, nil
}

// CancelChannelQR DELETE /api/voicebots/:id/devices/:did/channels/:platform/qr/:sid
// 结束会话并停掉后台轮询；幂等。
func (h *ChannelHandler) CancelChannelQR(c *gin.Context) {
	d, ok := deviceForVoicebot(c, h.voicebots, h.devices)
	if !ok {
		return
	}
	desc, binder, ok := h.qrBinder(c)
	if !ok {
		return
	}
	binder.Cancel(d.ID, c.Param("sid"))
	c.JSON(http.StatusOK, gin.H{"session_id": c.Param("sid"), "platform": desc.Name, "status": "canceled"})
}

// qrBinder 解析路径上的平台并取出它的扫码实现；失败时已写好响应。
func (h *ChannelHandler) qrBinder(c *gin.Context) (platform.Descriptor, qrbind.Binder, bool) {
	desc, ok := platform.Get(c.Param("platform"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown platform"})
		return platform.Descriptor{}, nil, false
	}
	binder := h.qrBinders[desc.Name]
	if binder == nil || !desc.QRBinding {
		c.JSON(http.StatusBadRequest, gin.H{"error": "platform does not support qr binding"})
		return platform.Descriptor{}, nil, false
	}
	return desc, binder, true
}

// newChannelQRSession 把会话快照转成响应；凭证明文不进响应。
func newChannelQRSession(desc platform.Descriptor, sess qrbind.Session) channelQRSession {
	return channelQRSession{
		SessionID: sess.ID,
		Platform:  desc.Name,
		Status:    string(sess.Status),
		QRContent: sess.QRContent,
		ExpiresAt: sess.ExpiresAt,
		Error:     sess.Error,
	}
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
