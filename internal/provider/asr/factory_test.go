package asr

import (
	"strings"
	"testing"
)

func TestNewRecognizerRejectsUnsupportedProvider(t *testing.T) {
	_, err := NewRecognizer(ProviderConfig{Type: "baidu"})
	if err == nil || !strings.Contains(err.Error(), "unsupported asr provider") {
		t.Fatalf("expected unsupported asr provider error, got %v", err)
	}
}
