package responses

import (
	"encoding/json"
	"testing"

	"github.com/liuscraft/orion-x/internal/llm"
)

func TestQwenDisabledThinkingUsesNoneEffort(t *testing.T) {
	a := &adapter{cfg: Config{Model: "qwen-plus"}}
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

// regression: input must be populated — map→JSON→unmarshal round-trip used to drop the
// ResponseNewParams.Input union field, causing "Either input or instructions must be provided".
func TestParamsIncludesInputMessages(t *testing.T) {
	a := &adapter{cfg: Config{Model: "deepseek-reasoner"}}
	params, err := a.params(llm.Request{
		Messages: []llm.Message{
			{Role: "system", Content: "你是助手"},
			{Role: "user", Content: "你好"},
		},
	})
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
	raw, ok := body["input"]
	if !ok {
		t.Fatalf("input missing from body: %s", data)
	}
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("input not an array: %T", raw)
	}
	if len(items) != 2 {
		t.Fatalf("input len = %d, want 2; body=%s", len(items), data)
	}
}

func TestParamsIncludesTools(t *testing.T) {
	a := &adapter{cfg: Config{Model: "deepseek-reasoner"}}
	params, err := a.params(llm.Request{
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
		Tools: []llm.ToolDefinition{{
			Name:        "get_weather",
			Description: "查询天气",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		}},
	})
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
	raw, ok := body["tools"]
	if !ok {
		t.Fatalf("tools missing from body: %s", data)
	}
	tools, ok := raw.([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v; body=%s", raw, data)
	}
	first := tools[0].(map[string]any)
	if first["name"] != "get_weather" || first["type"] != "function" {
		t.Fatalf("tool = %#v; body=%s", first, data)
	}
}

func TestParamsRestoresProviderContextItems(t *testing.T) {
	pcData, _ := json.Marshal(map[string]any{"items": []any{
		map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "之前回复"}}},
		map[string]any{"type": "function_call", "call_id": "call_1", "name": "get_weather", "arguments": `{"city":"北京"}`},
		map[string]any{"type": "reasoning", "id": "rs_1", "summary": []any{}},
	}})
	a := &adapter{cfg: Config{Model: "deepseek-reasoner"}}
	params, err := a.params(llm.Request{
		Messages: []llm.Message{
			{Role: "user", Content: "北京天气"},
			{Role: "assistant", ProviderContext: &llm.ProviderContext{Adapter: "openai-responses", Data: pcData}},
		},
	})
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
	raw, ok := body["input"]
	if !ok {
		t.Fatalf("input missing from body: %s", data)
	}
	items := raw.([]any)
	if len(items) != 3 {
		t.Fatalf("input len = %d, want 3 (user, restored assistant, restored function_call); body=%s", len(items), data)
	}
	// reasoning item must be skipped; last item must be the function_call
	last := items[2].(map[string]any)
	if last["type"] != "function_call" || last["call_id"] != "call_1" {
		t.Fatalf("last input = %#v; body=%s", last, data)
	}
}

// regression: a tool-role message must be encoded as function_call_output, never as a
// role:"tool" message item (DeepSeek Responses rejects the `tool` role).
func TestParamsToolRoleEncodesAsFunctionCallOutput(t *testing.T) {
	a := &adapter{cfg: Config{Model: "deepseek-reasoner"}}
	params, err := a.params(llm.Request{
		Messages: []llm.Message{
			{Role: "user", Content: "现在几点"},
			{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "getTime", Arguments: `{}`}}},
			{Role: "tool", ToolCallID: "call_1", Content: `{"current":"2026-08-20"}`},
		},
	})
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
	raw, ok := body["input"]
	if !ok {
		t.Fatalf("input missing from body: %s", data)
	}
	items := raw.([]any)
	var outputs int
	for _, it := range items {
		m := it.(map[string]any)
		if m["role"] == "tool" {
			t.Fatalf("found role=tool message item: %#v; body=%s", m, data)
		}
		if m["type"] == "function_call_output" {
			outputs++
			if m["call_id"] != "call_1" || m["output"] != `{"current":"2026-08-20"}` {
				t.Fatalf("bad function_call_output: %#v; body=%s", m, data)
			}
		}
	}
	if outputs != 1 {
		t.Fatalf("function_call_output count = %d, want 1; body=%s", outputs, data)
	}
}
