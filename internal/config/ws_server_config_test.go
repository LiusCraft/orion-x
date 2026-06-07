package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWSServer_DefaultFallback(t *testing.T) {
	t.Setenv("DASHSCOPE_API_KEY", "dash-key")
	t.Setenv("ZHIPU_API_KEY", "zhipu-key")

	cfg, err := LoadWSServer(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("LoadWSServer() error = %v", err)
	}

	resolved, profileID, ok := cfg.ResolveVoicebotForDevice("device-a")
	if !ok {
		t.Fatalf("expected default voicebot config")
	}
	if profileID != "default" {
		t.Fatalf("expected default profile, got %q", profileID)
	}
	if resolved.Provider.ASR.Aliyun.APIKey != "dash-key" {
		t.Fatalf("expected ASR key from env, got %q", resolved.Provider.ASR.Aliyun.APIKey)
	}
	if resolved.Provider.LLM.OpenAI.APIKey != "zhipu-key" {
		t.Fatalf("expected LLM key from env, got %q", resolved.Provider.LLM.OpenAI.APIKey)
	}
}

func TestLoadWSServer_DefaultDisabledRequiresBinding(t *testing.T) {
	t.Setenv("DASHSCOPE_API_KEY", "dash-key")
	t.Setenv("ZHIPU_API_KEY", "zhipu-key")

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "ws-server.json")
	data := `{
		"voicebot": {
			"default": null,
			"profiles": {
				"bot-a": {
					"provider": {
						"tts": {
							"aliyun": {"voice": "zhichu"}
						}
					}
				}
			},
			"local_bindings": {
				"dev-1": "bot-a"
			}
		}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadWSServer(path)
	if err != nil {
		t.Fatalf("LoadWSServer() error = %v", err)
	}

	_, _, ok := cfg.ResolveVoicebotForDevice("dev-unknown")
	if ok {
		t.Fatalf("expected unresolved device when default is disabled")
	}

	resolved, profileID, ok := cfg.ResolveVoicebotForDevice("dev-1")
	if !ok {
		t.Fatalf("expected bound device to resolve")
	}
	if profileID != "bot-a" {
		t.Fatalf("expected profile bot-a, got %q", profileID)
	}
	if resolved.Provider.TTS.Aliyun.Voice != "zhichu" {
		t.Fatalf("expected profile voice override, got %q", resolved.Provider.TTS.Aliyun.Voice)
	}
}

func TestLoadWSServer_InvalidBindingProfile(t *testing.T) {
	t.Setenv("DASHSCOPE_API_KEY", "dash-key")
	t.Setenv("ZHIPU_API_KEY", "zhipu-key")

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "ws-server.json")
	data := `{
		"voicebot": {
			"profiles": {
				"bot-a": {}
			},
			"local_bindings": {
				"dev-1": "bot-b"
			}
		}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadWSServer(path); err == nil {
		t.Fatalf("expected validation error for unknown binding profile")
	}
}
