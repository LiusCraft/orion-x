package audio

import (
	"math"
	"time"
)

const (
	defaultSilenceRMS          = 0.003
	defaultClippingSampleLevel = 0.98
	defaultClippingRatio       = 0.01
	defaultHighNoiseFloor      = 0.05
	noiseFloorAlpha            = 0.05
)

// AudioLevelSnapshot describes the energy and health of one PCM audio chunk.
type AudioLevelSnapshot struct {
	RMS             float64
	Peak            float64
	NoiseFloor      float64
	ClippingRatio   float64
	SilenceDuration time.Duration
	Silent          bool
	AboveNoiseFloor bool
	Clipping        bool
	Noisy           bool
}

// AudioLevelMonitor tracks audio energy for pre-VAD gating, diagnostics, and health checks.
type AudioLevelMonitor struct {
	sampleRate int

	noiseFloor     float64
	silenceStarted time.Time
}

func NewAudioLevelMonitor(sampleRate int) *AudioLevelMonitor {
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	return &AudioLevelMonitor{sampleRate: sampleRate}
}

func (m *AudioLevelMonitor) Observe(audio []byte) AudioLevelSnapshot {
	rms, peak, clippingRatio := PCM16Level(audio)
	now := time.Now()

	if m.noiseFloor == 0 {
		m.noiseFloor = rms
	} else if rms <= m.noiseFloor*1.5 || rms <= defaultHighNoiseFloor {
		m.noiseFloor = m.noiseFloor*(1-noiseFloorAlpha) + rms*noiseFloorAlpha
	}

	silent := rms <= defaultSilenceRMS
	if silent {
		if m.silenceStarted.IsZero() {
			m.silenceStarted = now
		}
	} else {
		m.silenceStarted = time.Time{}
	}

	var silenceDuration time.Duration
	if !m.silenceStarted.IsZero() {
		silenceDuration = now.Sub(m.silenceStarted)
	}

	noiseFloor := m.noiseFloor
	aboveNoiseFloor := false
	if noiseFloor > 0 {
		aboveNoiseFloor = rms >= maxFloat(defaultSilenceRMS, noiseFloor*2.5)
	}

	return AudioLevelSnapshot{
		RMS:             rms,
		Peak:            peak,
		NoiseFloor:      noiseFloor,
		ClippingRatio:   clippingRatio,
		SilenceDuration: silenceDuration,
		Silent:          silent,
		AboveNoiseFloor: aboveNoiseFloor,
		Clipping:        clippingRatio >= defaultClippingRatio,
		Noisy:           noiseFloor >= defaultHighNoiseFloor,
	}
}

func PCM16Level(audio []byte) (rms float64, peak float64, clippingRatio float64) {
	if len(audio) < 2 {
		return 0, 0, 0
	}

	count := len(audio) / 2
	sum := 0.0
	clipping := 0

	for i := 0; i < count; i++ {
		lo := audio[i*2]
		hi := audio[i*2+1]
		sample := int16(lo) | int16(hi)<<8
		value := float64(sample) / 32768.0
		absValue := math.Abs(value)
		sum += value * value
		if absValue > peak {
			peak = absValue
		}
		if absValue >= defaultClippingSampleLevel {
			clipping++
		}
	}

	return math.Sqrt(sum / float64(count)), peak, float64(clipping) / float64(count)
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
