package vad

import "errors"

// Type represents the type of VAD detector
type Type string

const (
	TypeSilero Type = "silero"
)

// DefaultModelPath is the default path for the Silero VAD model
const DefaultModelPath = "models/silero_vad.onnx"

// Detector is the interface for voice activity detection
type Detector interface {
	// Detect processes audio and returns true if speech is detected
	// audio is int16 PCM (little-endian)
	Detect(audio []byte) (bool, error)
	// Reset clears internal RNN state to prevent state drift over time.
	Reset()
	// Close releases resources
	Close() error
}

// noopVAD is a VAD detector that always returns false
type noopVAD struct{}

func (v *noopVAD) Detect([]byte) (bool, error) { return false, nil }
func (v *noopVAD) Reset()                      {}
func (v *noopVAD) Close() error                { return nil }

// ErrModelNotFound is returned when the VAD model file is not found
var ErrModelNotFound = errors.New("vad model not found")

// ErrUnsupportedType is returned when the configured detector is not a speech VAD.
var ErrUnsupportedType = errors.New("unsupported vad type")
