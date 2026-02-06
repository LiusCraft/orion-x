package sink

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/liuscraft/orion-x/internal/audio"
)

type fakeSender struct {
	last []byte
}

func (f *fakeSender) SendBinary(data []byte) error {
	f.last = make([]byte, len(data))
	copy(f.last, data)
	return nil
}

func TestWebSocketSinkWritePCM(t *testing.T) {
	sender := &fakeSender{}
	sink := NewWebSocketSink(sender, WebSocketSinkConfig{
		Format:          "pcm",
		SampleRate:      16000,
		Channels:        1,
		FrameDurationMs: 60,
	})

	format := audio.AudioFormat{
		SampleRate:      16000,
		Channels:        1,
		FramesPerBuffer: 4,
	}

	if err := sink.Start(context.Background(), format); err != nil {
		t.Fatalf("start sink failed: %v", err)
	}

	samples := []int16{1, -2, 3, -4}
	if err := sink.WritePCM(samples); err != nil {
		t.Fatalf("write pcm failed: %v", err)
	}

	if len(sender.last) != len(samples)*2 {
		t.Fatalf("unexpected payload length: %d", len(sender.last))
	}

	if got := int16(binary.LittleEndian.Uint16(sender.last[0:])); got != samples[0] {
		t.Fatalf("sample mismatch: got %d want %d", got, samples[0])
	}
}
