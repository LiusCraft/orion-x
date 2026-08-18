//go:build cgo

package codec

import (
	"math"
	"testing"
)

func TestNew_PCM(t *testing.T) {
	c, err := New(FormatPCM, 16000, 1, 60)
	if err != nil {
		t.Fatalf("New(pcm) failed: %v", err)
	}
	if _, ok := c.(*pcmCodec); !ok {
		t.Fatalf("expected *pcmCodec, got %T", c)
	}
}

func TestNew_EmptyFormatDefaultsToPCM(t *testing.T) {
	c, err := New(Format(""), 16000, 1, 60)
	if err != nil {
		t.Fatalf("New(\"\") failed: %v", err)
	}
	if _, ok := c.(*pcmCodec); !ok {
		t.Fatalf("expected empty format to default to *pcmCodec, got %T", c)
	}
}

func TestNew_Opus(t *testing.T) {
	c, err := New(FormatOpus, 16000, 1, 60)
	if err != nil {
		t.Fatalf("New(opus) failed: %v", err)
	}
	if _, ok := c.(*opusCodec); !ok {
		t.Fatalf("expected *opusCodec, got %T", c)
	}
}

func TestNew_UnsupportedFormat(t *testing.T) {
	if _, err := New(Format("mp3"), 16000, 1, 60); err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestNew_OpusInvalidSampleRate(t *testing.T) {
	if _, err := New(FormatOpus, 22050, 1, 60); err == nil {
		t.Fatal("expected error for non-opus sample rate (22050)")
	}
}

func TestNew_OpusInvalidChannels(t *testing.T) {
	if _, err := New(FormatOpus, 16000, 3, 60); err == nil {
		t.Fatal("expected error for invalid channel count")
	}
}

// --- PCM codec ---

func TestPCMCodec_RoundTrip(t *testing.T) {
	c := newPCMCodec()

	in := []int16{1, -1, 32767, -32768, 0, 12345}
	frames, err := c.Encode(in)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}

	out, err := c.Decode(frames[0])
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("expected %d samples, got %d", len(in), len(out))
	}
	for i := range in {
		if in[i] != out[i] {
			t.Errorf("sample %d: want %d, got %d", i, in[i], out[i])
		}
	}
}

func TestPCMCodec_EmptyInputProducesNoFrame(t *testing.T) {
	c := newPCMCodec()
	frames, err := c.Encode(nil)
	if err != nil {
		t.Fatalf("Encode(nil) failed: %v", err)
	}
	if len(frames) != 0 {
		t.Fatalf("expected no frames for empty input, got %d", len(frames))
	}
}

func TestPCMCodec_FlushIsNoop(t *testing.T) {
	c := newPCMCodec()
	frames, err := c.Flush()
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	if len(frames) != 0 {
		t.Fatalf("expected Flush to be a no-op, got %d frames", len(frames))
	}
}

// --- Opus codec ---

// sineWave generates n samples of a sine wave, used as non-trivial test PCM.
func sineWave(n int, freqHz, sampleRate float64) []int16 {
	out := make([]int16, n)
	for i := range out {
		t := float64(i) / sampleRate
		out[i] = int16(8000 * math.Sin(2*math.Pi*freqHz*t))
	}
	return out
}

func TestOpusCodec_RoundTripPreservesSampleCount(t *testing.T) {
	c, err := newOpusCodec(16000, 1, 60)
	if err != nil {
		t.Fatalf("newOpusCodec failed: %v", err)
	}

	frameSize := 16000 * 60 / 1000 // 960 samples @ 16kHz/60ms mono
	in := sineWave(frameSize, 440, 16000)

	frames, err := c.Encode(in)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("expected exactly 1 frame for exactly-one-frame input, got %d", len(frames))
	}
	if len(frames[0]) == 0 {
		t.Fatal("expected non-empty encoded frame")
	}

	out, err := c.Decode(frames[0])
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if len(out) != frameSize {
		t.Fatalf("expected %d decoded samples, got %d", frameSize, len(out))
	}
}

func TestOpusCodec_BuffersPartialFrames(t *testing.T) {
	c, err := newOpusCodec(16000, 1, 60)
	if err != nil {
		t.Fatalf("newOpusCodec failed: %v", err)
	}

	frameSize := 16000 * 60 / 1000
	half := sineWave(frameSize/2, 440, 16000)

	// 第一次 Encode 只有半帧，不应该产生任何输出。
	frames, err := c.Encode(half)
	if err != nil {
		t.Fatalf("first Encode failed: %v", err)
	}
	if len(frames) != 0 {
		t.Fatalf("expected 0 frames from a half-frame input, got %d", len(frames))
	}

	// 第二次 Encode 补齐另一半，应该恰好产生 1 个完整帧。
	frames, err = c.Encode(half)
	if err != nil {
		t.Fatalf("second Encode failed: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("expected exactly 1 frame after completing a full frame, got %d", len(frames))
	}
}

func TestOpusCodec_MultipleFramesInOneCall(t *testing.T) {
	c, err := newOpusCodec(16000, 1, 60)
	if err != nil {
		t.Fatalf("newOpusCodec failed: %v", err)
	}

	frameSize := 16000 * 60 / 1000
	in := sineWave(frameSize*3, 440, 16000) // 正好 3 帧

	frames, err := c.Encode(in)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(frames))
	}
}

func TestOpusCodec_FlushEmitsPaddedTrailingFrame(t *testing.T) {
	c, err := newOpusCodec(16000, 1, 60)
	if err != nil {
		t.Fatalf("newOpusCodec failed: %v", err)
	}

	frameSize := 16000 * 60 / 1000
	partial := sineWave(frameSize/3, 440, 16000)

	if _, err := c.Encode(partial); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	frames, err := c.Flush()
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("expected Flush to emit exactly 1 padded frame, got %d", len(frames))
	}

	// Flush 之后内部缓冲应该清空：再次 Flush 不应产生新帧。
	frames, err = c.Flush()
	if err != nil {
		t.Fatalf("second Flush failed: %v", err)
	}
	if len(frames) != 0 {
		t.Fatalf("expected second Flush to be a no-op, got %d frames", len(frames))
	}
}

func TestOpusCodec_SupportedSampleRates(t *testing.T) {
	for _, rate := range []int{8000, 12000, 16000, 24000, 48000} {
		if _, err := newOpusCodec(rate, 1, 60); err != nil {
			t.Errorf("newOpusCodec(%d, 1) failed: %v", rate, err)
		}
	}
}

func TestOpusCodec_RejectsInvalidSampleRate(t *testing.T) {
	for _, rate := range []int{22050, 44100, 11025} {
		if _, err := newOpusCodec(rate, 1, 60); err == nil {
			t.Errorf("expected newOpusCodec(%d, 1) to fail", rate)
		}
	}
}
