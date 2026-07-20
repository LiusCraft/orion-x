package messages

import (
	"encoding/json"
	"testing"

	"github.com/liuscraft/orion-x/internal/llm"
)

func TestThinkingUsesConfiguredBudget(t *testing.T) {
	budget := 4096
	a := &adapter{cfg: Config{Model: "claude-sonnet-4-5", Thinking: llm.ThinkingConfig{Mode: llm.ThinkingModeEnabled, BudgetTokens: &budget}}}
	params, err := a.params(llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" || thinking["budget_tokens"] != float64(budget) {
		t.Fatalf("thinking = %#v; body=%s", body["thinking"], data)
	}
}
