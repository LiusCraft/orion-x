//go:build opus

package sink

import (
	"context"
	"testing"

	"github.com/liuscraft/orion-x/internal/audio"
)

type countingSender struct {
	count int
}

func (c *countingSender) SendBinary(data []byte) error {
	if len(data) > 0 {
		c.count++
	}
	return nil
}

func TestWebSocketSinkWriteOpus(t *testing.T) {
	sender := &countingSender{}
	sink := NewWebSocketSink(sender, WebSocketSinkConfig{
		Format:          "opus",
		SampleRate:      16000,
		Channels:        1,
		FrameDurationMs: 20,
	})

	format := audio.AudioFormat{
		SampleRate:      16000,
		Channels:        1,
		FramesPerBuffer: 320,
	}

	if err := sink.Start(context.Background(), format); err != nil {
		t.Fatalf("start sink failed: %v", err)
	}

	sink.SetSendSilence(true)
	samples := make([]int16, 320)
	if err := sink.WritePCM(samples); err != nil {
		t.Fatalf("write opus failed: %v", err)
	}
	if sender.count == 0 {
		t.Fatal("expected opus packet to be sent")
	}
}
