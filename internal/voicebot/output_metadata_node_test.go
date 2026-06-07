package voicebot

import "testing"

func TestOutputMetadataNodeProcess(t *testing.T) {
	node := NewOutputMetadataNode()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "known emotion", input: "你好 [EMO:happy]", expected: "happy"},
		{name: "unknown emotion", input: "你好 [EMO:unknown]", expected: ""},
		{name: "no emotion", input: "你好", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := node.Process(tt.input)
			if got.Emotion != tt.expected {
				t.Fatalf("Emotion = %q, want %q", got.Emotion, tt.expected)
			}
		})
	}
}
