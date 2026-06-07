package audio

import "errors"

// VADType represents the type of VAD detector
type VADType string

const (
	VADTypeSilero VADType = "silero"
)

// DefaultVADModelPath is the default path for the Silero VAD model
const DefaultVADModelPath = "models/silero_vad.onnx"

// VADDetector is the interface for voice activity detection
type VADDetector interface {
	// Detect processes audio and returns true if speech is detected
	// audio is int16 PCM (little-endian)
	Detect(audio []byte) (bool, error)
	// Close releases resources
	Close() error
}

// noopVAD is a VAD detector that always returns false
type noopVAD struct{}

func (v *noopVAD) Detect([]byte) (bool, error) { return false, nil }
func (v *noopVAD) Close() error                { return nil }

// NewVADDetector creates a VAD detector based on config
func NewVADDetector(cfg *InPipeConfig) (VADDetector, error) {
	if cfg == nil || !cfg.EnableVAD {
		return &noopVAD{}, nil
	}

	vadType := VADType(cfg.VADType)
	if vadType == "" {
		vadType = VADTypeSilero
	}

	switch vadType {
	case VADTypeSilero:
		modelPath := cfg.VADModelPath
		if modelPath == "" {
			modelPath = DefaultVADModelPath
		}
		minSilenceMs := cfg.VADMinSilenceMs
		if minSilenceMs <= 0 {
			minSilenceMs = 500
		}
		speechPadMs := cfg.VADSpeechPadMs
		if speechPadMs <= 0 {
			speechPadMs = 300
		}
		threshold := cfg.VADThreshold
		if threshold <= 0 {
			threshold = 0.5
		}
		return NewSileroVAD(modelPath, cfg.SampleRate, threshold, minSilenceMs, speechPadMs)
	default:
		return nil, ErrUnsupportedVADType
	}
}

// ErrVADModelNotFound is returned when the VAD model file is not found
var ErrVADModelNotFound = errors.New("vad model not found")

// ErrUnsupportedVADType is returned when the configured detector is not a speech VAD.
var ErrUnsupportedVADType = errors.New("unsupported vad type")
