package audio

import "testing"

func TestPCM16Level(t *testing.T) {
	tests := []struct {
		name         string
		audio        []byte
		wantRMS      float64
		wantPeak     float64
		wantClipping float64
		allowedDelta float64
	}{
		{
			name:         "empty",
			audio:        nil,
			wantRMS:      0,
			wantPeak:     0,
			wantClipping: 0,
			allowedDelta: 0,
		},
		{
			name:         "silence",
			audio:        makePCM(0, 160),
			wantRMS:      0,
			wantPeak:     0,
			wantClipping: 0,
			allowedDelta: 0,
		},
		{
			name:         "constant half scale",
			audio:        makePCM(16384, 160),
			wantRMS:      0.5,
			wantPeak:     0.5,
			wantClipping: 0,
			allowedDelta: 0.001,
		},
		{
			name:         "clipping",
			audio:        makePCM(32767, 160),
			wantRMS:      0.999,
			wantPeak:     0.999,
			wantClipping: 1,
			allowedDelta: 0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rms, peak, clipping := PCM16Level(tt.audio)
			assertFloatNear(t, "rms", tt.wantRMS, rms, tt.allowedDelta)
			assertFloatNear(t, "peak", tt.wantPeak, peak, tt.allowedDelta)
			assertFloatNear(t, "clipping", tt.wantClipping, clipping, tt.allowedDelta)
		})
	}
}

func TestAudioLevelMonitorWithWAVFixtures(t *testing.T) {
	monitor := NewAudioLevelMonitor(16000)

	silence := monitor.Observe(makePCM(0, 1600))
	if !silence.Silent {
		t.Fatal("expected silence to be marked silent")
	}
	if silence.Clipping {
		t.Fatal("expected silence to not clip")
	}

	voice := readPCM16MonoWAV(t, "../../testdata/human_voice.wav", 16000)
	voiceLevel := monitor.Observe(voice)
	if voiceLevel.Silent {
		t.Fatal("expected human voice fixture to not be silent")
	}
	if voiceLevel.RMS <= 0 {
		t.Fatal("expected human voice fixture to have positive RMS")
	}
	if voiceLevel.Peak <= voiceLevel.RMS {
		t.Fatalf("expected peak to be above RMS, peak=%.4f rms=%.4f", voiceLevel.Peak, voiceLevel.RMS)
	}

	noise := readPCM16MonoWAV(t, "../../testdata/noise_3s_16k.wav", 16000)
	noiseLevel := monitor.Observe(noise)
	if noiseLevel.Silent {
		t.Fatal("expected noise fixture to not be silent")
	}
	if noiseLevel.RMS <= voiceLevel.RMS {
		t.Fatalf("expected test noise RMS to be above voice RMS, noise=%.4f voice=%.4f", noiseLevel.RMS, voiceLevel.RMS)
	}
	if !noiseLevel.AboveNoiseFloor {
		t.Fatal("expected high-energy noise fixture to be above noise floor")
	}

	clipped := monitor.Observe(makePCM(32767, 1600))
	if !clipped.Clipping {
		t.Fatal("expected full-scale PCM to be reported as clipped")
	}
}

func assertFloatNear(t *testing.T, name string, want, got, delta float64) {
	t.Helper()
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	if diff > delta {
		t.Fatalf("%s: expected %.4f, got %.4f", name, want, got)
	}
}
