package audio

import (
	"github.com/streamer45/silero-vad-go/speech"
)

// sileroVAD is a voice activity detector based on Silero VAD model
type sileroVAD struct {
	detector *speech.Detector
}

// NewSileroVAD creates a new Silero VAD detector
func NewSileroVAD(modelPath string, sampleRate int, threshold float64, minSilenceMs int, speechPadMs int) (VADDetector, error) {
	// Ensure model is downloaded
	if err := ensureVADModel(modelPath); err != nil {
		return nil, err
	}

	cfg := speech.DetectorConfig{
		ModelPath:            modelPath,
		SampleRate:           sampleRate,
		Threshold:            float32(threshold),
		MinSilenceDurationMs: minSilenceMs,
		SpeechPadMs:          speechPadMs,
	}

	detector, err := speech.NewDetector(cfg)
	if err != nil {
		return nil, err
	}

	return &sileroVAD{
		detector: detector,
	}, nil
}

func (v *sileroVAD) Detect(audio []byte) (bool, error) {
	if len(audio) < 2 {
		return false, nil
	}

	floats := Int16PCMToFloat32(audio)

	// Detect speech segments in the audio chunk
	segments, err := v.detector.Detect(floats)
	if err != nil {
		return false, err
	}

	// If we got any segments with speech, return true
	for _, seg := range segments {
		if seg.SpeechEndAt > 0 {
			// Speech segment with end time - speech was detected
			return true, nil
		}
	}

	return false, nil
}

func (v *sileroVAD) Close() error {
	return v.detector.Destroy()
}
