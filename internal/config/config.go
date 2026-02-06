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
	Server  ServerConfig  `json:"server"`
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
	Mixer        MixerConfig        `json:"mixer"`
	InPipe       InPipeConfig       `json:"in_pipe"`
	TTSPipeline  TTSPipelineConfig  `json:"tts_pipeline"`
	TTSScheduler TTSSchedulerConfig `json:"tts_scheduler"`
}

type TTSPipelineConfig struct {
	MaxTTSBuffer     int `json:"max_tts_buffer"`
	MaxConcurrentTTS int `json:"max_concurrent_tts"`
	TextQueueSize    int `json:"text_queue_size"`
}

type TTSSchedulerConfig struct {
	MaxInFlightSentences int `json:"max_in_flight_sentences"`
	MaxCacheSentences    int `json:"max_cache_sentences"`
}

type MixerConfig struct {
	TTSVolume       float64 `json:"tts_volume"`
	ResourceVolume  float64 `json:"resource_volume"`
	SampleRate      int     `json:"sample_rate"`
	Channels        int     `json:"channels"`
	FramesPerBuffer int     `json:"frames_per_buffer"`
}

type InPipeConfig struct {
	SampleRate   int     `json:"sample_rate"`
	Channels     int     `json:"channels"`
	EnableVAD    bool    `json:"enable_vad"`
	VADThreshold float64 `json:"vad_threshold"`
	BufferSize   int     `json:"buffer_size"`  // 缓冲区大小（样本数），默认 3200
	HighLatency  bool    `json:"high_latency"` // 高延迟模式，适合蓝牙设备
	InputDevice  string  `json:"input_device"` // 输入设备名称，空字符串表示使用默认设备
}

type ToolsConfig struct {
	Types           map[string]string `json:"types"`
	ActionResponses map[string]string `json:"action_responses"`
}

type ServerConfig struct {
	Address        string            `json:"address"`
	Path           string            `json:"path"`
	ReadTimeoutMs  int               `json:"read_timeout_ms"`
	WriteTimeoutMs int               `json:"write_timeout_ms"`
	Auth           AuthConfig        `json:"auth"`
	AudioParams    AudioParamsConfig `json:"audio_params"`
}

type AuthConfig struct {
	Enabled        bool     `json:"enabled"`
	Token          string   `json:"token"`
	AllowedDevices []string `json:"allowed_devices"`
}

type AudioParamsConfig struct {
	Format               string `json:"format"`
	SampleRate           int    `json:"sample_rate"`
	Channels             int    `json:"channels"`
	FrameDurationMs      int    `json:"frame_duration_ms"`
	BitsPerSample        int    `json:"bits_per_sample"`
	PlayBufferDurationMs int    `json:"play_buffer_duration_ms"`
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
				TTSVolume:       1.0,
				ResourceVolume:  1.0,
				FramesPerBuffer: 1024,
			},
			TTSPipeline: TTSPipelineConfig{
				MaxTTSBuffer:     3,
				MaxConcurrentTTS: 2,
				TextQueueSize:    100,
			},
			TTSScheduler: TTSSchedulerConfig{
				MaxInFlightSentences: 2,
				MaxCacheSentences:    0,
			},
			InPipe: InPipeConfig{
				SampleRate:   16000,
				Channels:     1,
				EnableVAD:    true,
				VADThreshold: 0.5,
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
		Server: ServerConfig{
			Address:        ":8000",
			Path:           "/xiaozhi/v1/",
			ReadTimeoutMs:  10000,
			WriteTimeoutMs: 10000,
			Auth: AuthConfig{
				Enabled:        false,
				Token:          "",
				AllowedDevices: nil,
			},
			AudioParams: AudioParamsConfig{
				Format:               "opus",
				SampleRate:           16000,
				Channels:             1,
				FrameDurationMs:      60,
				BitsPerSample:        16,
				PlayBufferDurationMs: 300,
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
	if c.Audio.Mixer.FramesPerBuffer < 0 {
		return errors.New("audio.mixer.frames_per_buffer must be non-negative")
	}
	if strings.TrimSpace(c.Server.Address) == "" {
		return errors.New("server.address must not be empty")
	}
	if strings.TrimSpace(c.Server.Path) == "" {
		return errors.New("server.path must not be empty")
	}

	ap := c.Server.AudioParams
	if ap.Format != "opus" && ap.Format != "pcm" {
		return fmt.Errorf("server.audio_params.format must be opus or pcm")
	}
	if ap.SampleRate != 16000 {
		return fmt.Errorf("server.audio_params.sample_rate must be 16000")
	}
	if ap.Channels != 1 && ap.Channels != 2 {
		return fmt.Errorf("server.audio_params.channels must be 1 or 2")
	}
	switch ap.FrameDurationMs {
	case 20, 40, 60, 100:
	default:
		return fmt.Errorf("server.audio_params.frame_duration_ms must be 20, 40, 60, or 100")
	}
	switch ap.BitsPerSample {
	case 16, 24, 32:
	default:
		return fmt.Errorf("server.audio_params.bits_per_sample must be 16, 24, or 32")
	}
	if ap.Format == "pcm" && ap.BitsPerSample != 16 {
		return fmt.Errorf("server.audio_params.bits_per_sample must be 16 when format is pcm")
	}
	if ap.PlayBufferDurationMs < 100 {
		return fmt.Errorf("server.audio_params.play_buffer_duration_ms must be >= 100")
	}
	if c.Audio.TTSScheduler.MaxInFlightSentences <= 0 {
		return errors.New("audio.tts_scheduler.max_in_flight_sentences must be positive")
	}
	if c.Audio.TTSScheduler.MaxCacheSentences < 0 {
		return errors.New("audio.tts_scheduler.max_cache_sentences must be >= 0")
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
