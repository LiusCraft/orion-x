package audio

import (
	"context"
	"io"
	"sync"

	"github.com/gordonklaus/portaudio"
	"github.com/liuscraft/orion-x/internal/logging"
)

type mixerImpl struct {
	config                *MixerConfig
	ttsStream             io.Reader
	resourceStream        io.Reader
	currentTTSVolume      float64
	currentResourceVolume float64
	mu                    sync.Mutex
	ctx                   context.Context
	cancel                context.CancelFunc
	player                *portaudio.Stream
	started               bool
}

func NewMixer(config *MixerConfig) (AudioMixer, error) {
	if config == nil {
		config = DefaultMixerConfig()
	}
	if err := portaudio.Initialize(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &mixerImpl{
		config:                config,
		currentTTSVolume:      config.TTSVolume,
		currentResourceVolume: config.ResourceVolume,
		ctx:                   ctx,
		cancel:                cancel,
	}
	// Use sample rate and channels from config
	sampleRate := config.SampleRate
	if sampleRate == 0 {
		sampleRate = 16000 // fallback to default
	}
	channels := config.Channels
	if channels == 0 {
		channels = 2 // fallback to stereo
	}

	stream, err := portaudio.OpenDefaultStream(0, channels, float64(sampleRate), 1024, m.audioCallback)
	if err != nil {
		portaudio.Terminate()
		cancel()
		return nil, err
	}
	m.player = stream
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

func (m *mixerImpl) Start() {
	m.mu.Lock()
	if m.player == nil || m.started {
		m.mu.Unlock()
		return
	}
	player := m.player
	m.started = true
	m.mu.Unlock()

	go func() {
		if err := player.Start(); err != nil {
			logging.Errorf("AudioMixer: failed to start stream: %v", err)
			m.mu.Lock()
			m.started = false
			m.mu.Unlock()
		}
	}()
}

func (m *mixerImpl) Stop() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	player := m.player
	m.player = nil
	m.started = false
	m.mu.Unlock()

	if player != nil {
		if err := player.Stop(); err != nil {
			logging.Errorf("AudioMixer: failed to stop stream: %v", err)
		}
		if err := player.Close(); err != nil {
			logging.Errorf("AudioMixer: failed to close stream: %v", err)
		}
	}

	portaudio.Terminate()
}

func (m *mixerImpl) audioCallback(out [][]float32) {
	for i := range out[0] {
		out[0][i] = 0
		out[1][i] = 0
	}
	m.mu.Lock()
	ttsStream := m.ttsStream
	resourceStream := m.resourceStream
	ttsVolume := m.currentTTSVolume
	resourceVolume := m.currentResourceVolume
	m.mu.Unlock()
	mixFromStream(ttsStream, out, float32(ttsVolume))
	mixFromStream(resourceStream, out, float32(resourceVolume))
}

func mixFromStream(stream io.Reader, buf [][]float32, volume float32) {
	if stream == nil {
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

		buf[0][i] += normalized * volume
		buf[1][i] += normalized * volume

		if buf[0][i] > 1.0 {
			buf[0][i] = 1.0
		} else if buf[0][i] < -1.0 {
			buf[0][i] = -1.0
		}

		if buf[1][i] > 1.0 {
			buf[1][i] = 1.0
		} else if buf[1][i] < -1.0 {
			buf[1][i] = -1.0
		}
	}
}
