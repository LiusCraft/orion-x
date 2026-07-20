package responses

import (
	"encoding/json"
	"testing"

	"github.com/liuscraft/orion-x/internal/llm"
)

func TestQwenDisabledThinkingUsesNoneEffort(t *testing.T) {
	a := &adapter{cfg: Config{Model: "qwen-plus", Dialect: "qwen"}}
	params, err := a.params(llm.Request{Thinking: llm.ThinkingConfig{Mode: llm.ThinkingModeDisabled}})
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
	reasoning, ok := body["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "none" {
		t.Fatalf("reasoning = %#v; body=%s", body["reasoning"], data)
	}
}
