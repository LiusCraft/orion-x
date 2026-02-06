package wsserver

import "testing"

func TestNormalizeAudioParams(t *testing.T) {
	defaults := AudioParams{
		Format:               "opus",
		SampleRate:           16000,
		Channels:             1,
		FrameDuration:        60,
		BitsPerSample:        16,
		PlayBufferDurationMs: 300,
	}

	got := NormalizeAudioParams(nil, defaults)
	if got.Format != "opus" || got.SampleRate != 16000 || got.FrameDuration != 60 {
		t.Fatalf("unexpected defaults: %+v", got)
	}

	custom := &AudioParams{
		Format:               "pcm",
		SampleRate:           16000,
		Channels:             2,
		FrameDuration:        40,
		BitsPerSample:        24,
		PlayBufferDurationMs: 500,
	}
	got = NormalizeAudioParams(custom, defaults)
	if got.Format != "pcm" || got.Channels != 2 || got.FrameDuration != 40 {
		t.Fatalf("unexpected custom params: %+v", got)
	}

	invalid := &AudioParams{
		Format:               "aac",
		SampleRate:           8000,
		Channels:             3,
		FrameDuration:        10,
		BitsPerSample:        8,
		PlayBufferDurationMs: 50,
	}
	got = NormalizeAudioParams(invalid, defaults)
	if got.Format != defaults.Format || got.SampleRate != defaults.SampleRate || got.PlayBufferDurationMs != defaults.PlayBufferDurationMs {
		t.Fatalf("invalid params should fallback to defaults: %+v", got)
	}
}

func TestValidateAudioParams(t *testing.T) {
	valid := AudioParams{
		Format:               "opus",
		SampleRate:           16000,
		Channels:             1,
		FrameDuration:        60,
		BitsPerSample:        16,
		PlayBufferDurationMs: 300,
	}
	if err := ValidateAudioParams(valid); err != nil {
		t.Fatalf("expected valid params, got error: %v", err)
	}

	invalid := valid
	invalid.SampleRate = 8000
	if err := ValidateAudioParams(invalid); err == nil {
		t.Fatal("expected error for invalid sample rate")
	}
}
