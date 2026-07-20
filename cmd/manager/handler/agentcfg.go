package handler

import (
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/llm"
)

// AgentConfig is the minimal agent config stored in config_json.
// It only stores model/voice references and user-adjustable parameters.
// /internal/device-config assembles the full AppConfig at runtime.
type AgentConfig struct {
	Language string                   `json:"language,omitempty"`
	ASR      ASRAgentConfig           `json:"asr"`
	TTS      TTSAgentConfig           `json:"tts"`
	LLM      LLMAgentConfig           `json:"llm"`
	Audio    AudioAgentConfig         `json:"audio,omitempty"`
	Memory   config.MemoryConfig      `json:"memory,omitempty"`
	MCP      []config.MCPServerConfig `json:"mcp,omitempty"`
}

type ASRAgentConfig struct {
	ModelID         string  `json:"model_id"`
	VADMode         string  `json:"vad_mode"`
	VADThreshold    float64 `json:"vad_threshold"`
	VADMinSilenceMs int     `json:"vad_min_silence_ms"`
	VADSpeechPadMs  int     `json:"vad_speech_pad_ms"`
}

type TTSAgentConfig struct {
	ModelID string  `json:"model_id"`
	VoiceID string  `json:"voice_id"`
	Volume  int     `json:"volume"`
	Rate    float64 `json:"rate"`
	Pitch   float64 `json:"pitch"`
}

type LLMAgentConfig struct {
	ModelID     string             `json:"model_id"`
	SoulPrompt  string             `json:"soul_prompt,omitempty"`
	RulesPrompt string             `json:"rules_prompt,omitempty"`
	Thinking    llm.ThinkingConfig `json:"thinking,omitempty"`
}

type AudioAgentConfig struct {
	SampleRate int `json:"sample_rate"`
}
