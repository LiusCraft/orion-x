package vad

import (
	"testing"
)

func TestInt16PCMToFloat32(t *testing.T) {
	tests := []struct {
		name     string
		pcm      []byte
		expected float32
		delta    float32
	}{
		{
			name:     "zero samples",
			pcm:      []byte{0, 0, 0, 0},
			expected: 0.0,
			delta:    0.001,
		},
		{
			name:     "positive 0.5",
			pcm:      []byte{0, 64}, // 16384 in little-endian int16 = 0.5
			expected: 0.5,
			delta:    0.001,
		},
		{
			name:     "negative -0.5",
			pcm:      []byte{0, 192}, // -16384 in little-endian int16 = -0.5
			expected: -0.5,
			delta:    0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Int16PCMToFloat32(tt.pcm)
			if len(result) != len(tt.pcm)/2 {
				t.Errorf("expected length %d, got %d", len(tt.pcm)/2, len(result))
			}
			for i, v := range result {
				diff := v - tt.expected
				if diff < 0 {
					diff = -diff
				}
				if diff > tt.delta {
					t.Errorf("at index %d: expected %.3f, got %.3f", i, tt.expected, v)
				}
			}
		})
	}
}

func TestNoopVAD(t *testing.T) {
	vad := &noopVAD{}

	buf := make([]byte, 100)
	for i := range buf {
		buf[i] = 0xFF
	}

	detected, err := vad.Detect(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detected {
		t.Error("expected noopVAD to always return false")
	}

	if err := vad.Close(); err != nil {
		t.Fatalf("unexpected error on close: %v", err)
	}
}
