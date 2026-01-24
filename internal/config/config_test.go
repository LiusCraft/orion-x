package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_MergesDefaultsAndEnv(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "voicebot.json")
	data := `{
		"logging": {"level": "debug"},
		"audio": {"in_pipe": {"sample_rate": 8000}},
		"tools": {"types": {"getTime": "query"}}
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
	if cfg.Tools.Types["playMusic"] != "action" {
		t.Fatalf("expected default tool types to be preserved")
	}
	if cfg.ASR.APIKey != "dash-key" {
		t.Fatalf("expected ASR api key from env")
	}
	if cfg.TTS.APIKey != "dash-key" {
		t.Fatalf("expected TTS api key from env")
	}
	if cfg.LLM.APIKey != "zhipu-key" {
		t.Fatalf("expected LLM api key from env")
	}
}

func TestValidateKeys(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.ValidateKeys(true, true, true); err == nil {
		t.Fatalf("expected error when keys are missing")
	}

	cfg.ASR.APIKey = "asr"
	cfg.TTS.APIKey = "tts"
	cfg.LLM.APIKey = "llm"
	if err := cfg.ValidateKeys(true, true, true); err != nil {
		t.Fatalf("unexpected key validation error: %v", err)
	}
}

func TestValidateToolTypes(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Tools.Types["getTime"] = "invalid"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected invalid tool type error")
	}
}
