package source

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
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

	// 启动状态
	started   bool
	startOnce sync.Once
	startErr  error

	// 诊断指标
	totalReads   int64
	blockedReads int64
	lastReadTime time.Time
	lastLogTime  time.Time
	mu           sync.Mutex
}

type audioStream interface {
	Start() error
	Read() error
	Abort() error
	Stop() error
	Close() error
}

// NewMicrophoneSource 创建新的麦克风音频源
// Note: The stream is NOT started immediately. Call Start() or Read() to start the stream.
// This avoids input overflow errors when there's a delay between creation and first read.
func NewMicrophoneSource(sampleRate, channels, bufferSize int) (*MicrophoneSource, error) {
	return NewMicrophoneSourceWithDevice(sampleRate, channels, bufferSize, false, "")
}

// NewMicrophoneSourceWithLatency 创建麦克风音频源，支持高延迟模式
// highLatency: 如果为 true，使用设备的默认高延迟设置（适合蓝牙设备）
// Note: The stream is NOT started immediately. Call Start() or Read() to start the stream.
func NewMicrophoneSourceWithLatency(sampleRate, channels, bufferSize int, highLatency bool) (*MicrophoneSource, error) {
	return NewMicrophoneSourceWithDevice(sampleRate, channels, bufferSize, highLatency, "")
}

// NewMicrophoneSourceWithDevice 创建麦克风音频源，支持指定设备和高延迟模式
// highLatency: 如果为 true，使用设备的默认高延迟设置（适合蓝牙设备）
// deviceName: 设备名称（部分匹配），空字符串表示使用默认设备
// Note: The stream is NOT started immediately. Call Start() or Read() to start the stream.
func NewMicrophoneSourceWithDevice(sampleRate, channels, bufferSize int, highLatency bool, deviceName string) (*MicrophoneSource, error) {
	// Note: PortAudio should be initialized by the caller before creating MicrophoneSource
	// This avoids multiple Initialize() calls which can cause device conflicts
	logging.Infof("MicrophoneSource: creating source (highLatency=%v, deviceName=%q)...", highLatency, deviceName)

	buffer := make([]int16, bufferSize)

	// 查找输入设备
	var inputDevice *portaudio.DeviceInfo
	var err error

	if deviceName != "" {
		// 按名称查找设备
		inputDevice, err = findInputDeviceByName(deviceName)
		if err != nil {
			logging.Warnf("MicrophoneSource: device %q not found, falling back to default: %v", deviceName, err)
			inputDevice = nil
		}
	}

	if inputDevice == nil {
		// 使用默认输入设备
		inputDevice, err = portaudio.DefaultInputDevice()
		if err != nil {
			logging.Errorf("MicrophoneSource: failed to get default input device: %v", err)
			// Fallback to simple stream
			stream, err := portaudio.OpenDefaultStream(channels, 0, float64(sampleRate), len(buffer), &buffer)
			if err != nil {
				return nil, err
			}
			logging.Infof("MicrophoneSource: created with fallback (sampleRate=%d, channels=%d, bufferSize=%d)", sampleRate, channels, bufferSize)
			return newMicrophoneSourceWithStream(stream, sampleRate, channels, bufferSize, buffer), nil
		}
	}

	// 选择延迟模式
	latency := inputDevice.DefaultLowInputLatency
	latencyMode := "low"
	if highLatency {
		latency = inputDevice.DefaultHighInputLatency
		latencyMode = "high"
	}

	logging.Infof("MicrophoneSource: device=%s, %s latency=%.1fms",
		inputDevice.Name, latencyMode, latency.Seconds()*1000)

	// 使用 StreamParameters 打开流，允许指定延迟
	streamParams := portaudio.StreamParameters{
		Input: portaudio.StreamDeviceParameters{
			Device:   inputDevice,
			Channels: channels,
			Latency:  latency,
		},
		SampleRate:      float64(sampleRate),
		FramesPerBuffer: bufferSize,
	}

	stream, err := portaudio.OpenStream(streamParams, &buffer)
	if err != nil {
		logging.Errorf("MicrophoneSource: failed to open stream with params: %v, falling back to default", err)
		// Fallback to simple stream
		stream, err := portaudio.OpenDefaultStream(channels, 0, float64(sampleRate), len(buffer), &buffer)
		if err != nil {
			return nil, err
		}
		logging.Infof("MicrophoneSource: created with fallback (sampleRate=%d, channels=%d, bufferSize=%d)", sampleRate, channels, bufferSize)
		return newMicrophoneSourceWithStream(stream, sampleRate, channels, bufferSize, buffer), nil
	}

	logging.Infof("MicrophoneSource: created with sampleRate=%d, channels=%d, bufferSize=%d, latency=%s (stream not started yet)",
		sampleRate, channels, bufferSize, latencyMode)

	return newMicrophoneSourceWithStream(stream, sampleRate, channels, bufferSize, buffer), nil
}

// findInputDeviceByName 按名称查找输入设备（支持部分匹配）
func findInputDeviceByName(name string) (*portaudio.DeviceInfo, error) {
	devices, err := portaudio.Devices()
	if err != nil {
		return nil, err
	}

	nameLower := strings.ToLower(name)
	for _, dev := range devices {
		if dev.MaxInputChannels > 0 && strings.Contains(strings.ToLower(dev.Name), nameLower) {
			logging.Infof("MicrophoneSource: found device %q matching %q", dev.Name, name)
			return dev, nil
		}
	}

	return nil, fmt.Errorf("no input device found matching %q", name)
}

// Start starts the audio stream. This is called automatically on first Read(),
// but can be called explicitly if you want to control when the stream starts.
func (m *MicrophoneSource) Start() error {
	m.startOnce.Do(func() {
		logging.Infof("MicrophoneSource: starting stream...")
		if err := m.stream.Start(); err != nil {
			logging.Errorf("MicrophoneSource: failed to start stream: %v", err)
			m.startErr = err
			return
		}
		m.started = true
		logging.Infof("MicrophoneSource: stream started successfully")
	})
	return m.startErr
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
// The stream is started automatically on first Read() if not already started.
func (m *MicrophoneSource) Read(ctx context.Context) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// Start the stream on first read (lazy initialization)
	if err := m.Start(); err != nil {
		return nil, err
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
