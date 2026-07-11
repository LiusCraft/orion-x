package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

const DefaultPath = "data/voicebot.json"

type AppConfig struct {
	Logging  LoggingConfig  `json:"logging"`
	Provider ProviderConfig `json:"provider"`
	Audio    AudioConfig    `json:"audio"`
	Tools    ToolsConfig    `json:"tools"`
	Memory   MemoryConfig   `json:"memory"`
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
	APIKey      string         `json:"api_key"`
	BaseURL     string         `json:"base_url"`
	Model       string         `json:"model"`
	Prompt      string         `json:"prompt,omitempty"`
	ExtraFields map[string]any `json:"extra_fields,omitempty"`
}

type ProviderConfig struct {
	ASR ASRProviderConfig `json:"asr"`
	TTS TTSProviderConfig `json:"tts"`
	LLM LLMProviderConfig `json:"llm"`
}

type ASRProviderConfig struct {
	Type   string    `json:"type"`
	Aliyun ASRConfig `json:"aliyun"`
}

type TTSProviderConfig struct {
	Type   string    `json:"type"`
	Aliyun TTSConfig `json:"aliyun"`
}

type LLMProviderConfig struct {
	Type   string    `json:"type"`
	OpenAI LLMConfig `json:"openai"`
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
	SampleRate      int     `json:"sample_rate"`
	Channels        int     `json:"channels"`
	EnableVAD       bool    `json:"enable_vad"`
	VADThreshold    float64 `json:"vad_threshold"`      // Silero speech probability threshold, default 0.5
	VADType         string  `json:"vad_type"`           // "silero", default "silero"
	VADModelPath    string  `json:"vad_model_path"`     // Silero VAD model path, default "models/silero_vad.onnx"
	VADMinSilenceMs int     `json:"vad_min_silence_ms"` // Silero VAD min silence duration in ms, default 500
	VADSpeechPadMs  int     `json:"vad_speech_pad_ms"`  // Silero VAD speech pad in ms, default 300
	BufferSize      int     `json:"buffer_size"`        // 缓冲区大小（样本数），默认 3200
	HighLatency     bool    `json:"high_latency"`       // 高延迟模式，适合蓝牙设备
	InputDevice     string  `json:"input_device"`       // 输入设备名称，空字符串表示使用默认设备
}

type ToolsConfig struct {
	MCP []MCPServerConfig `json:"mcp"`
}

type MemoryConfig struct {
	Mode                 string  `json:"mode"`
	SessionMaxTurns      int     `json:"session_max_turns"`
	SessionSummaryEveryN int     `json:"session_summary_every_n"`
	LongTermDBPath       string  `json:"long_term_db_path"`
	LongTermMaxResults   int     `json:"long_term_max_results"`
	RetentionDays        int     `json:"retention_days"`
	FTSMinScore          float64 `json:"fts_min_score"`
	MemoryCharLimit      int     `json:"memory_char_limit"`
	UserCharLimit        int     `json:"user_char_limit"`
	Review               struct {
		Enabled bool   `json:"enabled"`
		Model   string `json:"model"`
	} `json:"review"`
}

type MCPServerConfig struct {
	ID           string            `json:"id"`
	Transport    string            `json:"transport"` // stdio | sse | streamable
	Command      string            `json:"command"`
	Args         []string          `json:"args"`
	Env          map[string]string `json:"env,omitempty"`
	CWD          string            `json:"cwd,omitempty"`
	Endpoint     string            `json:"endpoint"`
	Headers      map[string]string `json:"headers,omitempty"`
	ToolNameList []string          `json:"tool_name_list"`
	TimeoutMs    int               `json:"timeout_ms"`
}

// ---------- Provider config ----------

func DefaultConfig() *AppConfig {
	enableDataInspection := true
	defaultASR := ASRConfig{
		Model: "fun-asr-realtime",
	}
	defaultTTS := TTSConfig{
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
	}
	defaultLLM := LLMConfig{
		BaseURL: "https://open.bigmodel.cn/api/coding/paas/v4",
		Model:   "glm-4-flash",
	}

	return &AppConfig{
		Logging: LoggingConfig{},
		Provider: ProviderConfig{
			ASR: ASRProviderConfig{
				Type:   "aliyun",
				Aliyun: defaultASR,
			},
			TTS: TTSProviderConfig{
				Type:   "aliyun",
				Aliyun: defaultTTS,
			},
			LLM: LLMProviderConfig{
				Type:   "openai",
				OpenAI: defaultLLM,
			},
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
				SampleRate:      16000,
				Channels:        1,
				EnableVAD:       true,
				VADThreshold:    0.5,
				VADType:         "silero",
				VADModelPath:    "models/silero_vad.onnx",
				VADMinSilenceMs: 500,
				VADSpeechPadMs:  300,
			},
		},
		Tools: ToolsConfig{MCP: nil},
		Memory: MemoryConfig{
			Mode:                 "session",
			SessionMaxTurns:      10,
			SessionSummaryEveryN: 20,
			LongTermDBPath:       "data/memory.db",
			LongTermMaxResults:   6,
			RetentionDays:        365,
			FTSMinScore:          0,
			MemoryCharLimit:      2200,
			UserCharLimit:        1375,
			Review: struct {
				Enabled bool   `json:"enabled"`
				Model   string `json:"model"`
			}{
				Enabled: true,
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

	cfg.NormalizeProviders()
	cfg.ApplyEnv()
	return cfg, cfg.Validate()
}

func (c *AppConfig) NormalizeProviders() {
	if strings.TrimSpace(c.Provider.ASR.Type) == "" {
		c.Provider.ASR.Type = "aliyun"
	}
	if strings.TrimSpace(c.Provider.TTS.Type) == "" {
		c.Provider.TTS.Type = "aliyun"
	}
	if strings.TrimSpace(c.Provider.LLM.Type) == "" {
		c.Provider.LLM.Type = "openai"
	}
}

func (c *AppConfig) ApplyEnv() {
	if level := strings.TrimSpace(os.Getenv("LOG_LEVEL")); level != "" {
		c.Logging.Level = level
	}
	if format := strings.TrimSpace(os.Getenv("LOG_FORMAT")); format != "" {
		c.Logging.Format = format
	}

	if dash := strings.TrimSpace(os.Getenv("DASHSCOPE_API_KEY")); dash != "" {
		c.Provider.ASR.Aliyun.APIKey = dash
		c.Provider.TTS.Aliyun.APIKey = dash
		if strings.TrimSpace(c.Provider.LLM.OpenAI.APIKey) == "" {
			c.Provider.LLM.OpenAI.APIKey = dash
		}
	}

	if zhipu := strings.TrimSpace(os.Getenv("ZHIPU_API_KEY")); zhipu != "" {
		c.Provider.LLM.OpenAI.APIKey = zhipu
	}
}

func (c *AppConfig) Validate() error {
	if c.Audio.InPipe.SampleRate <= 0 {
		return errors.New("audio.in_pipe.sample_rate must be positive")
	}
	vadType := strings.ToLower(strings.TrimSpace(c.Audio.InPipe.VADType))
	if c.Audio.InPipe.EnableVAD && vadType != "" && vadType != "silero" {
		return fmt.Errorf("audio.in_pipe.vad_type must be silero")
	}
	asrCfg := c.Provider.ASR.Aliyun
	ttsCfg := c.Provider.TTS.Aliyun
	llmCfg := c.Provider.LLM.OpenAI
	if strings.ToLower(strings.TrimSpace(c.Provider.ASR.Type)) != "aliyun" {
		return fmt.Errorf("provider.asr.type must be aliyun")
	}
	if strings.ToLower(strings.TrimSpace(c.Provider.TTS.Type)) != "aliyun" {
		return fmt.Errorf("provider.tts.type must be aliyun")
	}
	if strings.ToLower(strings.TrimSpace(c.Provider.LLM.Type)) != "openai" {
		return fmt.Errorf("provider.llm.type must be openai")
	}
	if ttsCfg.SampleRate <= 0 {
		return errors.New("provider.tts.aliyun.sample_rate must be positive")
	}
	if strings.TrimSpace(asrCfg.Model) == "" {
		return errors.New("provider.asr.aliyun.model must not be empty")
	}
	if strings.TrimSpace(ttsCfg.Model) == "" {
		return errors.New("provider.tts.aliyun.model must not be empty")
	}
	if strings.TrimSpace(llmCfg.Model) == "" {
		return errors.New("provider.llm.openai.model must not be empty")
	}
	if c.Audio.Mixer.FramesPerBuffer < 0 {
		return errors.New("audio.mixer.frames_per_buffer must be non-negative")
	}
	if c.Audio.TTSScheduler.MaxInFlightSentences <= 0 {
		return errors.New("audio.tts_scheduler.max_in_flight_sentences must be positive")
	}
	if c.Audio.TTSScheduler.MaxCacheSentences < 0 {
		return errors.New("audio.tts_scheduler.max_cache_sentences must be >= 0")
	}

	mcpIDs := make(map[string]struct{})
	for i, server := range c.Tools.MCP {
		id := strings.TrimSpace(server.ID)
		if id == "" {
			return fmt.Errorf("tools.mcp[%d].id must not be empty", i)
		}
		if _, exists := mcpIDs[id]; exists {
			return fmt.Errorf("duplicate tools.mcp.id: %s", id)
		}
		mcpIDs[id] = struct{}{}

		transport := strings.ToLower(strings.TrimSpace(server.Transport))
		switch transport {
		case "stdio", "sse", "streamable", "stream_http":
		default:
			return fmt.Errorf("invalid tools.mcp[%d].transport: %s", i, server.Transport)
		}

		if transport == "stdio" {
			if strings.TrimSpace(server.Command) == "" {
				return fmt.Errorf("tools.mcp[%d].command must not be empty", i)
			}
		} else {
			if strings.TrimSpace(server.Endpoint) == "" {
				return fmt.Errorf("tools.mcp[%d].endpoint must not be empty", i)
			}
		}
		if server.TimeoutMs < 0 {
			return fmt.Errorf("tools.mcp[%d].timeout_ms must be >= 0", i)
		}
	}

	memoryMode := strings.TrimSpace(c.Memory.Mode)
	if memoryMode == "" {
		memoryMode = "session"
	}
	switch memoryMode {
	case "none", "session", "long_term":
	default:
		return fmt.Errorf("memory.mode must be none, session, or long_term")
	}
	if c.Memory.SessionMaxTurns < 0 {
		return errors.New("memory.session_max_turns must be >= 0")
	}
	if c.Memory.SessionSummaryEveryN < 0 {
		return errors.New("memory.session_summary_every_n must be >= 0")
	}
	if c.Memory.LongTermMaxResults < 0 {
		return errors.New("memory.long_term_max_results must be >= 0")
	}
	if c.Memory.RetentionDays < 0 {
		return errors.New("memory.retention_days must be >= 0")
	}
	if memoryMode == "long_term" && strings.TrimSpace(c.Memory.LongTermDBPath) == "" {
		return errors.New("memory.long_term_db_path must not be empty when long_term mode")
	}

	return nil
}

func (c *AppConfig) ValidateKeys(requireASR, requireTTS, requireLLM bool) error {
	if requireASR && strings.TrimSpace(c.Provider.ASR.Aliyun.APIKey) == "" {
		return errors.New("provider.asr.aliyun.api_key is required")
	}
	if requireTTS && strings.TrimSpace(c.Provider.TTS.Aliyun.APIKey) == "" {
		return errors.New("provider.tts.aliyun.api_key is required")
	}
	if requireLLM && strings.TrimSpace(c.Provider.LLM.OpenAI.APIKey) == "" {
		return errors.New("provider.llm.openai.api_key is required")
	}
	return nil
}
