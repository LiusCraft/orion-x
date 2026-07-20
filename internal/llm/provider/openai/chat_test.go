package openai

import (
	"encoding/json"
	"testing"

	"github.com/liuscraft/orion-x/internal/llm"
	openaisdk "github.com/openai/openai-go/v3"
)

func TestApplyDialectThinking(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		thinking llm.ThinkingConfig
		want     map[string]any
	}{
		{name: "minimax disabled", model: "MiniMax-M3", thinking: llm.ThinkingConfig{Mode: llm.ThinkingModeDisabled}, want: map[string]any{"thinking": map[string]any{"type": "disabled"}}},
		{name: "deepseek enabled", model: "deepseek-chat", thinking: llm.ThinkingConfig{Mode: llm.ThinkingModeEnabled}, want: map[string]any{"thinking": map[string]any{"type": "enabled"}}},
		{name: "qwen budget", model: "qwen-plus", thinking: llm.ThinkingConfig{Mode: llm.ThinkingModeEnabled, BudgetTokens: intPtr(2048), PreserveHistory: llm.PreserveModeAll}, want: map[string]any{"enable_thinking": true, "thinking_budget": float64(2048), "preserve_thinking": true}},
		{name: "kimi k2.6 keep", model: "kimi-k2.6", thinking: llm.ThinkingConfig{Mode: llm.ThinkingModeEnabled, PreserveHistory: llm.PreserveModeAll}, want: map[string]any{"thinking": map[string]any{"type": "enabled", "keep": "all"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := openaisdk.ChatCompletionNewParams{Model: "model"}
			if err := applyDialect(&params, tt.model, tt.thinking, nil); err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			data, err := json.Marshal(params)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			for key, want := range tt.want {
				if !jsonEqual(got[key], want) {
					t.Fatalf("%s = %#v, want %#v; body=%s", key, got[key], want, data)
				}
			}
		})
	}
}

func TestMiniMaxM2CannotDisableThinking(t *testing.T) {
	params := openaisdk.ChatCompletionNewParams{Model: "model"}
	err := applyDialect(&params, "MiniMaxAI/MiniMax-M2.5", llm.ThinkingConfig{Mode: llm.ThinkingModeDisabled}, nil)
	if err == nil {
		t.Fatal("expected unsupported option error")
	}
}

func TestGenericModelDoesNotReceiveThinkingFields(t *testing.T) {
	params := openaisdk.ChatCompletionNewParams{Model: "model"}
	if err := applyDialect(&params, "gpt-5", llm.ThinkingConfig{Mode: llm.ThinkingModeEnabled}, nil); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["thinking"]; ok {
		t.Fatalf("generic model received thinking field: %s", data)
	}
}

func intPtr(v int) *int { return &v }

func jsonEqual(a, b any) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}
