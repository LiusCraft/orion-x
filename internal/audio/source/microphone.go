package source

import (
	"context"
	"encoding/binary"
	"io"
	"sync"

	"github.com/gordonklaus/portaudio"
	"github.com/liuscraft/orion-x/internal/logging"
)

// MicrophoneSource 麦克风音频源
type MicrophoneSource struct {
	stream     audioStream
	sampleRate int
	channels   int
	bufferSize int
	buffer     []int16
	closeCh    chan struct{}
	closeOnce  sync.Once
}

type audioStream interface {
	Read() error
	Abort() error
	Stop() error
	Close() error
}

// NewMicrophoneSource 创建新的麦克风音频源
func NewMicrophoneSource(sampleRate, channels, bufferSize int) (*MicrophoneSource, error) {
	logging.Infof("MicrophoneSource: initializing portaudio...")
	if err := portaudio.Initialize(); err != nil {
		return nil, err
	}

	buffer := make([]int16, bufferSize)
	stream, err := portaudio.OpenDefaultStream(channels, 0, float64(sampleRate), len(buffer), &buffer)
	if err != nil {
		portaudio.Terminate()
		return nil, err
	}

	logging.Infof("MicrophoneSource: created with sampleRate=%d, channels=%d, bufferSize=%d", sampleRate, channels, bufferSize)

	if err := stream.Start(); err != nil {
		stream.Close()
		portaudio.Terminate()
		return nil, err
	}

	logging.Infof("MicrophoneSource: stream started")

	return newMicrophoneSourceWithStream(stream, sampleRate, channels, bufferSize, buffer), nil
}

func newMicrophoneSourceWithStream(stream audioStream, sampleRate, channels, bufferSize int, buffer []int16) *MicrophoneSource {
	return &MicrophoneSource{
		stream:     stream,
		sampleRate: sampleRate,
		channels:   channels,
		bufferSize: bufferSize,
		buffer:     buffer,
		closeCh:    make(chan struct{}),
	}
}

// Read 读取音频数据
func (m *MicrophoneSource) Read(ctx context.Context) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	readErr := make(chan error, 1)
	go func() {
		readErr <- m.stream.Read()
	}()

	select {
	case <-ctx.Done():
		m.abortStream("context canceled")
		return nil, ctx.Err()
	case <-m.closeCh:
		m.abortStream("source closed")
		return nil, io.EOF
	case err := <-readErr:
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			select {
			case <-m.closeCh:
				return nil, io.EOF
			default:
			}
			return nil, err
		}
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	select {
	case <-m.closeCh:
		return nil, io.EOF
	default:
	}

	byteData := make([]byte, len(m.buffer)*2)
	for i, v := range m.buffer {
		binary.LittleEndian.PutUint16(byteData[i*2:], uint16(v))
	}

	return byteData, nil
}

// Close 关闭音频源
func (m *MicrophoneSource) Close() error {
	logging.Infof("MicrophoneSource: closing...")

	m.closeOnce.Do(func() {
		close(m.closeCh)
	})

	if err := m.stream.Stop(); err != nil {
		logging.Errorf("MicrophoneSource: error stopping stream: %v", err)
	}

	if err := m.stream.Close(); err != nil {
		logging.Errorf("MicrophoneSource: error closing stream: %v", err)
	}

	logging.Infof("MicrophoneSource: stream closed successfully")

	// Note: We don't terminate portaudio here as it may be used by other components
	// The program will terminate portaudio when it exits

	return nil
}

func (m *MicrophoneSource) abortStream(reason string) {
	if err := m.stream.Abort(); err != nil {
		logging.Errorf("MicrophoneSource: error aborting stream (%s): %v", reason, err)
	}
}
