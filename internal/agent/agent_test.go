package agent

import (
	"errors"
	"testing"
)

func TestNormalizeConfigRequiresLLMEndpointAndModel(t *testing.T) {
	_, err := normalizeConfig(Config{APIKey: "test-key", Model: "model"})
	if !errors.Is(err, errLLMBaseURLRequired) {
		t.Fatalf("expected base URL error, got %v", err)
	}

	_, err = normalizeConfig(Config{APIKey: "test-key", BaseURL: "https://example.com"})
	if !errors.Is(err, errLLMModelRequired) {
		t.Fatalf("expected model error, got %v", err)
	}
}
