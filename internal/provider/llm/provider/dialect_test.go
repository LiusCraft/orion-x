package provider

import "testing"

func TestInferDialect(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "deepseek", model: "deepseek-v4-flash", want: DialectDeepSeek},
		{name: "namespaced deepseek", model: "deepseek-ai/DeepSeek-R1", want: DialectDeepSeek},
		{name: "qwen", model: "Qwen/Qwen3-235B", want: DialectQwen},
		{name: "minimax", model: "MiniMax-M3", want: DialectMiniMax},
		{name: "kimi", model: "kimi-k2.6", want: DialectKimi},
		{name: "moonshot", model: "moonshot-v1-128k", want: DialectKimi},
		{name: "generic", model: "gpt-5", want: DialectGeneric},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InferDialect(tt.model); got != tt.want {
				t.Fatalf("InferDialect(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}
