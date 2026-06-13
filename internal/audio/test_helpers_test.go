package audio

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"testing"
)

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
