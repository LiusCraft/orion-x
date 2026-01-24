package audio

import (
	"context"
	"encoding/binary"
	"log"

	"github.com/gordonklaus/portaudio"
)

// MicrophoneSource 麦克风音频源
type MicrophoneSource struct {
	stream     *portaudio.Stream
	sampleRate int
	channels   int
	bufferSize int
	buffer     []int16
}

// NewMicrophoneSource 创建新的麦克风音频源
func NewMicrophoneSource(sampleRate, channels, bufferSize int) (*MicrophoneSource, error) {
	log.Printf("MicrophoneSource: initializing portaudio...")
	if err := portaudio.Initialize(); err != nil {
		return nil, err
	}

	buffer := make([]int16, bufferSize)
	stream, err := portaudio.OpenDefaultStream(channels, 0, float64(sampleRate), len(buffer), &buffer)
	if err != nil {
		portaudio.Terminate()
		return nil, err
	}

	log.Printf("MicrophoneSource: created with sampleRate=%d, channels=%d, bufferSize=%d", sampleRate, channels, bufferSize)

	if err := stream.Start(); err != nil {
		stream.Close()
		portaudio.Terminate()
		return nil, err
	}

	log.Printf("MicrophoneSource: stream started")

	return &MicrophoneSource{
		stream:     stream,
		sampleRate: sampleRate,
		channels:   channels,
		bufferSize: bufferSize,
		buffer:     buffer,
	}, nil
}

// Read 读取音频数据
func (m *MicrophoneSource) Read(ctx context.Context) ([]byte, error) {
	if err := m.stream.Read(); err != nil {
		return nil, err
	}

	byteData := make([]byte, len(m.buffer)*2)
	for i, v := range m.buffer {
		binary.LittleEndian.PutUint16(byteData[i*2:], uint16(v))
	}

	return byteData, nil
}

// Close 关闭音频源
func (m *MicrophoneSource) Close() error {
	log.Printf("MicrophoneSource: closing...")

	if err := m.stream.Abort(); err != nil {
		log.Printf("MicrophoneSource: error aborting stream: %v", err)
	}

	if err := m.stream.Stop(); err != nil {
		log.Printf("MicrophoneSource: error stopping stream: %v", err)
	}

	if err := m.stream.Close(); err != nil {
		log.Printf("MicrophoneSource: error closing stream: %v", err)
	}

	log.Printf("MicrophoneSource: stream closed successfully")

	// Note: We don't terminate portaudio here as it may be used by other components
	// The program will terminate portaudio when it exits

	return nil
}
