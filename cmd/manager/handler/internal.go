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
	models    *store.AIModelStore
	voices    *store.ModelVoiceStore
}

func NewInternalHandler(voicebots *store.VoicebotStore, devices *store.DeviceStore, models *store.AIModelStore, voices *store.ModelVoiceStore) *InternalHandler {
	return &InternalHandler{voicebots: voicebots, devices: devices, models: models, voices: voices}
}

// GET /internal/device-config?device_id=xxx
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

	var agentCfg AgentConfig
	if err := json.Unmarshal([]byte(v.ConfigJSON), &agentCfg); err == nil && agentCfg.ASR.ModelID != "" {
		full, err := h.assembleConfig(agentCfg)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, full)
		return
	}

	var oldCfg config.AppConfig
	if err := json.Unmarshal([]byte(v.ConfigJSON), &oldCfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid stored config"})
		return
	}
	c.Data(http.StatusOK, "application/json", []byte(v.ConfigJSON))
}

func effectiveBaseURL2(m store.AIModel) string {
	if m.BaseURL != "" {
		return m.BaseURL
	}
	if m.Provider != nil && m.Provider.BaseURL != "" {
		return m.Provider.BaseURL
	}
	return ""
}

func (h *InternalHandler) assembleConfig(ac AgentConfig) (*config.AppConfig, error) {
	full := config.DefaultConfig()

	// ── ASR ──
	if ac.ASR.ModelID != "" {
		if m, err := h.models.GetByID(ac.ASR.ModelID); err == nil {
			full.Provider.ASR.Aliyun.Model = m.ModelID
			if m.Provider != nil {
				full.Provider.ASR.Aliyun.APIKey = m.Provider.APIKeyEnc
			}
			full.Provider.ASR.Aliyun.Endpoint = effectiveBaseURL2(*m)
		}
	}

	vadMode := ac.ASR.VADMode
	if vadMode == "" {
		vadMode = "auto"
	}
	full.Audio.InPipe.EnableVAD = vadMode != "manual"
	if ac.ASR.VADThreshold > 0 {
		full.Audio.InPipe.VADThreshold = ac.ASR.VADThreshold
	}
	if ac.ASR.VADMinSilenceMs > 0 {
		full.Audio.InPipe.VADMinSilenceMs = ac.ASR.VADMinSilenceMs
	}
	if ac.ASR.VADSpeechPadMs > 0 {
		full.Audio.InPipe.VADSpeechPadMs = ac.ASR.VADSpeechPadMs
	}

	// ── TTS: voice lookup (voice_id stores the ModelVoice DB UUID) ──
	if ac.TTS.VoiceID != "" {
		if voice, err := h.voices.GetByID(ac.TTS.VoiceID); err == nil {
			full.Provider.TTS.Aliyun.Voice = voice.VoiceID
			// If model not already set via model_id, derive from voice
			if ac.TTS.ModelID == "" {
				if m, err := h.models.GetByID(voice.ModelID); err == nil {
					full.Provider.TTS.Aliyun.Model = m.ModelID
					if m.Provider != nil {
						full.Provider.TTS.Aliyun.APIKey = m.Provider.APIKeyEnc
					}
					full.Provider.TTS.Aliyun.Endpoint = effectiveBaseURL2(*m)
				}
			}
		}
	}

	if ac.TTS.ModelID != "" {
		if m, err := h.models.GetByID(ac.TTS.ModelID); err == nil {
			full.Provider.TTS.Aliyun.Model = m.ModelID
			if m.Provider != nil {
				full.Provider.TTS.Aliyun.APIKey = m.Provider.APIKeyEnc
			}
			full.Provider.TTS.Aliyun.Endpoint = effectiveBaseURL2(*m)
		}
	}
	full.Provider.TTS.Aliyun.Volume = ac.TTS.Volume
	full.Provider.TTS.Aliyun.Rate = ac.TTS.Rate
	full.Provider.TTS.Aliyun.Pitch = ac.TTS.Pitch

	// ── LLM ──
	if ac.LLM.ModelID != "" {
		if m, err := h.models.GetByID(ac.LLM.ModelID); err == nil {
			full.Provider.LLM.OpenAI.Model = m.ModelID
			if m.Provider != nil {
				full.Provider.LLM.OpenAI.APIKey = m.Provider.APIKeyEnc
			}
			if m.BaseURL != "" {
				full.Provider.LLM.OpenAI.BaseURL = m.BaseURL
			} else if m.Provider != nil && m.Provider.BaseURL != "" {
				full.Provider.LLM.OpenAI.BaseURL = m.Provider.BaseURL
			}
		}
	}
	if ac.LLM.Prompt != "" {
		full.Provider.LLM.OpenAI.Prompt = ac.LLM.Prompt
	}

	// ── Memory / MCP / Audio ──
	if ac.Memory.Mode != "" {
		full.Memory = ac.Memory
	}
	if len(ac.MCP) > 0 {
		full.Tools.MCP = ac.MCP
	}
	if ac.Audio.SampleRate > 0 {
		full.Audio.InPipe.SampleRate = ac.Audio.SampleRate
		full.Provider.TTS.Aliyun.SampleRate = ac.Audio.SampleRate
	}

	return full, nil
}
