package xiaozhi

import (
	"context"
	"io"
	"sync"

	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/audio/codec"
	"github.com/liuscraft/orion-x/internal/audio/resampler"
	"github.com/liuscraft/orion-x/internal/logging"
)

// wsAudioSourceBufferFrames bounds how many pending PCM frames WSAudioSource
// buffers before dropping new ones under backpressure.
const wsAudioSourceBufferFrames = 64

// WSAudioSource implements AudioSource for a WebSocket connection. The
// connection's read loop pushes raw binary frames in via PushBinaryFrame
// (which decodes and, if needed, resamples them to the internal PCM16LE
// 16kHz mono format); ASRStage drains them via Read, exactly as it would
// drain a microphone. This lets ASRStage stay unaware of WebSocket/codec
// concerns.
type WSAudioSource struct {
	codec      codec.Codec
	resampler  resampler.Resampler
	clientRate int

	ch        chan []byte
	closeOnce sync.Once
	closed    chan struct{}
}

// NewWSAudioSource creates a WSAudioSource. c decodes incoming binary
// frames into PCM samples; clientSampleRate is the negotiated sample rate
// of those samples (from the hello handshake's audio_params), used to
// resample to the internal standard rate when they differ. A
// clientSampleRate of 0 (or equal to audio.InternalSampleRate) skips
// resampling.
func NewWSAudioSource(c codec.Codec, clientSampleRate int) *WSAudioSource {
	return &WSAudioSource{
		codec:      c,
		resampler:  resampler.NewLinearResampler(),
		clientRate: clientSampleRate,
		ch:         make(chan []byte, wsAudioSourceBufferFrames),
		closed:     make(chan struct{}),
	}
}

// PushBinaryFrame decodes a raw WebSocket binary frame and enqueues the
// resulting PCM16LE 16kHz mono bytes for Read. It's called from the
// connection's read loop and must never block it: if the internal buffer is
// full (Read side falling behind), the frame is dropped and logged, mirroring
// the existing drop-under-backpressure convention used by
// audio.ASRProcessor's internal frame channel.
func (s *WSAudioSource) PushBinaryFrame(data []byte) {
	samples, err := s.codec.Decode(data)
	if err != nil {
		logging.Warnf("WSAudioSource: decode error: %v", err)
		return
	}
	if len(samples) == 0 {
		return
	}

	if s.clientRate > 0 && s.clientRate != audio.InternalSampleRate {
		samples, err = s.resampler.Resample(samples, s.clientRate, audio.InternalSampleRate, audio.InternalChannels)
		if err != nil {
			logging.Warnf("WSAudioSource: resample error: %v", err)
			return
		}
		if len(samples) == 0 {
			return
		}
	}

	select {
	case s.ch <- audio.Int16ToBytesLE(samples):
	default:
		logging.Warnf("WSAudioSource: buffer full, dropping audio frame")
	}
}

// Read implements audio.AudioSource.
func (s *WSAudioSource) Read(ctx context.Context) ([]byte, error) {
	select {
	case data, ok := <-s.ch:
		if !ok {
			return nil, io.EOF
		}
		return data, nil
	case <-s.closed:
		return nil, io.EOF
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close implements audio.AudioSource. Safe to call multiple times.
func (s *WSAudioSource) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}
