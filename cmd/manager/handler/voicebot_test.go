package handler

import (
	"encoding/json"
	"testing"

	"github.com/liuscraft/orion-x/internal/config"
)

func TestNormalizeCreateConfigEmpty(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"empty string", ""},
		{"null", "null"},
		{"empty object", "{}"},
		{"whitespace object", "  {  }  "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeCreateConfig(json.RawMessage(tc.raw))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want, _ := json.Marshal(config.DefaultConfig())
			if got != string(want) {
				t.Fatalf("got %s, want default config", got)
			}
		})
	}
}

func TestNormalizeCreateConfigInvalidJSON(t *testing.T) {
	if _, err := normalizeCreateConfig(json.RawMessage(`{bad`)); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestNormalizeCreateConfigPartialAgentConfig(t *testing.T) {
	raw := `{"llm":{"soul_prompt":"你好"}}`
	got, err := normalizeCreateConfig(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != raw {
		t.Fatalf("partial config must be preserved: got %s", got)
	}
}

func TestNormalizeCreateConfigStringForm(t *testing.T) {
	// 前端 create 历史行为：config_json 作为 JSON 字符串发送（外层带引号）
	raw := `"{\"llm\":{\"soul_prompt\":\"你好\"}}"`
	got, err := normalizeCreateConfig(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `{"llm":{"soul_prompt":"你好"}}`
	if got != want {
		t.Fatalf("string form must be unwrapped: got %s, want %s", got, want)
	}
}

func TestNormalizeCreateConfigStringFormEmpty(t *testing.T) {
	got, err := normalizeCreateConfig(json.RawMessage(`"{}"`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want, _ := json.Marshal(config.DefaultConfig())
	if got != string(want) {
		t.Fatalf("got %s, want default config", got)
	}
}

func TestNormalizeCreateConfigStringFormInvalid(t *testing.T) {
	if _, err := normalizeCreateConfig(json.RawMessage(`"not json"`)); err == nil {
		t.Fatal("expected error for invalid JSON string form")
	}
}
