package audio

import (
	"bytes"
	"context"
	"testing"
	"time"
)

type stubSource struct {
	data []byte
}

func (s *stubSource) Read(ctx context.Context) ([]byte, error) {
	return append([]byte(nil), s.data...), nil
}

func (s *stubSource) Close() error {
	return nil
}

type stubReference struct {
	active bool
	frame  []byte
}

func (r *stubReference) ReadReference() []byte {
	return append([]byte(nil), r.frame...)
}

func (r *stubReference) IsActive() bool {
	return r.active
}

func TestReferenceBuffer_DelayAndActive(t *testing.T) {
	buf := NewReferenceBuffer(4, 4, 1)
	buf.SetActiveWindow(50 * time.Millisecond)
	buf.WriteReference([]byte{0x01, 0x02, 0x03, 0x04})
	if !buf.IsActive() {
		t.Fatalf("expected reference buffer to be active after write")
	}

	buf.WriteReference([]byte{0x05, 0x06, 0x07, 0x08})
	frame := buf.ReadReference()
	if !bytes.Equal(frame, []byte{0x01, 0x02, 0x03, 0x04}) {
		t.Fatalf("unexpected frame: %v", frame)
	}

	time.Sleep(60 * time.Millisecond)
	if buf.IsActive() {
		t.Fatalf("expected reference buffer to be inactive after window")
	}
}

func TestEchoCancellingSource_Gate(t *testing.T) {
	frameBytes := FrameBytes(16000, 1, 10)
	data := bytes.Repeat([]byte{0x10}, frameBytes)
	source := &stubSource{data: data}
	ref := &stubReference{active: true, frame: make([]byte, frameBytes)}
	cfg := EchoCancelConfig{Enabled: true, Mode: "gate", FrameMs: 10}

	wrapped := NewEchoCancellingSource(source, cfg, ref, NewNoopEchoCanceller(), 16000, 1)
	out, err := wrapped.Read(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bytes.Equal(out, data) {
		t.Fatalf("expected gated output to be silent")
	}
	if !bytes.Equal(out, make([]byte, len(data))) {
		t.Fatalf("expected silent output, got %v", out)
	}
}

func TestEchoCancellingSource_PassThrough(t *testing.T) {
	frameBytes := FrameBytes(16000, 1, 10)
	data := bytes.Repeat([]byte{0x22}, frameBytes)
	source := &stubSource{data: data}
	ref := &stubReference{active: false, frame: make([]byte, frameBytes)}
	cfg := EchoCancelConfig{Enabled: true, Mode: "aec", FrameMs: 10}

	wrapped := NewEchoCancellingSource(source, cfg, ref, NewNoopEchoCanceller(), 16000, 1)
	out, err := wrapped.Read(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(out, data) {
		t.Fatalf("expected passthrough output")
	}
}
