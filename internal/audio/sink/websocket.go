package sink

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

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

	sendSilence bool

	paceMu   sync.Mutex
	nextSend time.Time
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

	s.paceMu.Lock()
	s.nextSend = time.Time{}
	s.paceMu.Unlock()

	return nil
}

func (s *WebSocketSink) WritePCM(samples []int16) error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return errors.New("WebSocketSink: not started")
	}
	encoder := s.encoder
	format := s.format
	sendSilence := s.sendSilence
	s.mu.Unlock()

	s.paceWrite(format, samples)

	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return errors.New("WebSocketSink: not started")
	}
	encoder = s.encoder
	sendSilence = s.sendSilence
	s.mu.Unlock()

	if len(samples) == 0 {
		return nil
	}
	if isSilentPCM(samples) && !sendSilence {
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

func (s *WebSocketSink) SetSendSilence(enabled bool) {
	s.mu.Lock()
	s.sendSilence = enabled
	s.mu.Unlock()
}

func (s *WebSocketSink) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.started = false
	s.encoder = nil

	s.paceMu.Lock()
	s.nextSend = time.Time{}
	s.paceMu.Unlock()

	return nil
}

func (s *WebSocketSink) paceWrite(format audio.AudioFormat, samples []int16) {
	if len(samples) == 0 {
		return
	}
	if format.SampleRate <= 0 || format.Channels <= 0 {
		return
	}
	frames := len(samples) / format.Channels
	if frames <= 0 {
		return
	}

	duration := time.Duration(frames) * time.Second / time.Duration(format.SampleRate)
	if duration <= 0 {
		return
	}

	s.paceMu.Lock()
	now := time.Now()
	if s.nextSend.IsZero() {
		s.nextSend = now
	}

	sleepFor := s.nextSend.Sub(now)
	if sleepFor < 0 {
		s.nextSend = now
		sleepFor = 0
	}
	s.nextSend = s.nextSend.Add(duration)
	s.paceMu.Unlock()

	if sleepFor > 0 {
		time.Sleep(sleepFor)
	}
}
