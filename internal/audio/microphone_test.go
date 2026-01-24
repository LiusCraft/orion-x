package audio

import (
	"context"
	"errors"
	"testing"
	"time"
)

type blockingStream struct {
	readStarted chan struct{}
	abortCalled chan struct{}
}

func newBlockingStream() *blockingStream {
	return &blockingStream{
		readStarted: make(chan struct{}),
		abortCalled: make(chan struct{}),
	}
}

func (s *blockingStream) Read() error {
	close(s.readStarted)
	<-s.abortCalled
	return errors.New("aborted")
}

func (s *blockingStream) Abort() error {
	select {
	case <-s.abortCalled:
	default:
		close(s.abortCalled)
	}
	return nil
}

func (s *blockingStream) Stop() error {
	return nil
}

func (s *blockingStream) Close() error {
	return nil
}

func TestMicrophoneSourceReadCanceled(t *testing.T) {
	stream := newBlockingStream()
	buffer := make([]int16, 160)
	mic := newMicrophoneSourceWithStream(stream, 16000, 1, len(buffer), buffer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := mic.Read(ctx)
		errCh <- err
	}()

	<-stream.readStarted
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Read should return after context cancellation")
	}

	select {
	case <-stream.abortCalled:
	default:
		t.Fatal("expected Abort to be called on context cancellation")
	}
}
