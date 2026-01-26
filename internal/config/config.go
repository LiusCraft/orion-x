package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

const DefaultPath = "config/voicebot.json"

type AppConfig struct {
	Logging LoggingConfig `json:"logging"`
	ASR     ASRConfig     `json:"asr"`
	TTS     TTSConfig     `json:"tts"`
	LLM     LLMConfig     `json:"llm"`
	Audio   AudioConfig   `json:"audio"`
	Tools   ToolsConfig   `json:"tools"`
}

type LoggingConfig struct {
	Level  string `json:"level"`
	Format string `json:"format"`
}

type ASRConfig struct {
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
	Endpoint string `json:"endpoint"`
}

type TTSConfig struct {
	APIKey               string            `json:"api_key"`
	Endpoint             string            `json:"endpoint"`
	Workspace            string            `json:"workspace"`
	Model                string            `json:"model"`
	Voice                string            `json:"voice"`
	Format               string            `json:"format"`
	SampleRate           int               `json:"sample_rate"`
	Volume               int               `json:"volume"`
	Rate                 float64           `json:"rate"`
	Pitch                float64           `json:"pitch"`
	EnableSSML           bool              `json:"enable_ssml"`
	TextType             string            `json:"text_type"`
	EnableDataInspection *bool             `json:"enable_data_inspection"`
	VoiceMap             map[string]string `json:"voice_map"`
}

type LLMConfig struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

type AudioConfig struct {
	Mixer  MixerConfig  `json:"mixer"`
	InPipe InPipeConfig `json:"in_pipe"`
}

type MixerConfig struct {
	TTSVolume      float64 `json:"tts_volume"`
	ResourceVolume float64 `json:"resource_volume"`
}

type InPipeConfig struct {
	SampleRate   int       `json:"sample_rate"`
	Channels     int       `json:"channels"`
	EnableVAD    bool      `json:"enable_vad"`
	VADThreshold float64   `json:"vad_threshold"`
	AEC          AECConfig `json:"aec"`
}

type AECConfig struct {
	Enable                  bool   `json:"enable"`
	Mode                    string `json:"mode"`
	FrameMs                 int    `json:"frame_ms"`
	FarEndDelayMs           int    `json:"far_end_delay_ms"`
	ReferenceActiveWindowMs int    `json:"reference_active_window_ms"`
}

type ToolsConfig struct {
	Types           map[string]string `json:"types"`
	ActionResponses map[string]string `json:"action_responses"`
}

func DefaultConfig() *AppConfig {
	enableDataInspection := true

	return &AppConfig{
		Logging: LoggingConfig{},
		ASR: ASRConfig{
			Model: "fun-asr-realtime",
		},
		TTS: TTSConfig{
			Model:                "cosyvoice-v3-flash",
			Voice:                "longanyang",
			Format:               "pcm",
			SampleRate:           16000,
			Volume:               50,
			Rate:                 1.0,
			Pitch:                1.0,
			TextType:             "PlainText",
			EnableDataInspection: &enableDataInspection,
			VoiceMap: map[string]string{
				"happy":   "longanyang",
				"sad":     "zhichu",
				"angry":   "zhimeng",
				"calm":    "longxiaochun",
				"excited": "longanyang",
				"default": "longanyang",
			},
		},
		LLM: LLMConfig{
			BaseURL: "https://open.bigmodel.cn/api/coding/paas/v4",
			Model:   "glm-4-flash",
		},
		Audio: AudioConfig{
			Mixer: MixerConfig{
				TTSVolume:      1.0,
				ResourceVolume: 1.0,
			},
			InPipe: InPipeConfig{
				SampleRate:   16000,
				Channels:     1,
				EnableVAD:    true,
				VADThreshold: 0.5,
				AEC: AECConfig{
					Enable:                  true,
					Mode:                    "gate",
					FrameMs:                 10,
					FarEndDelayMs:           50,
					ReferenceActiveWindowMs: 200,
				},
			},
		},
		Tools: ToolsConfig{
			Types: map[string]string{
				"getWeather": "query",
				"getTime":    "query",
				"search":     "query",
				"playMusic":  "action",
				"setVolume":  "action",
				"pauseMusic": "action",
			},
			ActionResponses: map[string]string{
				"playMusic":  "正在为您播放{{song}}",
				"setVolume":  "已将音量设置为{{level}}",
				"pauseMusic": "音乐已暂停",
			},
		},
	}
}

func Load(path string) (*AppConfig, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = DefaultPath
	}

	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg.ApplyEnv()
			return cfg, cfg.Validate()
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	cfg.ApplyEnv()
	return cfg, cfg.Validate()
}

func (c *AppConfig) ApplyEnv() {
	if level := strings.TrimSpace(os.Getenv("LOG_LEVEL")); level != "" {
		c.Logging.Level = level
	}
	if format := strings.TrimSpace(os.Getenv("LOG_FORMAT")); format != "" {
		c.Logging.Format = format
	}

	if dash := strings.TrimSpace(os.Getenv("DASHSCOPE_API_KEY")); dash != "" {
		c.ASR.APIKey = dash
		c.TTS.APIKey = dash
		if strings.TrimSpace(c.LLM.APIKey) == "" {
			c.LLM.APIKey = dash
		}
	}

	if zhipu := strings.TrimSpace(os.Getenv("ZHIPU_API_KEY")); zhipu != "" {
		c.LLM.APIKey = zhipu
	}
}

func (c *AppConfig) Validate() error {
	if c.Audio.InPipe.SampleRate <= 0 {
		return errors.New("audio.in_pipe.sample_rate must be positive")
	}
	if c.TTS.SampleRate <= 0 {
		return errors.New("tts.sample_rate must be positive")
	}

	for name, value := range c.Tools.Types {
		lower := strings.ToLower(strings.TrimSpace(value))
		switch lower {
		case "query", "action":
			continue
		default:
			return fmt.Errorf("invalid tool type for %s: %s", name, value)
		}
	}

	if c.Audio.InPipe.AEC.FrameMs < 0 {
		return errors.New("audio.in_pipe.aec.frame_ms must be non-negative")
	}
	if c.Audio.InPipe.AEC.FarEndDelayMs < 0 {
		return errors.New("audio.in_pipe.aec.far_end_delay_ms must be non-negative")
	}
	if c.Audio.InPipe.AEC.ReferenceActiveWindowMs < 0 {
		return errors.New("audio.in_pipe.aec.reference_active_window_ms must be non-negative")
	}

	return nil
}

func (c *AppConfig) ValidateKeys(requireASR, requireTTS, requireLLM bool) error {
	if requireASR && strings.TrimSpace(c.ASR.APIKey) == "" {
		return errors.New("asr api_key is required")
	}
	if requireTTS && strings.TrimSpace(c.TTS.APIKey) == "" {
		return errors.New("tts api_key is required")
	}
	if requireLLM && strings.TrimSpace(c.LLM.APIKey) == "" {
		return errors.New("llm api_key is required")
	}
	return nil
}
