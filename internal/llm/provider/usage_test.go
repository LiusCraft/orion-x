package provider

import "testing"

type wantUsage struct {
	input, output, reasoning, cacheRead, cacheWrite, total int64
}

func TestNormalizeUsage(t *testing.T) {
	tests := []struct {
		name  string
		parts UsageParts
		want  wantUsage
	}{
		{
			name:  "no details",
			parts: UsageParts{InputTokens: 100, OutputTokens: 20, TotalTokens: 120},
			want:  wantUsage{input: 100, output: 20, total: 120},
		},
		{
			name:  "cached only",
			parts: UsageParts{InputTokens: 100, OutputTokens: 20, TotalTokens: 120, CacheReadTokens: 60},
			want:  wantUsage{input: 40, output: 20, cacheRead: 60, total: 120},
		},
		{
			name:  "reasoning only",
			parts: UsageParts{InputTokens: 100, OutputTokens: 50, TotalTokens: 150, ReasoningTokens: 30},
			want:  wantUsage{input: 100, output: 20, reasoning: 30, total: 150},
		},
		{
			name: "cache and reasoning",
			parts: UsageParts{
				InputTokens: 1000, OutputTokens: 200, TotalTokens: 1200,
				CacheReadTokens: 400, CacheWriteTokens: 50, ReasoningTokens: 150,
			},
			want: wantUsage{input: 550, output: 50, reasoning: 150, cacheRead: 400, cacheWrite: 50, total: 1200},
		},
		{
			// 兼容厂商给出“子集比父集还大”的脏数据时必须夹在 0，不能出负数。
			name: "details larger than parent clamp at zero",
			parts: UsageParts{
				InputTokens: 10, OutputTokens: 5, TotalTokens: 15,
				CacheReadTokens: 40, CacheWriteTokens: 40, ReasoningTokens: 99,
			},
			want: wantUsage{reasoning: 99, cacheRead: 40, cacheWrite: 40, total: 15},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeUsage(tt.parts)
			if got.InputTokens != tt.want.input {
				t.Errorf("InputTokens = %d, want %d", got.InputTokens, tt.want.input)
			}
			if got.OutputTokens != tt.want.output {
				t.Errorf("OutputTokens = %d, want %d", got.OutputTokens, tt.want.output)
			}
			if got.ReasoningTokens != tt.want.reasoning {
				t.Errorf("ReasoningTokens = %d, want %d", got.ReasoningTokens, tt.want.reasoning)
			}
			if got.CacheReadTokens != tt.want.cacheRead {
				t.Errorf("CacheReadTokens = %d, want %d", got.CacheReadTokens, tt.want.cacheRead)
			}
			if got.CacheWriteTokens != tt.want.cacheWrite {
				t.Errorf("CacheWriteTokens = %d, want %d", got.CacheWriteTokens, tt.want.cacheWrite)
			}
			if got.TotalTokens != tt.want.total {
				t.Errorf("TotalTokens = %d, want %d", got.TotalTokens, tt.want.total)
			}
		})
	}
}
