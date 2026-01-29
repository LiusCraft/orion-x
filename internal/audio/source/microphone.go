package source

import (
	"context"
	"encoding/binary"
	"io"
	"sync"
	"time"

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

	// 诊断指标
	totalReads   int64
	blockedReads int64
	lastReadTime time.Time
	lastLogTime  time.Time
	mu           sync.Mutex
}

type audioStream interface {
	Read() error
	Abort() error
	Stop() error
	Close() error
}

// NewMicrophoneSource 创建新的麦克风音频源
func NewMicrophoneSource(sampleRate, channels, bufferSize int) (*MicrophoneSource, error) {
	// Note: PortAudio should be initialized by the caller before creating MicrophoneSource
	// This avoids multiple Initialize() calls which can cause device conflicts
	logging.Infof("MicrophoneSource: creating source...")

	buffer := make([]int16, bufferSize)
	stream, err := portaudio.OpenDefaultStream(channels, 0, float64(sampleRate), len(buffer), &buffer)
	if err != nil {
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

	readStart := time.Now()
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
		// 记录读取延迟
		readDuration := time.Since(readStart)
		m.recordReadMetrics(readDuration)

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

func (m *MicrophoneSource) recordReadMetrics(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalReads++
	m.lastReadTime = time.Now()

	// 预期读取时间：bufferSize / sampleRate
	// 例如：3200 samples @ 16kHz = 200ms
	expectedDuration := time.Duration(float64(m.bufferSize)/float64(m.sampleRate)*1000) * time.Millisecond
	threshold := expectedDuration * 3 // 3倍预期时间视为阻塞

	if duration > threshold {
		m.blockedReads++
		logging.Warnf("MicrophoneSource: Read blocked for %v (expected ~%v), blocked count: %d/%d",
			duration, expectedDuration, m.blockedReads, m.totalReads)
	}

	// 每 10 秒打印一次诊断信息
	now := time.Now()
	if now.Sub(m.lastLogTime) >= 10*time.Second {
		m.lastLogTime = now
		blockRate := float64(m.blockedReads) / float64(m.totalReads) * 100
		logging.Infof("MicrophoneSource: metrics - total reads: %d, blocked: %d (%.1f%%), last read: %v ago",
			m.totalReads, m.blockedReads, blockRate, time.Since(m.lastReadTime))
	}
}
