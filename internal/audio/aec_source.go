package audio

import (
	"context"
	"io"
	"strings"
)

// EchoCancellingSource wraps an AudioSource and applies echo control at read time.
type EchoCancellingSource struct {
	source     AudioSource
	canceller  EchoCanceller
	reference  ReferenceSource
	config     EchoCancelConfig
	sampleRate int
	channels   int
	frameBytes int
}

func NewEchoCancellingSource(source AudioSource, config EchoCancelConfig, reference ReferenceSource, canceller EchoCanceller, sampleRate, channels int) *EchoCancellingSource {
	if config.FrameMs <= 0 {
		config.FrameMs = 10
	}
	if config.ReferenceActiveWindowMs <= 0 {
		config.ReferenceActiveWindowMs = 200
	}
	return &EchoCancellingSource{
		source:     source,
		canceller:  canceller,
		reference:  reference,
		config:     config,
		sampleRate: sampleRate,
		channels:   channels,
		frameBytes: FrameBytes(sampleRate, channels, config.FrameMs),
	}
}

func (s *EchoCancellingSource) Read(ctx context.Context) ([]byte, error) {
	if s.source == nil {
		return nil, io.EOF
	}
	data, err := s.source.Read(ctx)
	if err != nil || len(data) == 0 {
		return data, err
	}
	if !s.config.Enabled {
		return data, nil
	}
	if s.channels != 1 {
		return data, nil
	}
	mode := strings.ToLower(strings.TrimSpace(s.config.Mode))
	if mode == "gate" {
		if s.reference != nil && s.reference.IsActive() {
			return make([]byte, len(data)), nil
		}
		return data, nil
	}
	if s.canceller == nil || s.reference == nil || s.frameBytes <= 0 {
		return data, nil
	}

	processed := make([]byte, len(data))
	copy(processed, data)
	for offset := 0; offset+s.frameBytes <= len(processed); offset += s.frameBytes {
		near := processed[offset : offset+s.frameBytes]
		far := s.reference.ReadReference()
		if len(far) != len(near) {
			far = make([]byte, len(near))
		}
		out, err := s.canceller.Process(near, far)
		if err != nil || len(out) != len(near) {
			continue
		}
		copy(processed[offset:offset+s.frameBytes], out)
	}
	return processed, nil
}

func (s *EchoCancellingSource) Close() error {
	if s.canceller != nil {
		_ = s.canceller.Close()
	}
	if s.source != nil {
		return s.source.Close()
	}
	return nil
}
