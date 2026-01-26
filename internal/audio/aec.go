package audio

import (
	"sync"
	"time"
)

// EchoCanceller processes near-end audio with far-end reference.
type EchoCanceller interface {
	Process(near []byte, far []byte) ([]byte, error)
	Close() error
}

// NoopEchoCanceller is a placeholder implementation.
type NoopEchoCanceller struct{}

func NewNoopEchoCanceller() *NoopEchoCanceller {
	return &NoopEchoCanceller{}
}

func (c *NoopEchoCanceller) Process(near []byte, far []byte) ([]byte, error) {
	return near, nil
}

func (c *NoopEchoCanceller) Close() error {
	return nil
}

// EchoCancelConfig controls how echo cancellation is applied at the source level.
type EchoCancelConfig struct {
	Enabled                 bool
	Mode                    string
	FrameMs                 int
	FarEndDelayMs           int
	ReferenceActiveWindowMs int
}

func DefaultEchoCancelConfig() EchoCancelConfig {
	return EchoCancelConfig{
		Enabled:                 true,
		Mode:                    "gate",
		FrameMs:                 10,
		FarEndDelayMs:           50,
		ReferenceActiveWindowMs: 200,
	}
}

// ReferenceSink receives playback reference PCM data.
type ReferenceSink interface {
	WriteReference(p []byte)
}

// ReferenceSource provides reference frames for echo cancellation.
type ReferenceSource interface {
	ReadReference() []byte
	IsActive() bool
}

// ReferenceBuffer stores far-end reference audio in fixed-size frames.
type ReferenceBuffer struct {
	mu           sync.Mutex
	frameBytes   int
	maxFrames    int
	delayFrames  int
	frames       [][]byte
	head         int
	size         int
	lastWrite    time.Time
	activeWindow time.Duration
}

func NewReferenceBuffer(frameBytes, maxFrames, delayFrames int) *ReferenceBuffer {
	if frameBytes <= 0 {
		frameBytes = 320
	}
	if maxFrames <= 0 {
		maxFrames = 200
	}
	if delayFrames < 0 {
		delayFrames = 0
	}

	frames := make([][]byte, maxFrames)
	for i := 0; i < maxFrames; i++ {
		frames[i] = make([]byte, frameBytes)
	}

	return &ReferenceBuffer{
		frameBytes:   frameBytes,
		maxFrames:    maxFrames,
		delayFrames:  delayFrames,
		frames:       frames,
		activeWindow: 200 * time.Millisecond,
	}
}

func (b *ReferenceBuffer) SetActiveWindow(window time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if window <= 0 {
		window = 200 * time.Millisecond
	}
	b.activeWindow = window
}

func (b *ReferenceBuffer) WriteReference(p []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(p) == 0 || b.frameBytes <= 0 {
		return
	}
	for offset := 0; offset+b.frameBytes <= len(p); offset += b.frameBytes {
		frame := b.frames[(b.head+b.size)%b.maxFrames]
		copy(frame, p[offset:offset+b.frameBytes])
		if b.size < b.maxFrames {
			b.size++
		} else {
			b.head = (b.head + 1) % b.maxFrames
		}
		b.lastWrite = time.Now()
	}
}

func (b *ReferenceBuffer) ReadReference() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.frameBytes <= 0 {
		return nil
	}
	if b.size <= b.delayFrames {
		return make([]byte, b.frameBytes)
	}
	frame := make([]byte, b.frameBytes)
	copy(frame, b.frames[b.head])
	b.head = (b.head + 1) % b.maxFrames
	b.size--
	return frame
}

func (b *ReferenceBuffer) IsActive() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.lastWrite.IsZero() {
		return false
	}
	return time.Since(b.lastWrite) <= b.activeWindow
}

func FrameBytes(sampleRate, channels, frameMs int) int {
	if sampleRate <= 0 || channels <= 0 || frameMs <= 0 {
		return 0
	}
	samples := sampleRate * frameMs / 1000
	return samples * channels * 2
}
