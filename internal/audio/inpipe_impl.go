package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/asr"
	"github.com/liuscraft/orion-x/internal/logging"
)

type InPipeState int

const (
	InPipeStateIdle InPipeState = iota
	InPipeStateListening
	InPipeStateStopping
)

func (s InPipeState) String() string {
	switch s {
	case InPipeStateIdle:
		return "Idle"
	case InPipeStateListening:
		return "Listening"
	case InPipeStateStopping:
		return "Stopping"
	default:
		return "Unknown"
	}
}

type inPipeImpl struct {
	state       InPipeState
	config      *InPipeConfig
	recognizer  asr.Recognizer
	asrHandler  func(text string, isFinal bool)
	vadHandler  func()
	audioSource AudioSource
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	mu          sync.Mutex

	vadEnabled     bool
	vadThreshold   float64
	vadMinInterval time.Duration
	lastVADTime    time.Time
}

func NewInPipeWithRecognizer(config *InPipeConfig, recognizer asr.Recognizer) AudioInPipe {
	if config == nil {
		config = DefaultInPipeConfig()
	}
	vadThreshold := config.VADThreshold
	if vadThreshold <= 0 {
		vadThreshold = 0.5
	}
	return &inPipeImpl{
		state:          InPipeStateIdle,
		config:         config,
		recognizer:     recognizer,
		vadEnabled:     config.EnableVAD,
		vadThreshold:   vadThreshold,
		vadMinInterval: 300 * time.Millisecond,
	}
}

func (p *inPipeImpl) SetAudioSource(source AudioSource) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.audioSource = source
}

func (p *inPipeImpl) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != InPipeStateIdle {
		return logError("AudioInPipe: already started, current state: %s", p.state)
	}

	p.ctx, p.cancel = context.WithCancel(ctx)

	if err := p.recognizer.Start(p.ctx); err != nil {
		return logError("AudioInPipe: ASR start error: %v", err)
	}

	p.recognizer.OnResult(func(result asr.Result) {
		p.handleASRResult(result)
	})

	p.state = InPipeStateListening

	if p.audioSource != nil {
		logging.Infof("AudioInPipe: starting audio source...")
		p.wg.Add(1)
		go p.readAudioFromSource(p.ctx)
	}

	logging.Infof("AudioInPipe: started, state: %s", p.state)
	return nil
}

func (p *inPipeImpl) Stop() error {
	p.mu.Lock()
	if p.state == InPipeStateIdle {
		p.mu.Unlock()
		return logError("AudioInPipe: already stopped")
	}

	if p.state == InPipeStateStopping {
		p.mu.Unlock()
		return logError("AudioInPipe: already stopping")
	}

	p.state = InPipeStateStopping
	cancel := p.cancel
	audioSource := p.audioSource
	recognizer := p.recognizer
	ctx := p.ctx
	p.mu.Unlock()

	logging.Infof("AudioInPipe: stopping...")

	if cancel != nil {
		logging.Infof("AudioInPipe: canceling context...")
		cancel()
	}

	if audioSource != nil {
		logging.Infof("AudioInPipe: closing audio source (should unblock read)...")
		if err := audioSource.Close(); err != nil {
			logging.Errorf("AudioInPipe: error closing audio source: %v", err)
		}
		logging.Infof("AudioInPipe: audio source closed")
	}

	if recognizer != nil {
		if ctx == nil {
			ctx = context.Background()
		}
		logging.Infof("AudioInPipe: finishing ASR...")
		_ = recognizer.Finish(ctx)
		_ = recognizer.Close()
		logging.Infof("AudioInPipe: ASR finished")
	}

	logging.Infof("AudioInPipe: waiting for goroutines to finish...")
	p.wg.Wait()
	logging.Infof("AudioInPipe: all goroutines finished")

	p.mu.Lock()
	p.state = InPipeStateIdle
	logging.Infof("AudioInPipe: stopped, state: %s", p.state)
	p.mu.Unlock()
	return nil
}

func (p *inPipeImpl) SendAudio(audio []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == InPipeStateStopping {
		return nil
	}

	if p.state != InPipeStateListening {
		return logError("AudioInPipe: not in listening state, current: %s", p.state)
	}

	if p.recognizer == nil {
		return logError("AudioInPipe: recognizer not initialized")
	}

	if err := p.recognizer.SendAudio(p.ctx, audio); err != nil {
		if err == context.Canceled {
			return nil
		}
		return logError("AudioInPipe: send audio error: %v", err)
	}

	return nil
}

func (p *inPipeImpl) OnASRResult(handler func(text string, isFinal bool)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.asrHandler = handler
}

func (p *inPipeImpl) OnUserSpeakingDetected(handler func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.vadHandler = handler
}

func (p *inPipeImpl) readAudioFromSource(ctx context.Context) {
	defer p.wg.Done()

	logging.Infof("AudioInPipe: audio reader goroutine started")
	defer logging.Infof("AudioInPipe: audio reader goroutine stopped")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		audio, err := p.audioSource.Read(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, io.EOF) {
				return
			}
			logging.Errorf("AudioInPipe: error reading from audio source: %v", err)
			return
		}

		p.handleVAD(audio)

		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := p.SendAudio(audio); err != nil {
			if err == context.Canceled {
				return
			}
			logging.Errorf("AudioInPipe: error sending audio to ASR: %v", err)
		}
	}
}

func (p *inPipeImpl) handleASRResult(result asr.Result) {
	p.mu.Lock()
	handler := p.asrHandler
	p.mu.Unlock()

	if handler != nil {
		handler(result.Text, result.IsFinal)
	}
}

func (p *inPipeImpl) handleVAD(audio []byte) {
	if !p.vadEnabled {
		return
	}
	if !p.detectSpeech(audio) {
		return
	}

	now := time.Now()
	p.mu.Lock()
	last := p.lastVADTime
	minInterval := p.vadMinInterval
	handler := p.vadHandler
	if now.Sub(last) >= minInterval {
		p.lastVADTime = now
	}
	p.mu.Unlock()

	if handler == nil {
		return
	}
	if now.Sub(last) < minInterval {
		return
	}
	handler()
}

func (p *inPipeImpl) detectSpeech(audio []byte) bool {
	if len(audio) < 2 {
		return false
	}
	var sum float64
	count := len(audio) / 2
	for i := 0; i < count; i++ {
		lo := audio[i*2]
		hi := audio[i*2+1]
		sample := int16(lo) | int16(hi)<<8
		v := float64(sample) / 32768.0
		sum += v * v
	}
	rms := math.Sqrt(sum / float64(count))
	return rms >= p.vadThreshold
}

func (p *inPipeImpl) GetState() InPipeState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func logError(format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	logging.Errorf("%s", msg)
	return fmt.Errorf("%s", msg)
}
