package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

const DefaultWSServerPath = "data/ws-server.json"

type WSServerAppConfig struct {
	Logging  LoggingConfig          `json:"logging"`
	Server   ServerConfig           `json:"server"`
	Metrics  MetricsConfig          `json:"metrics"`
	Voicebot WSServerVoicebotConfig `json:"voicebot"`
}

type WSServerVoicebotConfig struct {
	Default       *VoicebotSessionConfig           `json:"default"`
	Profiles      map[string]VoicebotSessionConfig `json:"profiles"`
	LocalBindings map[string]string                `json:"local_bindings"`
}

type VoicebotSessionConfig struct {
	ASR    ASRConfig    `json:"asr"`
	TTS    TTSConfig    `json:"tts"`
	LLM    LLMConfig    `json:"llm"`
	Audio  AudioConfig  `json:"audio"`
	Tools  ToolsConfig  `json:"tools"`
	Memory MemoryConfig `json:"memory"`
}

func DefaultWSServerConfig() *WSServerAppConfig {
	base := DefaultConfig()
	defaultVoicebot := voicebotSessionFromApp(base)
	return &WSServerAppConfig{
		Logging: base.Logging,
		Server:  base.Server,
		Metrics: base.Metrics,
		Voicebot: WSServerVoicebotConfig{
			Default:       &defaultVoicebot,
			Profiles:      map[string]VoicebotSessionConfig{},
			LocalBindings: map[string]string{},
		},
	}
}

func LoadWSServer(path string) (*WSServerAppConfig, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = DefaultWSServerPath
	}

	cfg := DefaultWSServerConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg.ApplyEnv()
			return cfg, cfg.Validate()
		}
		return nil, fmt.Errorf("read ws server config %s: %w", path, err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse ws server config %s: %w", path, err)
	}

	cfg.ApplyEnv()
	return cfg, cfg.Validate()
}

func (c *WSServerAppConfig) ApplyEnv() {
	if level := strings.TrimSpace(os.Getenv("LOG_LEVEL")); level != "" {
		c.Logging.Level = level
	}
	if format := strings.TrimSpace(os.Getenv("LOG_FORMAT")); format != "" {
		c.Logging.Format = format
	}

	dash := strings.TrimSpace(os.Getenv("DASHSCOPE_API_KEY"))
	zhipu := strings.TrimSpace(os.Getenv("ZHIPU_API_KEY"))

	if c.Voicebot.Default != nil {
		applyAPIKeys(c.Voicebot.Default, dash, zhipu)
	}
	for id, profile := range c.Voicebot.Profiles {
		applyAPIKeys(&profile, dash, zhipu)
		c.Voicebot.Profiles[id] = profile
	}
}

func (c *WSServerAppConfig) Validate() error {
	if err := c.validateServerAndMetrics(); err != nil {
		return err
	}

	if c.Voicebot.Default != nil {
		resolved := c.resolvedDefaultSession()
		if err := validateVoicebotSessionConfig("voicebot.default", resolved); err != nil {
			return err
		}
	}

	for profileID := range c.Voicebot.Profiles {
		if strings.TrimSpace(profileID) == "" {
			return errors.New("voicebot.profiles contains empty profile id")
		}
		resolved, err := c.ResolveVoicebotForDeviceProfile(profileID)
		if err != nil {
			return err
		}
		if err := validateVoicebotSessionConfig("voicebot.profiles."+profileID, resolved); err != nil {
			return err
		}
	}

	for deviceID, profileID := range c.Voicebot.LocalBindings {
		if strings.TrimSpace(deviceID) == "" {
			return errors.New("voicebot.local_bindings contains empty device id")
		}
		if strings.TrimSpace(profileID) == "" {
			return fmt.Errorf("voicebot.local_bindings[%s] profile id must not be empty", deviceID)
		}
		if _, ok := c.Voicebot.Profiles[profileID]; !ok {
			return fmt.Errorf("voicebot.local_bindings[%s] references unknown profile: %s", deviceID, profileID)
		}
	}

	return nil
}

func (c *WSServerAppConfig) ResolveVoicebotForDevice(deviceID string) (VoicebotSessionConfig, string, bool) {
	deviceID = strings.TrimSpace(deviceID)
	if profileID, ok := c.Voicebot.LocalBindings[deviceID]; ok {
		profileID = strings.TrimSpace(profileID)
		if profileID == "" {
			return VoicebotSessionConfig{}, "", false
		}
		cfg, err := c.ResolveVoicebotForDeviceProfile(profileID)
		if err != nil {
			return VoicebotSessionConfig{}, "", false
		}
		return cfg, profileID, true
	}

	if c.Voicebot.Default == nil {
		return VoicebotSessionConfig{}, "", false
	}
	return c.resolvedDefaultSession(), "default", true
}

func (c *WSServerAppConfig) ResolveVoicebotForDeviceProfile(profileID string) (VoicebotSessionConfig, error) {
	profile, ok := c.Voicebot.Profiles[profileID]
	if !ok {
		return VoicebotSessionConfig{}, fmt.Errorf("voicebot profile not found: %s", profileID)
	}
	resolved := c.resolvedDefaultSession()
	resolved = mergeVoicebotSessionConfig(resolved, profile)
	return resolved, nil
}

func (c *WSServerAppConfig) resolvedDefaultSession() VoicebotSessionConfig {
	base := voicebotSessionFromApp(DefaultConfig())
	if c.Voicebot.Default != nil {
		return mergeVoicebotSessionConfig(base, *c.Voicebot.Default)
	}
	return base
}

func (c *WSServerAppConfig) validateServerAndMetrics() error {
	base := DefaultConfig()
	base.Server = c.Server
	base.Metrics = c.Metrics
	return base.Validate()
}

func validateVoicebotSessionConfig(name string, cfg VoicebotSessionConfig) error {
	base := DefaultConfig()
	base.ASR = cfg.ASR
	base.TTS = cfg.TTS
	base.LLM = cfg.LLM
	base.Audio = cfg.Audio
	base.Tools = cfg.Tools
	base.Memory = cfg.Memory
	if err := base.Validate(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := base.ValidateKeys(true, true, true); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func voicebotSessionFromApp(app *AppConfig) VoicebotSessionConfig {
	return VoicebotSessionConfig{
		ASR:    app.ASR,
		TTS:    app.TTS,
		LLM:    app.LLM,
		Audio:  app.Audio,
		Tools:  app.Tools,
		Memory: app.Memory,
	}
}

func mergeVoicebotSessionConfig(base VoicebotSessionConfig, override VoicebotSessionConfig) VoicebotSessionConfig {
	out := base

	if strings.TrimSpace(override.ASR.APIKey) != "" {
		out.ASR.APIKey = override.ASR.APIKey
	}
	if strings.TrimSpace(override.ASR.Model) != "" {
		out.ASR.Model = override.ASR.Model
	}
	if strings.TrimSpace(override.ASR.Endpoint) != "" {
		out.ASR.Endpoint = override.ASR.Endpoint
	}

	if strings.TrimSpace(override.TTS.APIKey) != "" {
		out.TTS.APIKey = override.TTS.APIKey
	}
	if strings.TrimSpace(override.TTS.Endpoint) != "" {
		out.TTS.Endpoint = override.TTS.Endpoint
	}
	if strings.TrimSpace(override.TTS.Workspace) != "" {
		out.TTS.Workspace = override.TTS.Workspace
	}
	if strings.TrimSpace(override.TTS.Model) != "" {
		out.TTS.Model = override.TTS.Model
	}
	if strings.TrimSpace(override.TTS.Voice) != "" {
		out.TTS.Voice = override.TTS.Voice
	}
	if strings.TrimSpace(override.TTS.Format) != "" {
		out.TTS.Format = override.TTS.Format
	}
	if override.TTS.SampleRate > 0 {
		out.TTS.SampleRate = override.TTS.SampleRate
	}
	if override.TTS.Volume != 0 {
		out.TTS.Volume = override.TTS.Volume
	}
	if override.TTS.Rate != 0 {
		out.TTS.Rate = override.TTS.Rate
	}
	if override.TTS.Pitch != 0 {
		out.TTS.Pitch = override.TTS.Pitch
	}
	out.TTS.EnableSSML = override.TTS.EnableSSML
	if strings.TrimSpace(override.TTS.TextType) != "" {
		out.TTS.TextType = override.TTS.TextType
	}
	if override.TTS.EnableDataInspection != nil {
		out.TTS.EnableDataInspection = override.TTS.EnableDataInspection
	}
	if len(override.TTS.VoiceMap) > 0 {
		out.TTS.VoiceMap = cloneStringMap(override.TTS.VoiceMap)
	}

	if strings.TrimSpace(override.LLM.APIKey) != "" {
		out.LLM.APIKey = override.LLM.APIKey
	}
	if strings.TrimSpace(override.LLM.BaseURL) != "" {
		out.LLM.BaseURL = override.LLM.BaseURL
	}
	if strings.TrimSpace(override.LLM.Model) != "" {
		out.LLM.Model = override.LLM.Model
	}

	if override.Audio.Mixer.TTSVolume != 0 {
		out.Audio.Mixer.TTSVolume = override.Audio.Mixer.TTSVolume
	}
	if override.Audio.Mixer.ResourceVolume != 0 {
		out.Audio.Mixer.ResourceVolume = override.Audio.Mixer.ResourceVolume
	}
	if override.Audio.Mixer.SampleRate > 0 {
		out.Audio.Mixer.SampleRate = override.Audio.Mixer.SampleRate
	}
	if override.Audio.Mixer.Channels > 0 {
		out.Audio.Mixer.Channels = override.Audio.Mixer.Channels
	}
	if override.Audio.Mixer.FramesPerBuffer > 0 {
		out.Audio.Mixer.FramesPerBuffer = override.Audio.Mixer.FramesPerBuffer
	}

	if override.Audio.InPipe.SampleRate > 0 {
		out.Audio.InPipe.SampleRate = override.Audio.InPipe.SampleRate
	}
	if override.Audio.InPipe.Channels > 0 {
		out.Audio.InPipe.Channels = override.Audio.InPipe.Channels
	}
	out.Audio.InPipe.EnableVAD = override.Audio.InPipe.EnableVAD
	if override.Audio.InPipe.VADThreshold > 0 {
		out.Audio.InPipe.VADThreshold = override.Audio.InPipe.VADThreshold
	}
	if strings.TrimSpace(override.Audio.InPipe.VADType) != "" {
		out.Audio.InPipe.VADType = override.Audio.InPipe.VADType
	}
	if strings.TrimSpace(override.Audio.InPipe.VADModelPath) != "" {
		out.Audio.InPipe.VADModelPath = override.Audio.InPipe.VADModelPath
	}
	if override.Audio.InPipe.VADMinSilenceMs > 0 {
		out.Audio.InPipe.VADMinSilenceMs = override.Audio.InPipe.VADMinSilenceMs
	}
	if override.Audio.InPipe.VADSpeechPadMs > 0 {
		out.Audio.InPipe.VADSpeechPadMs = override.Audio.InPipe.VADSpeechPadMs
	}
	if override.Audio.InPipe.BufferSize > 0 {
		out.Audio.InPipe.BufferSize = override.Audio.InPipe.BufferSize
	}
	out.Audio.InPipe.HighLatency = override.Audio.InPipe.HighLatency
	if strings.TrimSpace(override.Audio.InPipe.InputDevice) != "" {
		out.Audio.InPipe.InputDevice = override.Audio.InPipe.InputDevice
	}

	if override.Audio.TTSPipeline.MaxTTSBuffer > 0 {
		out.Audio.TTSPipeline.MaxTTSBuffer = override.Audio.TTSPipeline.MaxTTSBuffer
	}
	if override.Audio.TTSPipeline.MaxConcurrentTTS > 0 {
		out.Audio.TTSPipeline.MaxConcurrentTTS = override.Audio.TTSPipeline.MaxConcurrentTTS
	}
	if override.Audio.TTSPipeline.TextQueueSize > 0 {
		out.Audio.TTSPipeline.TextQueueSize = override.Audio.TTSPipeline.TextQueueSize
	}
	if override.Audio.TTSScheduler.MaxInFlightSentences > 0 {
		out.Audio.TTSScheduler.MaxInFlightSentences = override.Audio.TTSScheduler.MaxInFlightSentences
	}
	if override.Audio.TTSScheduler.MaxCacheSentences > 0 {
		out.Audio.TTSScheduler.MaxCacheSentences = override.Audio.TTSScheduler.MaxCacheSentences
	}

	if len(override.Tools.Types) > 0 {
		out.Tools.Types = cloneStringMap(override.Tools.Types)
	}
	if len(override.Tools.ActionResponses) > 0 {
		out.Tools.ActionResponses = cloneStringMap(override.Tools.ActionResponses)
	}
	if len(override.Tools.MCP) > 0 {
		out.Tools.MCP = append([]MCPServerConfig(nil), override.Tools.MCP...)
	}

	if strings.TrimSpace(override.Memory.Mode) != "" {
		out.Memory.Mode = override.Memory.Mode
	}
	if override.Memory.SessionMaxTurns > 0 {
		out.Memory.SessionMaxTurns = override.Memory.SessionMaxTurns
	}
	if override.Memory.SessionSummaryEveryN > 0 {
		out.Memory.SessionSummaryEveryN = override.Memory.SessionSummaryEveryN
	}
	if strings.TrimSpace(override.Memory.LongTermDBPath) != "" {
		out.Memory.LongTermDBPath = override.Memory.LongTermDBPath
	}
	if override.Memory.LongTermMaxResults > 0 {
		out.Memory.LongTermMaxResults = override.Memory.LongTermMaxResults
	}
	if override.Memory.RetentionDays > 0 {
		out.Memory.RetentionDays = override.Memory.RetentionDays
	}
	if override.Memory.FTSMinScore > 0 {
		out.Memory.FTSMinScore = override.Memory.FTSMinScore
	}

	return out
}

func applyAPIKeys(cfg *VoicebotSessionConfig, dash, zhipu string) {
	if strings.TrimSpace(dash) != "" {
		cfg.ASR.APIKey = dash
		cfg.TTS.APIKey = dash
		if strings.TrimSpace(cfg.LLM.APIKey) == "" {
			cfg.LLM.APIKey = dash
		}
	}
	if strings.TrimSpace(zhipu) != "" {
		cfg.LLM.APIKey = zhipu
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
