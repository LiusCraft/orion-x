package audio

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/audio/vad"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/provider/asr"
)

// ASRResult holds the output from speech recognition.
type ASRResult struct {
	Text    string
	IsFinal bool
}

// ASRConfig configures an ASRProcessor.
type ASRConfig struct {
	EnableVAD       bool
	VADThreshold    float64
	VADType         string
	VADModelPath    string
	VADMinSilenceMs int
	VADSpeechPadMs  int
	// Recognizer is the ASR backend. Required.
	Recognizer asr.Recognizer
}

// DefaultASRConfig returns a sensible default. Recognizer must be set by caller.
func DefaultASRConfig() *ASRConfig {
	return &ASRConfig{
		EnableVAD:       true,
		VADThreshold:    0.5,
		VADType:         string(vad.TypeSilero),
		VADModelPath:    vad.DefaultModelPath,
		VADMinSilenceMs: 500,
		VADSpeechPadMs:  300,
	}
}

// ASRProcessor receives PCM16LE 16kHz mono audio via Write, performs VAD + ASR
// internally, and delivers results via OnResult. It has no knowledge of where
// the audio comes from.
type ASRProcessor interface {
	// Write pushes a chunk of PCM16LE 16kHz mono bytes into the processor.
	Write(data []byte) error
	// OnResult registers the callback for ASR results.
	OnResult(func(ASRResult))
	// OnSpeechStart registers the callback triggered when speech onset is detected.
	OnSpeechStart(func())
	// Start initializes workers. Must be called before Write.
	Start(ctx context.Context) error
	// Stop flushes pending audio and shuts down.
	Stop() error
}

// NewASRProcessor creates an ASRProcessor. cfg.Recognizer must not be nil.
func NewASRProcessor(cfg *ASRConfig) (ASRProcessor, error) {
	return newASRProcessor(cfg)
}

type asrProcessor struct {
	cfg        *ASRConfig
	recognizer asr.Recognizer
	onResult   func(ASRResult)
	onSpeech   func()
	vadEnabled bool
	segmenter  vad.Segmenter
	ctx        context.Context
	cancel     context.CancelFunc
	asrWG      sync.WaitGroup
	mu         sync.Mutex
	started    bool

	speechMu    sync.Mutex
	speechStart time.Time
}

func newASRProcessor(cfg *ASRConfig) (*asrProcessor, error) {
	if cfg == nil {
		cfg = DefaultASRConfig()
	}
	if cfg.Recognizer == nil {
		return nil, fmt.Errorf("ASRProcessor: Recognizer is required")
	}

	var seg vad.Segmenter
	var err error
	if cfg.EnableVAD && cfg.VADType == string(vad.TypeSilero) {
		seg, err = vad.NewSegmenterWithConfig(vad.SegmenterConfig{
			SampleRate:   InternalSampleRate,
			Threshold:    cfg.VADThreshold,
			MinSilenceMs: cfg.VADMinSilenceMs,
			SpeechPadMs:  cfg.VADSpeechPadMs,
			ModelPath:    cfg.VADModelPath,
		})
		if err != nil {
			logging.Warnf("ASRProcessor: failed to create VAD segmenter: %v, VAD disabled", err)
		}
	}

	return &asrProcessor{
		cfg:        cfg,
		recognizer: cfg.Recognizer,
		vadEnabled: cfg.EnableVAD && seg != nil,
		segmenter:  seg,
	}, nil
}

func (p *asrProcessor) OnResult(fn func(ASRResult)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onResult = fn
}

func (p *asrProcessor) OnSpeechStart(fn func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onSpeech = fn
}

func (p *asrProcessor) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return fmt.Errorf("ASRProcessor: already started")
	}

	p.ctx, p.cancel = context.WithCancel(ctx)

	p.recognizer.OnResult(func(result asr.Result) {
		logging.Infof("ASRProcessor: result received (final=%v, text_len=%d, usage_duration=%s, begin_ms=%d, end_ms=%s)",
			result.IsFinal,
			len([]rune(result.Text)),
			formatOptionalInt(result.UsageDuration),
			result.BeginTimeMs,
			formatOptionalInt64(result.EndTimeMs),
		)
		p.mu.Lock()
		fn := p.onResult
		p.mu.Unlock()
		if fn != nil {
			fn(ASRResult{Text: result.Text, IsFinal: result.IsFinal})
		}
	})

	if !p.vadEnabled {
		if err := p.recognizer.Start(p.ctx); err != nil {
			p.cancel()
			return fmt.Errorf("ASRProcessor: start recognizer: %w", err)
		}
	}

	p.started = true
	logging.Infof("ASRProcessor: started (VAD=%v)", p.vadEnabled)
	return nil
}

func (p *asrProcessor) Stop() error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return nil
	}
	recognizer := p.recognizer
	segmenter := p.segmenter
	ctx := p.ctx
	cancel := p.cancel
	vadEnabled := p.vadEnabled
	p.mu.Unlock()

	logging.Infof("ASRProcessor: stopping...")

	if vadEnabled {
		if segmenter != nil {
			if seg := segmenter.Flush(); seg != nil && seg.Bytes > 0 {
				_ = p.recognizeSegment(ctx, *seg)
			}
		}
		if cancel != nil {
			cancel()
		}
		p.asrWG.Wait()
		if recognizer != nil {
			_ = recognizer.Close()
		}
	} else {
		if cancel != nil {
			cancel()
		}
		if recognizer != nil {
			_ = recognizer.Finish(ctx)
			_ = recognizer.Close()
		}
	}

	if segmenter != nil {
		_ = segmenter.Close()
	}

	p.mu.Lock()
	p.started = false
	p.mu.Unlock()

	logging.Infof("ASRProcessor: stopped")
	return nil
}

func (p *asrProcessor) Write(data []byte) error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return nil // silent drop before start
	}
	ctx := p.ctx
	vadEnabled := p.vadEnabled
	p.mu.Unlock()

	if vadEnabled {
		p.processVAD(ctx, data)
		return nil
	}

	if err := p.recognizer.SendAudio(ctx, data); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return nil
		}
		return fmt.Errorf("ASRProcessor: send audio: %w", err)
	}
	return nil
}

func (p *asrProcessor) processVAD(ctx context.Context, audio []byte) {
	seg, started := p.segmenter.Process(audio)
	if started {
		now := time.Now()
		p.speechMu.Lock()
		p.speechStart = now
		p.speechMu.Unlock()
		logging.Infof("ASRProcessor: VAD speech started (frame_bytes=%d)", len(audio))

		p.mu.Lock()
		fn := p.onSpeech
		p.mu.Unlock()
		if fn != nil {
			fn()
		}
	}
	if seg == nil || seg.Bytes == 0 {
		return
	}

	segmentReadyAt := time.Now()
	p.speechMu.Lock()
	speechStart := p.speechStart
	p.speechStart = time.Time{}
	p.speechMu.Unlock()
	if speechStart.IsZero() {
		logging.Infof("ASRProcessor: VAD segment ready (bytes=%d, frames=%d)", seg.Bytes, len(seg.Frames))
	} else {
		logging.Infof("ASRProcessor: VAD segment ready (bytes=%d, frames=%d, speech_duration=%v)",
			seg.Bytes, len(seg.Frames), segmentReadyAt.Sub(speechStart))
	}

	p.asrWG.Add(1)
	go func() {
		defer p.asrWG.Done()
		if err := p.recognizeSegment(ctx, *seg); err != nil {
			if !errors.Is(err, context.Canceled) {
				logging.Errorf("ASRProcessor: recognize segment error: %v", err)
			}
		}
	}()
}

func (p *asrProcessor) recognizeSegment(ctx context.Context, segment vad.Segment) error {
	if ctx == nil {
		ctx = context.Background()
	}

	totalStart := time.Now()
	logging.Infof("ASRProcessor: ASR task starting (bytes=%d, frames=%d)", segment.Bytes, len(segment.Frames))

	startAt := time.Now()
	if err := p.recognizer.Start(ctx); err != nil {
		return fmt.Errorf("start segment: %w", err)
	}
	logging.Infof("ASRProcessor: recognizer started in %v", time.Since(startAt))

	sendStart := time.Now()
	sentFrames := 0
	sentBytes := 0
	for _, frame := range segment.Frames {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if len(frame) == 0 {
			continue
		}
		if err := p.recognizer.SendAudio(ctx, frame); err != nil {
			return fmt.Errorf("send segment audio: %w", err)
		}
		sentFrames++
		sentBytes += len(frame)
	}
	logging.Infof("ASRProcessor: sent segment audio in %v (bytes=%d, frames=%d)", time.Since(sendStart), sentBytes, sentFrames)

	finishStart := time.Now()
	if err := p.recognizer.Finish(ctx); err != nil {
		return err
	}
	logging.Infof("ASRProcessor: recognizer finished in %v (total=%v)", time.Since(finishStart), time.Since(totalStart))
	return nil
}

func formatOptionalInt(v *int) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf("%d", *v)
}

func formatOptionalInt64(v *int64) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf("%d", *v)
}
