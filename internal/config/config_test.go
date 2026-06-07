package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_MergesDefaultsAndEnv(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "voicebot.json")
	data := `{
		"logging": {"level": "debug"},
		"audio": {"in_pipe": {"sample_rate": 8000}}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("DASHSCOPE_API_KEY", "dash-key")
	t.Setenv("ZHIPU_API_KEY", "zhipu-key")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Logging.Level != "warn" {
		t.Fatalf("expected LOG_LEVEL to override config, got %q", cfg.Logging.Level)
	}
	if cfg.Audio.InPipe.SampleRate != 8000 {
		t.Fatalf("expected sample rate to be 8000, got %d", cfg.Audio.InPipe.SampleRate)
	}
	if cfg.Provider.ASR.Aliyun.APIKey != "dash-key" {
		t.Fatalf("expected ASR api key from env")
	}
	if cfg.Provider.TTS.Aliyun.APIKey != "dash-key" {
		t.Fatalf("expected TTS api key from env")
	}
	if cfg.Provider.LLM.OpenAI.APIKey != "zhipu-key" {
		t.Fatalf("expected LLM api key from env")
	}
}

func TestValidateKeys(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.ValidateKeys(true, true, true); err == nil {
		t.Fatalf("expected error when keys are missing")
	}

	cfg.Provider.ASR.Aliyun.APIKey = "asr"
	cfg.Provider.TTS.Aliyun.APIKey = "tts"
	cfg.Provider.LLM.OpenAI.APIKey = "llm"
	if err := cfg.ValidateKeys(true, true, true); err != nil {
		t.Fatalf("unexpected key validation error: %v", err)
	}
}

func TestVoicebotExampleConfigLoadsForLocalVoicebot(t *testing.T) {
	path := filepath.Join("..", "..", "voicebot.example.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read voicebot example: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse voicebot example: %v", err)
	}
	if _, ok := raw["server"]; ok {
		t.Fatalf("voicebot.example.json should not contain top-level server config")
	}
	if _, ok := raw["metrics"]; ok {
		t.Fatalf("voicebot.example.json should not contain top-level metrics config")
	}
	if _, ok := raw["asr"]; ok {
		t.Fatalf("voicebot.example.json should not contain top-level asr config")
	}
	if _, ok := raw["tts"]; ok {
		t.Fatalf("voicebot.example.json should not contain top-level tts config")
	}
	if _, ok := raw["llm"]; ok {
		t.Fatalf("voicebot.example.json should not contain top-level llm config")
	}
	if _, ok := raw["provider"]; !ok {
		t.Fatalf("voicebot.example.json should contain top-level provider config")
	}

	t.Setenv("DASHSCOPE_API_KEY", "dash-key")
	t.Setenv("ZHIPU_API_KEY", "zhipu-key")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(voicebot.example.json) error = %v", err)
	}
	if err := cfg.ValidateKeys(true, true, true); err != nil {
		t.Fatalf("ValidateKeys(voicebot.example.json) error = %v", err)
	}
}

func TestValidateOriginCheck(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.OriginCheck.Enabled = true
	cfg.Server.OriginCheck.AllowedOrigins = []string{"https://example.com"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected origin validation error: %v", err)
	}

	cfg = DefaultConfig()
	cfg.Server.OriginCheck.Enabled = true
	cfg.Server.OriginCheck.AllowedOrigins = []string{"example.com"}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected invalid origin error")
	}
}

func TestValidateMetrics(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Metrics.Enabled = true
	cfg.Metrics.Path = ""
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected invalid metrics path error")
	}

	cfg = DefaultConfig()
	cfg.Metrics.Enabled = true
	cfg.Metrics.Address = "not-a-host-port"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected invalid metrics address error")
	}
}

func TestValidateMCPConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Tools.MCP = []MCPServerConfig{
		{
			ID:        "demo",
			Transport: "sse",
			Endpoint:  "http://localhost:12345/sse",
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected mcp validate error: %v", err)
	}

	cfg = DefaultConfig()
	cfg.Tools.MCP = []MCPServerConfig{
		{
			ID:        "bad",
			Transport: "unknown",
			Endpoint:  "http://localhost:12345/sse",
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected invalid transport error")
	}

	cfg = DefaultConfig()
	cfg.Tools.MCP = []MCPServerConfig{
		{
			ID:        "stdio",
			Transport: "stdio",
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected missing command error")
	}

	cfg = DefaultConfig()
	cfg.Tools.MCP = []MCPServerConfig{
		{
			ID:        "dup",
			Transport: "sse",
			Endpoint:  "http://localhost:12345/sse",
		},
		{
			ID:        "dup",
			Transport: "sse",
			Endpoint:  "http://localhost:12345/sse",
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected duplicate id error")
	}
}
