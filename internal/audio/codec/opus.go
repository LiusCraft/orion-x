package codec

import (
	"errors"
	"fmt"

	"github.com/hraban/opus"
)

type OpusConfig struct {
	SampleRate      int
	Channels        int
	FrameDurationMs int
}

type OpusEncoder struct {
	encoder       *opus.Encoder
	frameSize     int
	channels      int
	maxPacketSize int
}

type OpusDecoder struct {
	decoder   *opus.Decoder
	frameSize int
	channels  int
}

func NewOpusEncoder(cfg OpusConfig) (*OpusEncoder, error) {
	frameSize, err := validateOpusConfig(cfg)
	if err != nil {
		return nil, err
	}

	enc, err := opus.NewEncoder(cfg.SampleRate, cfg.Channels, opus.AppAudio)
	if err != nil {
		return nil, err
	}

	return &OpusEncoder{
		encoder:       enc,
		frameSize:     frameSize,
		channels:      cfg.Channels,
		maxPacketSize: 4000,
	}, nil
}

func NewOpusDecoder(cfg OpusConfig) (*OpusDecoder, error) {
	frameSize, err := validateOpusConfig(cfg)
	if err != nil {
		return nil, err
	}

	dec, err := opus.NewDecoder(cfg.SampleRate, cfg.Channels)
	if err != nil {
		return nil, err
	}

	return &OpusDecoder{
		decoder:   dec,
		frameSize: frameSize,
		channels:  cfg.Channels,
	}, nil
}

func (e *OpusEncoder) Encode(pcm []int16) ([]byte, error) {
	if len(pcm) < e.frameSize*e.channels {
		return nil, fmt.Errorf("opus encoder: pcm too short: got %d, want %d", len(pcm), e.frameSize*e.channels)
	}

	buffer := make([]byte, e.maxPacketSize)
	n, err := e.encoder.Encode(pcm[:e.frameSize*e.channels], buffer)
	if err != nil {
		return nil, err
	}
	if n <= 0 {
		return nil, errors.New("opus encoder: empty packet")
	}

	packet := make([]byte, n)
	copy(packet, buffer[:n])
	return packet, nil
}

func (d *OpusDecoder) Decode(packet []byte) ([]int16, error) {
	if len(packet) == 0 {
		return nil, errors.New("opus decoder: empty packet")
	}

	pcm := make([]int16, d.frameSize*d.channels)
	n, err := d.decoder.Decode(packet, pcm)
	if err != nil {
		return nil, err
	}
	if n <= 0 {
		return nil, errors.New("opus decoder: empty pcm")
	}
	return pcm[:n*d.channels], nil
}

func validateOpusConfig(cfg OpusConfig) (int, error) {
	if cfg.SampleRate != 16000 {
		return 0, fmt.Errorf("opus: sample rate must be 16000")
	}
	if cfg.Channels != 1 && cfg.Channels != 2 {
		return 0, fmt.Errorf("opus: channels must be 1 or 2")
	}
	switch cfg.FrameDurationMs {
	case 20, 40, 60, 100:
	default:
		return 0, fmt.Errorf("opus: frame duration must be 20, 40, 60, or 100")
	}
	frameSize := cfg.SampleRate * cfg.FrameDurationMs / 1000
	if frameSize <= 0 {
		return 0, fmt.Errorf("opus: invalid frame size")
	}
	return frameSize, nil
}
