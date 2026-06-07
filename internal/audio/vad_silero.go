package audio

import (
	"github.com/streamer45/silero-vad-go/speech"
)

type sileroVAD struct {
	inner      *speech.Detector
	windowSize int
	threshold  float32
	hyp        float32 // threshold - hysteresis, the floor for ending speech
	minSilenceSamples int

	triggered  bool
	tempEnd    int
	currSample int
}

func NewSileroVAD(modelPath string, sampleRate int, threshold float64, minSilenceMs int, speechPadMs int) (VADDetector, error) {
	if err := ensureVADModel(modelPath); err != nil {
		return nil, err
	}

	inner, err := speech.NewDetector(speech.DetectorConfig{
		ModelPath:  modelPath,
		SampleRate: sampleRate,
	})
	if err != nil {
		return nil, err
	}

	ws := 512
	if sampleRate == 8000 {
		ws = 256
	}

	thr := float32(threshold)
	hyp := thr - 0.15
	if hyp < 0 {
		hyp = 0
	}

	return &sileroVAD{
		inner:             inner,
		windowSize:        ws,
		threshold:         thr,
		hyp:               hyp,
		minSilenceSamples: minSilenceMs * sampleRate / 1000,
	}, nil
}

func (v *sileroVAD) Detect(audio []byte) (bool, error) {
	if len(audio) < 2 {
		return false, nil
	}

	floats := Int16PCMToFloat32(audio)
	ws := v.windowSize

	for i := 0; i <= len(floats)-ws; i += ws {
		prob, err := v.inner.Infer(floats[i : i+ws])
		if err != nil {
			return false, err
		}
		v.currSample += ws

		if prob >= v.threshold {
			v.tempEnd = 0
			if !v.triggered {
				v.triggered = true
			}
		} else if prob < v.hyp && v.triggered {
			if v.tempEnd == 0 {
				v.tempEnd = v.currSample
			}
			if v.currSample-v.tempEnd >= v.minSilenceSamples {
				v.tempEnd = 0
				v.triggered = false
			}
		}
	}

	return v.triggered, nil
}

func (v *sileroVAD) Close() error {
	return v.inner.Destroy()
}
