package realtime

import (
	"encoding/binary"
	"testing"
)

func TestPCM16Resample(t *testing.T) {
	input := make([]byte, 4)
	binary.LittleEndian.PutUint16(input[0:2], 1000)
	binary.LittleEndian.PutUint16(input[2:4], 2000)

	output, err := pcm16Resample(input, 16000, 24000)
	if err != nil {
		t.Fatalf("pcm16Resample() error = %v", err)
	}
	if len(output) != 6 {
		t.Fatalf("len(output) = %d, want 6", len(output))
	}
}

func TestPCM16ResampleRejectsOddBytes(t *testing.T) {
	if _, err := pcm16Resample([]byte{1}, 24000, 16000); err == nil {
		t.Fatal("pcm16Resample() error = nil, want odd-byte error")
	}
}

func TestDefaultSessionConfig(t *testing.T) {
	cfg := defaultSessionConfig()
	if cfg.Type != "realtime" {
		t.Errorf("cfg.Type = %q, want realtime", cfg.Type)
	}
	if cfg.Audio == nil || cfg.Audio.Input == nil || cfg.Audio.Input.Format.Rate != 24000 {
		t.Fatalf("cfg.Audio.Input.Format = %#v, want 24000 Hz", cfg.Audio)
	}
	if cfg.Audio.Input.TurnDetection == nil || cfg.Audio.Input.TurnDetection.Type != "server_vad" {
		t.Fatalf("turn detection = %#v, want server_vad", cfg.Audio.Input.TurnDetection)
	}
}
