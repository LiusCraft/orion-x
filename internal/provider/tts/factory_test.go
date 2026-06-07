package tts

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestNewProviderUsesRegisteredProvider(t *testing.T) {
	Register("fake", func() Provider { return fakeProvider{} })

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

func (fakeProvider) Start(context.Context, Config) (Stream, error) {
	return fakeStream{}, nil
}

type fakeStream struct{}

func (fakeStream) WriteTextChunk(context.Context, string) error { return nil }
func (fakeStream) Close(context.Context) error                  { return nil }
func (fakeStream) AudioReader() io.ReadCloser                   { return nil }
func (fakeStream) SampleRate() int                              { return 16000 }
func (fakeStream) Channels() int                                { return 1 }
