package sink

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/audio/codec"
)

type BinarySender interface {
	SendBinary(data []byte) error
}

type WebSocketSinkConfig struct {
	Format          string
	SampleRate      int
	Channels        int
	FrameDurationMs int
}

type WebSocketSink struct {
	mu      sync.Mutex
	sender  BinarySender
	config  WebSocketSinkConfig
	format  audio.AudioFormat
	encoder *codec.OpusEncoder
	started bool
}

func NewWebSocketSink(sender BinarySender, config WebSocketSinkConfig) *WebSocketSink {
	return &WebSocketSink{
		sender: sender,
		config: config,
	}
}

func (s *WebSocketSink) Start(ctx context.Context, format audio.AudioFormat) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return errors.New("WebSocketSink: already started")
	}
	if s.sender == nil {
		return errors.New("WebSocketSink: sender is nil")
	}

	if format.SampleRate <= 0 || format.Channels <= 0 || format.FramesPerBuffer <= 0 {
		return fmt.Errorf("WebSocketSink: invalid audio format")
	}
	if s.config.Format != "opus" && s.config.Format != "pcm" {
		return fmt.Errorf("WebSocketSink: invalid format: %s", s.config.Format)
	}
	if s.config.Format == "opus" && s.config.FrameDurationMs <= 0 {
		return errors.New("WebSocketSink: frame duration is required for opus")
	}

	s.format = format
	s.started = true

	if s.config.Format == "opus" {
		encoder, err := codec.NewOpusEncoder(codec.OpusConfig{
			SampleRate:      format.SampleRate,
			Channels:        format.Channels,
			FrameDurationMs: s.config.FrameDurationMs,
		})
		if err != nil {
			return err
		}
		s.encoder = encoder
	}

	return nil
}

func (s *WebSocketSink) WritePCM(samples []int16) error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return errors.New("WebSocketSink: not started")
	}
	encoder := s.encoder
	s.mu.Unlock()

	if len(samples) == 0 || isSilentPCM(samples) {
		return nil
	}

	if s.config.Format == "opus" {
		if encoder == nil {
			return errors.New("WebSocketSink: opus encoder not initialized")
		}
		packet, err := encoder.Encode(samples)
		if err != nil {
			return err
		}
		return s.sender.SendBinary(packet)
	}

	if s.config.Format != "pcm" {
		return fmt.Errorf("WebSocketSink: unsupported format: %s", s.config.Format)
	}

	payload := make([]byte, len(samples)*2)
	for i, v := range samples {
		binary.LittleEndian.PutUint16(payload[i*2:], uint16(v))
	}

	return s.sender.SendBinary(payload)
}

func isSilentPCM(samples []int16) bool {
	for _, v := range samples {
		if v != 0 {
			return false
		}
	}
	return true
}

func (s *WebSocketSink) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.started = false
	s.encoder = nil
	return nil
}
