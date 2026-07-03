package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/store"
)

type InternalHandler struct {
	voicebots *store.VoicebotStore
	devices   *store.DeviceStore
}

func NewInternalHandler(voicebots *store.VoicebotStore, devices *store.DeviceStore) *InternalHandler {
	return &InternalHandler{voicebots: voicebots, devices: devices}
}

// GET /internal/device-config?device_id=xxx
// Returns the AppConfig JSON for the voicebot that owns the given device.
// 404 when the device is not registered.
func (h *InternalHandler) DeviceConfig(c *gin.Context) {
	deviceID := c.Query("device_id")
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "device_id is required"})
		return
	}

	d, err := h.devices.GetByID(deviceID)
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	v, err := h.voicebots.GetByID(d.VoicebotID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Validate the stored JSON is parseable before returning
	var cfg config.AppConfig
	if err := json.Unmarshal([]byte(v.ConfigJSON), &cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid stored config"})
		return
	}

	c.Data(http.StatusOK, "application/json", []byte(v.ConfigJSON))
}
