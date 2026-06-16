package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/audio/vad"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/provider/asr"
	_ "github.com/liuscraft/orion-x/internal/provider/asr/register"
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

// AudioSource 音频输入源接口
type AudioSource interface {
	Read(ctx context.Context) ([]byte, error)
	Close() error
}

// InPipeConfig InPipe配置
type InPipeConfig struct {
	SampleRate      int
	Channels        int
	EnableVAD       bool
	VADThreshold    float64
	VADType         string // "silero", default "silero"
	VADModelPath    string // Silero VAD model path, default "models/silero_vad.onnx"
	VADMinSilenceMs int    // Silero VAD min silence duration in ms, default 500
	VADSpeechPadMs  int    // Silero VAD speech pad in ms, default 300
	ASRAPIKey       string
	ASRProviderType string
	ASRModel        string
	ASREndpoint     string
}

// DefaultInPipeConfig 默认配置
func DefaultInPipeConfig() *InPipeConfig {
	return &InPipeConfig{
		SampleRate:      16000,
		Channels:        1,
		EnableVAD:       true,
		VADThreshold:    0.5,
		VADType:         string(vad.TypeSilero),
		VADModelPath:    vad.DefaultModelPath,
		VADMinSilenceMs: 500,
		VADSpeechPadMs:  300,
		ASRModel:        "fun-asr-realtime",
	}
}

// InPipe 音频输入管道，负责音频输入管理、VAD 和 ASR 调用
type InPipe struct {
	state       InPipeState
	config      *InPipeConfig
	recognizer  asr.Recognizer
	asrHandler  func(text string, isFinal bool)
	vadHandler  func()
	audioSource AudioSource
	ctx         context.Context
	cancel      context.CancelFunc
	readWG      sync.WaitGroup
	asrWG       sync.WaitGroup
	mu          sync.Mutex

	vadEnabled bool
	segmenter  vad.Segmenter // VAD + 切段
}

func NewInPipe(config *InPipeConfig, source AudioSource) (*InPipe, error) {
	if config == nil {
		config = DefaultInPipeConfig()
	}

	asrCfg := asr.Config{
		APIKey:     config.ASRAPIKey,
		Model:      config.ASRModel,
		Endpoint:   config.ASREndpoint,
		Format:     "pcm",
		SampleRate: config.SampleRate,
	}

	recognizer, err := asr.NewRecognizer(asr.ProviderConfig{
		Type:   config.ASRProviderType,
		Config: asrCfg,
	})
	if err != nil {
		return nil, err
	}

	var seg vad.Segmenter
	if config.EnableVAD && config.VADType == string(vad.TypeSilero) {
		seg, err = vad.NewSegmenterWithConfig(vad.SegmenterConfig{
			SampleRate:   config.SampleRate,
			Threshold:    config.VADThreshold,
			MinSilenceMs: config.VADMinSilenceMs,
			SpeechPadMs:  config.VADSpeechPadMs,
			ModelPath:    config.VADModelPath,
		})
		if err != nil {
			logging.Warnf("AudioInPipe: failed to create segmenter: %v, VAD will be disabled", err)
		}
	}

	return &InPipe{
		state:       InPipeStateIdle,
		config:      config,
		recognizer:  recognizer,
		vadEnabled:  config.EnableVAD && seg != nil,
		segmenter:   seg,
		audioSource: source,
	}, nil
}

func (p *InPipe) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != InPipeStateIdle {
		return logError("AudioInPipe: already started, current state: %s", p.state)
	}

	p.ctx, p.cancel = context.WithCancel(ctx)

	p.recognizer.OnResult(func(result asr.Result) {
		p.handleASRResult(result)
	})

	if p.vadEnabled {
		// VAD 模式下不提前 Start recognizer，每个语音段单独 Start/Send/Finish
	} else {
		if err := p.recognizer.Start(p.ctx); err != nil {
			return logError("AudioInPipe: ASR start error: %v", err)
		}
	}

	p.state = InPipeStateListening

	if p.audioSource != nil {
		logging.Infof("AudioInPipe: starting audio source...")
		p.readWG.Add(1)
		go p.readAudioFromSource(p.ctx)
	}

	logging.Infof("AudioInPipe: started, state: %s", p.state)
	return nil
}

func (p *InPipe) Stop() error {
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
	segmenter := p.segmenter
	ctx := p.ctx
	vadEnabled := p.vadEnabled
	p.mu.Unlock()

	logging.Infof("AudioInPipe: stopping...")

	if audioSource != nil {
		logging.Infof("AudioInPipe: closing audio source (should unblock read)...")
		if err := audioSource.Close(); err != nil {
			logging.Errorf("AudioInPipe: error closing audio source: %v", err)
		}
		logging.Infof("AudioInPipe: audio source closed")
	}

	if vadEnabled {
		logging.Infof("AudioInPipe: waiting for audio reader to finish...")
		p.readWG.Wait()
		if seg := segmenter.Flush(); seg != nil && seg.Bytes > 0 {
			_ = p.recognizeSegment(ctx, *seg)
		}
		if cancel != nil {
			cancel()
		}
		logging.Infof("AudioInPipe: waiting for in-flight ASR to finish...")
		p.asrWG.Wait()
		if recognizer != nil {
			_ = recognizer.Close()
			logging.Infof("AudioInPipe: ASR closed")
		}
	} else {
		if cancel != nil {
			logging.Infof("AudioInPipe: canceling context...")
			cancel()
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
		logging.Infof("AudioInPipe: waiting for audio reader to finish...")
		p.readWG.Wait()
	}
	if segmenter != nil {
		if err := segmenter.Close(); err != nil {
			logging.Warnf("AudioInPipe: close segmenter failed: %v", err)
		}
	}

	logging.Infof("AudioInPipe: all goroutines finished")

	p.mu.Lock()
	p.state = InPipeStateIdle
	logging.Infof("AudioInPipe: stopped, state: %s", p.state)
	p.mu.Unlock()
	return nil
}

func (p *InPipe) SendAudio(audio []byte) error {
	p.mu.Lock()
	if p.state == InPipeStateStopping {
		p.mu.Unlock()
		return nil
	}

	if p.state != InPipeStateListening {
		p.mu.Unlock()
		return logError("AudioInPipe: not in listening state, current: %s", p.state)
	}

	recognizer := p.recognizer
	ctx := p.ctx
	vadEnabled := p.vadEnabled
	p.mu.Unlock()

	if recognizer == nil {
		return logError("AudioInPipe: recognizer not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if vadEnabled {
		p.handleVADAudio(ctx, audio)
		return nil
	}

	if err := recognizer.SendAudio(ctx, audio); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return nil
		}
		return logError("AudioInPipe: send audio error: %v", err)
	}

	return nil
}

func (p *InPipe) OnASRResult(handler func(text string, isFinal bool)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.asrHandler = handler
}

func (p *InPipe) OnUserSpeakingDetected(handler func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.vadHandler = handler
}

func (p *InPipe) readAudioFromSource(ctx context.Context) {
	defer p.readWG.Done()

	logging.Infof("AudioInPipe: audio reader goroutine started")
	defer logging.Infof("AudioInPipe: audio reader goroutine stopped")
	defer func() {
		if p.vadEnabled && p.segmenter != nil {
			if seg := p.segmenter.Flush(); seg != nil && seg.Bytes > 0 {
				_ = p.recognizeSegment(ctx, *seg)
			}
		}
	}()

	consecutiveErrors := 0
	const maxConsecutiveErrors = 5

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

			// Handle transient errors like "Input overflowed" gracefully
			// These can happen during startup or under high load
			consecutiveErrors++
			if consecutiveErrors >= maxConsecutiveErrors {
				logging.Errorf("AudioInPipe: too many consecutive errors (%d), stopping: %v", consecutiveErrors, err)
				return
			}

			logging.Warnf("AudioInPipe: transient error reading from audio source (attempt %d/%d): %v",
				consecutiveErrors, maxConsecutiveErrors, err)

			// Brief pause before retry to avoid tight error loop
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
			continue
		}

		// Reset error counter on successful read
		consecutiveErrors = 0

		if err := p.SendAudio(audio); err != nil {
			if err == context.Canceled {
				return
			}
			logging.Errorf("AudioInPipe: error sending audio to ASR: %v", err)
		}
	}
}

func (p *InPipe) handleASRResult(result asr.Result) {
	p.mu.Lock()
	handler := p.asrHandler
	p.mu.Unlock()

	if handler != nil {
		handler(result.Text, result.IsFinal)
	}
}

func (p *InPipe) handleVADAudio(ctx context.Context, audio []byte) {
	seg, started := p.segmenter.Process(audio)
	if started {
		p.mu.Lock()
		if p.vadHandler != nil {
			p.vadHandler()
		}
		p.mu.Unlock()
	}
	if seg == nil || seg.Bytes == 0 {
		return
	}

	logging.Infof("AudioInPipe: queued VAD segment frames=%d bytes=%d", len(seg.Frames), seg.Bytes)
	p.asrWG.Add(1)
	go func() {
		defer p.asrWG.Done()
		_ = p.recognizeSegment(ctx, *seg)
	}()
}

func (p *InPipe) recognizeSegment(ctx context.Context, segment vad.Segment) error {
	p.mu.Lock()
	recognizer := p.recognizer
	p.mu.Unlock()

	if recognizer == nil {
		return logError("AudioInPipe: recognizer not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	logging.Infof("AudioInPipe: recognizing VAD segment frames=%d bytes=%d", len(segment.Frames), segment.Bytes)
	if err := recognizer.Start(ctx); err != nil {
		return fmt.Errorf("start ASR segment: %w", err)
	}

	for _, frame := range segment.Frames {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if len(frame) == 0 {
			continue
		}
		if err := recognizer.SendAudio(ctx, frame); err != nil {
			return fmt.Errorf("send ASR segment audio: %w", err)
		}
	}

	if err := recognizer.Finish(ctx); err != nil {
		return fmt.Errorf("finish ASR segment: %w", err)
	}

	return nil
}

func (p *InPipe) GetState() InPipeState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func logError(format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	logging.Errorf("%s", msg)
	return fmt.Errorf("%s", msg)
}
