package source

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestWebSocketSourceRead(t *testing.T) {
	s, err := NewWebSocketSource(nil)
	if err != nil {
		t.Fatalf("NewWebSocketSource failed: %v", err)
	}

	want := []byte{1, 2, 3, 4}
	if err := s.PushPCM(want); err != nil {
		t.Fatalf("PushPCM failed: %v", err)
	}

	got, err := s.Read(context.Background())
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected frame len: got=%d want=%d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("unexpected byte at %d: got=%d want=%d", i, got[i], want[i])
		}
	}
}

func TestWebSocketSourceReadCanceled(t *testing.T) {
	s, err := NewWebSocketSource(nil)
	if err != nil {
		t.Fatalf("NewWebSocketSource failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = s.Read(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestWebSocketSourceCloseUnblocksRead(t *testing.T) {
	s, err := NewWebSocketSource(nil)
	if err != nil {
		t.Fatalf("NewWebSocketSource failed: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, readErr := s.Read(context.Background())
		errCh <- readErr
	}()

	time.Sleep(10 * time.Millisecond)
	if err := s.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("expected EOF, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Read should return after Close")
	}

	if err := s.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

func TestWebSocketSourceQueueFull(t *testing.T) {
	s, err := NewWebSocketSource(&WebSocketSourceConfig{QueueSize: 1})
	if err != nil {
		t.Fatalf("NewWebSocketSource failed: %v", err)
	}

	if err := s.PushPCM([]byte{1, 2}); err != nil {
		t.Fatalf("first PushPCM failed: %v", err)
	}
	if err := s.PushPCM([]byte{3, 4}); !errors.Is(err, ErrWebSocketSourceQueueFull) {
		t.Fatalf("expected queue full error, got %v", err)
	}
}
