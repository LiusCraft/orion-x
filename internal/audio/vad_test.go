package audio

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
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

func TestSileroVADWithWAVFixtures(t *testing.T) {
	modelPath := filepath.Clean("../../models/silero_vad.onnx")
	if _, err := os.Stat(modelPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Skipf("Silero model not found at %s", modelPath)
		}
		t.Fatalf("stat Silero model: %v", err)
	}

	vad, err := NewSileroVAD(modelPath, 16000, 0.5, 200, 100)
	if err != nil {
		t.Fatalf("new Silero VAD: %v", err)
	}
	defer vad.Close()

	voice := readPCM16MonoWAV(t, "../../testdata/human_voice.wav", 16000)
	detected, err := vad.Detect(voice)
	if err != nil {
		t.Fatalf("detect voice: %v", err)
	}
	if !detected {
		t.Fatal("expected human voice fixture to trigger Silero VAD")
	}

	noise := readPCM16MonoWAV(t, "../../testdata/noise_3s_16k.wav", 16000)
	detected, err = vad.Detect(noise)
	if err != nil {
		t.Fatalf("detect noise: %v", err)
	}
	if detected {
		t.Fatal("expected noise fixture to not trigger Silero VAD")
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

func readPCM16MonoWAV(t *testing.T, path string, wantSampleRate uint32) []byte {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open wav fixture %s: %v", path, err)
	}
	defer f.Close()

	var riff [12]byte
	if _, err := io.ReadFull(f, riff[:]); err != nil {
		t.Fatalf("read wav header: %v", err)
	}
	if string(riff[0:4]) != "RIFF" || string(riff[8:12]) != "WAVE" {
		t.Fatalf("invalid wav header in %s", path)
	}

	var (
		gotFormat     uint16
		gotChannels   uint16
		gotSampleRate uint32
		gotBits       uint16
		data          []byte
	)

	for {
		var chunkHeader [8]byte
		_, err := io.ReadFull(f, chunkHeader[:])
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		if err != nil {
			t.Fatalf("read wav chunk header: %v", err)
		}

		chunkID := string(chunkHeader[0:4])
		chunkSize := binary.LittleEndian.Uint32(chunkHeader[4:8])

		switch chunkID {
		case "fmt ":
			chunk := make([]byte, chunkSize)
			if _, err := io.ReadFull(f, chunk); err != nil {
				t.Fatalf("read fmt chunk: %v", err)
			}
			if len(chunk) < 16 {
				t.Fatalf("invalid fmt chunk size %d", len(chunk))
			}
			gotFormat = binary.LittleEndian.Uint16(chunk[0:2])
			gotChannels = binary.LittleEndian.Uint16(chunk[2:4])
			gotSampleRate = binary.LittleEndian.Uint32(chunk[4:8])
			gotBits = binary.LittleEndian.Uint16(chunk[14:16])
		case "data":
			data = make([]byte, chunkSize)
			if _, err := io.ReadFull(f, data); err != nil {
				t.Fatalf("read data chunk: %v", err)
			}
		default:
			if _, err := f.Seek(int64(chunkSize), io.SeekCurrent); err != nil {
				t.Fatalf("skip %s chunk: %v", chunkID, err)
			}
		}

		if chunkSize%2 == 1 {
			if _, err := f.Seek(1, io.SeekCurrent); err != nil {
				t.Fatalf("skip wav padding byte: %v", err)
			}
		}
	}

	if gotFormat != 1 {
		t.Fatalf("expected PCM wav format 1, got %d", gotFormat)
	}
	if gotChannels != 1 {
		t.Fatalf("expected mono wav, got %d channels", gotChannels)
	}
	if gotSampleRate != wantSampleRate {
		t.Fatalf("expected sample rate %d, got %d", wantSampleRate, gotSampleRate)
	}
	if gotBits != 16 {
		t.Fatalf("expected 16-bit wav, got %d bits", gotBits)
	}
	if len(data) == 0 {
		t.Fatal("wav data chunk is empty")
	}
	return data
}

func TestNewVADDetector(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *InPipeConfig
		expectType  VADType
		expectError bool
	}{
		{
			name: "nil config returns noop",
			cfg:  nil,
		},
		{
			name: "disabled VAD returns noop",
			cfg: &InPipeConfig{
				EnableVAD: false,
			},
		},
		{
			name:        "RMS is not a VAD detector",
			expectError: true,
			cfg: &InPipeConfig{
				EnableVAD:    true,
				VADThreshold: 0.5,
				VADType:      "rms",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vad, err := NewVADDetector(tt.cfg)
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.expectError {
				return
			}
			if vad == nil {
				return
			}
			// Basic functionality test
			_, err = vad.Detect([]byte{0, 0, 0, 0})
			if err != nil {
				t.Errorf("unexpected error in Detect: %v", err)
			}
			vad.Close()
		})
	}
}
