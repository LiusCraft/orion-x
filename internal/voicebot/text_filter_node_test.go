package voicebot

import "testing"

func TestTextFilterNodeProcess(t *testing.T) {
	node := NewTextFilterNode()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "markdown and emotion tag",
			input:    "**你好** [EMO:happy]",
			expected: "你好",
		},
		{
			name:     "link and emotion tag",
			input:    "请访问[官网](https://example.com) [EMO:calm]",
			expected: "请访问官网",
		},
		{
			name:     "unknown tag remains",
			input:    "你好 [EMO:unknown]",
			expected: "你好 [EMO:unknown]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := node.Process(tt.input)
			if got != tt.expected {
				t.Errorf("Process() = %q, want %q", got, tt.expected)
			}
		})
	}
}
