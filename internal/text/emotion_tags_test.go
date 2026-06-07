package text

import "testing"

func TestRemoveEmotionTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "known tag",
			input:    "你好啊 [EMO:happy]",
			expected: "你好啊",
		},
		{
			name:     "unknown tag remains",
			input:    "你好 [EMO:unknown]",
			expected: "你好 [EMO:unknown]",
		},
		{
			name:     "trim spaces",
			input:    " [EMO:calm] ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RemoveEmotionTags(tt.input)
			if got != tt.expected {
				t.Errorf("RemoveEmotionTags() = %q, want %q", got, tt.expected)
			}
		})
	}
}
