package store

import (
	"encoding/json"
	"testing"
)

func TestDefaultTemplatesConfigs(t *testing.T) {
	tmpls := defaultTemplates()
	if len(tmpls) == 0 {
		t.Fatal("no default templates")
	}
	for _, tp := range tmpls {
		t.Run(tp.Name, func(t *testing.T) {
			if tp.ConfigJSON == nil {
				t.Fatal("template has nil config")
			}
			b, err := json.Marshal(tp.ConfigJSON)
			if err != nil {
				t.Fatalf("marshal config: %v", err)
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatalf("config is not a valid JSON object: %v", err)
			}
			if len(m) == 0 {
				t.Fatal("template config must not be empty")
			}
			llm, ok := m["llm"].(map[string]any)
			if !ok {
				t.Fatalf("config must contain llm section, got %v", m)
			}
			if llm["soul_prompt"] == nil && llm["rules_prompt"] == nil {
				t.Fatal("llm section must contain soul_prompt or rules_prompt")
			}
		})
	}
}
