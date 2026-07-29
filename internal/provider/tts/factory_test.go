package tts

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestNewProviderUsesRegisteredProvider(t *testing.T) {
	Register("fake", func(cfg Config) (Provider, error) { return fakeProvider{}, nil }, ProviderMeta{})

	provider, err := NewProvider(ProviderConfig{Type: "fake"})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	if provider == nil {
		t.Fatalf("expected provider")
	}
}

func TestNewProviderRejectsUnsupportedProvider(t *testing.T) {
	_, err := NewProvider(ProviderConfig{Type: "huoshan"})
	if err == nil || !strings.Contains(err.Error(), "unsupported tts provider") {
		t.Fatalf("expected unsupported tts provider error, got %v", err)
	}
}

type fakeProvider struct{}

func (fakeProvider) Synthesize(_ context.Context, _ SynthesizeRequest) (*SynthesizeResult, error) {
	return &SynthesizeResult{Audio: io.NopCloser(strings.NewReader(""))}, nil
}
