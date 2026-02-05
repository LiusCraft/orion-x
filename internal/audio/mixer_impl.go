package audio

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/liuscraft/orion-x/internal/logging"
)

type mixerImpl struct {
	config                *MixerConfig
	sink                  AudioSink
	ttsStream             io.Reader
	resourceStream        io.Reader
	currentTTSVolume      float64
	currentResourceVolume float64
	mu                    sync.Mutex
	ctx                   context.Context
	cancel                context.CancelFunc
	wg                    sync.WaitGroup
	started               bool
}

func NewMixer(config *MixerConfig) (AudioMixer, error) {
	if config == nil {
		config = DefaultMixerConfig()
	}

	m := &mixerImpl{
		config:                config,
		currentTTSVolume:      config.TTSVolume,
		currentResourceVolume: config.ResourceVolume,
	}
	return m, nil
}

func (m *mixerImpl) AddTTSStream(audio io.Reader) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ttsStream = audio
}

func (m *mixerImpl) AddResourceStream(audio io.Reader) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resourceStream = audio
}

func (m *mixerImpl) RemoveTTSStream() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ttsStream = nil
}

func (m *mixerImpl) RemoveResourceStream() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resourceStream = nil
}

func (m *mixerImpl) SetTTSVolume(volume float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentTTSVolume = volume
}

func (m *mixerImpl) SetResourceVolume(volume float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentResourceVolume = volume
}

func (m *mixerImpl) OnTTSStarted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	logging.Infof("AudioMixer: TTS started, reducing resource volume to 50%%")
	m.currentResourceVolume = m.config.ResourceVolume * 0.5
}

func (m *mixerImpl) OnTTSFinished() {
	m.mu.Lock()
	defer m.mu.Unlock()
	logging.Infof("AudioMixer: TTS finished, restoring resource volume to 100%%")
	m.currentResourceVolume = m.config.ResourceVolume
}

func (m *mixerImpl) SetSink(sink AudioSink) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		logging.Warnf("AudioMixer: SetSink called after Start; ignoring")
		return
	}
	m.sink = sink
}

func (m *mixerImpl) Start() error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}
	if m.sink == nil {
		m.mu.Unlock()
		return errors.New("AudioMixer: sink not set")
	}

	sampleRate := m.config.SampleRate
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	channels := m.config.Channels
	if channels <= 0 {
		channels = 2
	}
	framesPerBuffer := m.config.FramesPerBuffer
	if framesPerBuffer <= 0 {
		framesPerBuffer = 1024
	}

	m.ctx, m.cancel = context.WithCancel(context.Background())
	sink := m.sink
	m.started = true
	m.mu.Unlock()

	format := AudioFormat{
		SampleRate:      sampleRate,
		Channels:        channels,
		FramesPerBuffer: framesPerBuffer,
	}
	if err := sink.Start(m.ctx, format); err != nil {
		m.mu.Lock()
		m.started = false
		m.cancel = nil
		m.mu.Unlock()
		return err
	}

	m.wg.Add(1)
	go m.mixLoop(m.ctx, sink, format)

	return nil
}

func (m *mixerImpl) Stop() error {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return nil
	}
	cancel := m.cancel
	sink := m.sink
	m.started = false
	m.cancel = nil
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if sink != nil {
		if err := sink.Stop(); err != nil {
			logging.Errorf("AudioMixer: failed to stop sink: %v", err)
		}
	}

	m.wg.Wait()
	return nil
}

func (m *mixerImpl) mixLoop(ctx context.Context, sink AudioSink, format AudioFormat) {
	defer m.wg.Done()

	if format.Channels <= 0 || format.FramesPerBuffer <= 0 {
		return
	}

	out := make([][]float32, format.Channels)
	for ch := range out {
		out[ch] = make([]float32, format.FramesPerBuffer)
	}
	interleaved := make([]int16, format.FramesPerBuffer*format.Channels)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		zeroOut(out)

		m.mu.Lock()
		ttsStream := m.ttsStream
		resourceStream := m.resourceStream
		ttsVolume := m.currentTTSVolume
		resourceVolume := m.currentResourceVolume
		m.mu.Unlock()

		mixFromStream(ttsStream, out, float32(ttsVolume))
		mixFromStream(resourceStream, out, float32(resourceVolume))
		interleaveToInt16(out, interleaved)

		if err := sink.WritePCM(interleaved); err != nil {
			logging.Errorf("AudioMixer: sink write failed: %v", err)
			return
		}
	}
}

func zeroOut(buf [][]float32) {
	for ch := range buf {
		for i := range buf[ch] {
			buf[ch][i] = 0
		}
	}
}

func interleaveToInt16(src [][]float32, dst []int16) {
	if len(src) == 0 || len(src[0]) == 0 || len(dst) == 0 {
		return
	}

	frames := len(src[0])
	channels := len(src)
	maxFrames := len(dst) / channels
	if frames > maxFrames {
		frames = maxFrames
	}

	idx := 0
	for i := 0; i < frames; i++ {
		for ch := 0; ch < channels; ch++ {
			sample := src[ch][i]
			if sample > 1.0 {
				sample = 1.0
			} else if sample < -1.0 {
				sample = -1.0
			}
			dst[idx] = int16(sample * 32767.0)
			idx++
		}
	}
}

func mixFromStream(stream io.Reader, buf [][]float32, volume float32) {
	if stream == nil || len(buf) == 0 || len(buf[0]) == 0 {
		return
	}

	// 16-bit PCM uses 2 bytes per sample; read exactly the frame size to avoid dropping data
	samples := make([]byte, len(buf[0])*2)
	n, err := io.ReadFull(stream, samples)
	if err != nil && err != io.ErrUnexpectedEOF {
		return
	}
	limit := n / 2
	for i := 0; i < limit && i < len(buf[0]); i++ {
		sample := int16(samples[i*2]) | int16(samples[i*2+1])<<8
		normalized := float32(sample) / 32768.0

		for ch := range buf {
			buf[ch][i] += normalized * volume
			if buf[ch][i] > 1.0 {
				buf[ch][i] = 1.0
			} else if buf[ch][i] < -1.0 {
				buf[ch][i] = -1.0
			}
		}
	}
}
