package wsserver

import (
	"testing"
	"time"
)

func TestClientSupportsInterimSTT(t *testing.T) {
	tests := []struct {
		name     string
		features map[string]any
		expected bool
	}{
		{
			name:     "empty",
			features: nil,
			expected: false,
		},
		{
			name:     "top level bool",
			features: map[string]any{helloFeatureInterimSTT: true},
			expected: true,
		},
		{
			name:     "top level string true",
			features: map[string]any{helloFeatureInterimSTT: "true"},
			expected: true,
		},
		{
			name: "nested stt interim",
			features: map[string]any{
				helloFeatureInterimSTTGroup: map[string]any{"interim": true},
			},
			expected: true,
		},
		{
			name: "nested stt string false",
			features: map[string]any{
				helloFeatureInterimSTTGroup: map[string]any{"interim": "false"},
			},
			expected: false,
		},
		{
			name: "invalid nested payload",
			features: map[string]any{
				helloFeatureInterimSTTGroup: "enabled",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clientSupportsInterimSTT(tt.features); got != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestSessionShouldSendASRPartial(t *testing.T) {
	s := &Session{}
	s.applyClientFeatures(map[string]any{helloFeatureInterimSTT: true})

	base := time.Unix(100, 0)
	if !s.shouldSendASRPartial("hello", base) {
		t.Fatal("expected first partial to pass")
	}
	if s.shouldSendASRPartial("hello", base.Add(300*time.Millisecond)) {
		t.Fatal("expected duplicate partial to be suppressed")
	}
	if s.shouldSendASRPartial("hello there", base.Add(100*time.Millisecond)) {
		t.Fatal("expected throttled partial to be suppressed")
	}
	if !s.shouldSendASRPartial("hello there", base.Add(interimSTTThrottleInterval)) {
		t.Fatal("expected changed partial to pass after throttle interval")
	}
}

func TestSessionShouldSendASRPartialDisabledByDefault(t *testing.T) {
	s := &Session{}
	if s.shouldSendASRPartial("hello", time.Now()) {
		t.Fatal("expected partial STT to be disabled by default")
	}
}

func TestMarkASRFinalResetsPartialState(t *testing.T) {
	s := &Session{}
	s.applyClientFeatures(map[string]any{helloFeatureInterimSTT: true})

	now := time.Unix(200, 0)
	if !s.shouldSendASRPartial("same text", now) {
		t.Fatal("expected first partial to pass")
	}

	s.markASRFinal()
	if !s.shouldSendASRPartial("same text", now.Add(10*time.Millisecond)) {
		t.Fatal("expected partial tracker reset after final")
	}
}
