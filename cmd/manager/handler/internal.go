package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/store"
)

type InternalHandler struct {
	voicebots *store.VoicebotStore
	devices   *store.DeviceStore
	models    *store.AIModelStore
	voices    *store.ModelVoiceStore
	mcpBinds  *store.VoicebotMCPBindingStore
}

func NewInternalHandler(voicebots *store.VoicebotStore, devices *store.DeviceStore, models *store.AIModelStore, voices *store.ModelVoiceStore, mcpBinds *store.VoicebotMCPBindingStore) *InternalHandler {
	return &InternalHandler{voicebots: voicebots, devices: devices, models: models, voices: voices, mcpBinds: mcpBinds}
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
		full, err := h.assembleConfig(agentCfg, v.ID)
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

// GET /internal/devices/tg-bots — 返回所有配置了 TG Bot Token 的设备列表。
func (h *InternalHandler) DeviceTGBots(c *gin.Context) {
	devices, err := h.devices.ListWithTGBot()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	type tgBotDevice struct {
		DeviceID     string `json:"device_id"`
		DeviceName   string `json:"device_name"`
		TgBotToken   string `json:"tg_bot_token"`
		VoicebotID   string `json:"voicebot_id"`
		VoicebotName string `json:"voicebot_name,omitempty"`
	}
	result := make([]tgBotDevice, 0, len(devices))
	for _, d := range devices {
		entry := tgBotDevice{
			DeviceID:   d.ID,
			DeviceName: d.Name,
			TgBotToken: d.TgBotToken,
			VoicebotID: d.VoicebotID,
		}
		if v, err := h.voicebots.GetByID(d.VoicebotID); err == nil {
			entry.VoicebotName = v.Name
		}
		result = append(result, entry)
	}
	c.JSON(http.StatusOK, result)
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

func jsonbToSS(m map[string]any) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func (h *InternalHandler) assembleConfig(ac AgentConfig, voicebotID string) (*config.AppConfig, error) {
	full := config.DefaultConfig()

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

	if ac.TTS.VoiceID != "" {
		if voice, err := h.voices.GetByID(ac.TTS.VoiceID); err == nil {
			full.Provider.TTS.Aliyun.Voice = voice.VoiceID
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

	if ac.LLM.ModelID != "" {
		if m, err := h.models.GetByID(ac.LLM.ModelID); err == nil {
			full.Provider.LLM.OpenAI.Model = m.ModelID
			if m.Provider != nil {
				full.Provider.LLM.Type = llmAdapterFromSlug(m.Provider.Slug)
				full.Provider.LLM.OpenAI.APIKey = m.Provider.APIKeyEnc
				full.Provider.LLM.OpenAI.ExtraFields = mapValue(m.Provider.Extra, "extra_fields")
			}
			if options := mapValue(m.Extra, "options"); options != nil {
				full.Provider.LLM.OpenAI.Options, _ = json.Marshal(options)
			}
			if m.BaseURL != "" {
				full.Provider.LLM.OpenAI.BaseURL = m.BaseURL
			} else if m.Provider != nil && m.Provider.BaseURL != "" {
				full.Provider.LLM.OpenAI.BaseURL = m.Provider.BaseURL
			}
		}
	}
	full.Provider.LLM.OpenAI.Thinking = ac.LLM.Thinking
	if full.Provider.LLM.OpenAI.Thinking.IsDefault() {
		full.Provider.LLM.OpenAI.Thinking.Mode = llm.ThinkingModeDisabled
	}
	if ac.LLM.SoulPrompt != "" {
		full.Provider.LLM.OpenAI.SoulPrompt = ac.LLM.SoulPrompt
	}
	if ac.LLM.RulesPrompt != "" {
		full.Provider.LLM.OpenAI.RulesPrompt = ac.LLM.RulesPrompt
	}

	if ac.Memory.MemoryCharLimit > 0 {
		full.Memory = ac.Memory
	}
	if h.mcpBinds != nil {
		dbList, err := h.mcpBinds.ListByVoicebotWithServers(voicebotID)
		if err != nil {
			return nil, err
		}
		if len(dbList) > 0 {
			var mcpCfgs []config.MCPServerConfig
			for _, m := range dbList {
				mcpCfgs = append(mcpCfgs, config.MCPServerConfig{
					ID:           m.ID,
					Transport:    string(m.Transport),
					Command:      m.Command,
					Args:         m.Args,
					Env:          jsonbToSS(m.Env),
					CWD:          m.CWD,
					Endpoint:     m.Endpoint,
					Headers:      jsonbToSS(m.Headers),
					ToolNameList: m.ToolNameList,
					TimeoutMs:    m.TimeoutMs,
				})
			}
			full.Tools.MCP = mcpCfgs
		}
	}
	if len(ac.MCP) > 0 && len(full.Tools.MCP) == 0 {
		full.Tools.MCP = ac.MCP
	}
	if ac.Audio.SampleRate > 0 {
		full.Audio.InPipe.SampleRate = ac.Audio.SampleRate
		full.Provider.TTS.Aliyun.SampleRate = ac.Audio.SampleRate
	}

	return full, nil
}

func llmAdapterFromSlug(slug string) string {
	switch strings.ToLower(strings.TrimSpace(slug)) {
	case "llm/openai-responses", "openai-responses":
		return "openai-responses"
	case "llm/anthropic-messages", "anthropic-messages":
		return "anthropic-messages"
	case "llm/openai", "openai", "llm/openai-completions", "openai-completions":
		return "openai-completions"
	default:
		return "openai-completions"
	}
}

func mapValue(values map[string]any, key string) map[string]any {
	value, _ := values[key].(map[string]any)
	return value
}
