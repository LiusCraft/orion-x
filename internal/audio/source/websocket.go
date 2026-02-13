package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
)

const defaultWebSocketSourceQueueSize = 32

var ErrWebSocketSourceQueueFull = errors.New("websocket source queue is full")

type WebSocketSourceConfig struct {
	QueueSize int
}

type WebSocketSource struct {
	frames  chan []byte
	closeCh chan struct{}
	once    sync.Once
}

func NewWebSocketSource(config *WebSocketSourceConfig) (*WebSocketSource, error) {
	queueSize := defaultWebSocketSourceQueueSize
	if config != nil && config.QueueSize > 0 {
		queueSize = config.QueueSize
	}
	if queueSize <= 0 {
		return nil, fmt.Errorf("invalid websocket source queue size: %d", queueSize)
	}

	return &WebSocketSource{
		frames:  make(chan []byte, queueSize),
		closeCh: make(chan struct{}),
	}, nil
}

func (s *WebSocketSource) PushPCM(frame []byte) error {
	if len(frame) == 0 {
		return nil
	}

	copied := make([]byte, len(frame))
	copy(copied, frame)

	select {
	case <-s.closeCh:
		return io.EOF
	default:
	}

	select {
	case <-s.closeCh:
		return io.EOF
	case s.frames <- copied:
		return nil
	default:
		return ErrWebSocketSourceQueueFull
	}
}

func (s *WebSocketSource) Read(ctx context.Context) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.closeCh:
		return nil, io.EOF
	case frame := <-s.frames:
		return frame, nil
	}
}

func (s *WebSocketSource) Close() error {
	s.once.Do(func() {
		close(s.closeCh)
	})
	return nil
}
