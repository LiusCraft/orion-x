package sink

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/logging"
)

// PortAudioSink plays PCM16 samples through the system audio device.
// Note: PortAudio must be initialized by the caller before Start().
type PortAudioSink struct {
	mu      sync.Mutex
	stream  *portaudio.Stream
	buffer  []int16
	started bool

	lastUnderflowLog time.Time
}

func NewPortAudioSink() *PortAudioSink {
	return &PortAudioSink{}
}

func (s *PortAudioSink) Start(ctx context.Context, format audio.AudioFormat) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return errors.New("PortAudioSink: already started")
	}

	sampleRate := format.SampleRate
	if sampleRate <= 0 {
		return errors.New("PortAudioSink: invalid sample rate")
	}
	channels := format.Channels
	if channels <= 0 {
		return errors.New("PortAudioSink: invalid channels")
	}
	frames := format.FramesPerBuffer
	if frames <= 0 {
		frames = 1024
	}

	// 查询默认输出设备支持的最大声道数，蓝牙 HFP 模式下只支持 1 声道
	if outputDevice, err := portaudio.DefaultOutputDevice(); err == nil {
		if outputDevice.MaxOutputChannels < channels {
			logging.Warnf("PortAudioSink: output device %q supports max %d channels, reducing from %d (likely Bluetooth HFP mode)",
				outputDevice.Name, outputDevice.MaxOutputChannels, channels)
			channels = outputDevice.MaxOutputChannels
		}
	}

	s.buffer = make([]int16, frames*channels)
	stream, err := portaudio.OpenDefaultStream(0, channels, float64(sampleRate), frames, &s.buffer)
	if err != nil {
		return err
	}

	if err := stream.Start(); err != nil {
		_ = stream.Close()
		return err
	}

	s.stream = stream
	s.started = true
	logging.Infof("PortAudioSink: started (sampleRate=%d, channels=%d, frames=%d)", sampleRate, channels, frames)

	return nil
}

func (s *PortAudioSink) WritePCM(samples []int16) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started || s.stream == nil {
		return errors.New("PortAudioSink: not started")
	}

	n := copy(s.buffer, samples)
	for i := n; i < len(s.buffer); i++ {
		s.buffer[i] = 0
	}

	if err := s.stream.Write(); err != nil {
		if isOutputUnderflow(err) {
			now := time.Now()
			if now.Sub(s.lastUnderflowLog) >= 5*time.Second {
				s.lastUnderflowLog = now
				logging.Warnf("PortAudioSink: output underflowed; continuing local playback")
			}
			return nil
		}
		return err
	}
	return nil
}

func isOutputUnderflow(err error) bool {
	if err == nil {
		return false
	}
	if err == portaudio.OutputUnderflowed {
		return true
	}
	var paErr portaudio.Error
	return errors.As(err, &paErr) && paErr == portaudio.OutputUnderflowed
}

func (s *PortAudioSink) Stop() error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return nil
	}
	stream := s.stream
	s.stream = nil
	s.started = false
	s.mu.Unlock()

	if stream != nil {
		if err := stream.Stop(); err != nil {
			logging.Errorf("PortAudioSink: failed to stop stream: %v", err)
		}
		if err := stream.Close(); err != nil {
			logging.Errorf("PortAudioSink: failed to close stream: %v", err)
		}
	}
	logging.Infof("PortAudioSink: stopped")
	return nil
}
