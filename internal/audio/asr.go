package audio

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	lastActive  time.Time // 最后一次检测到人声的时间，用于长静音后 Reset VAD
	inSpeech    bool      // 当前是否处于人声活跃中
	vadResetDur time.Duration

	// VAD 与 ASR 通信 channel（VAD 不等待 ASR，只负责发信号）
	asrStartCh  chan struct{} // VAD 检测到人声开始
	asrFrameCh  chan []byte   // 实时音频帧
	asrFinishCh chan struct{} // VAD 切段（静音结束），触发 ASR Finish
	speechCh    chan struct{} // 人声活跃心跳，供 coalescence 等待用

	silenceTimeout time.Duration // ASR final 后等待新人声的窗口（= VADMinSilenceMs）
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
		cfg:            cfg,
		recognizer:     cfg.Recognizer,
		vadEnabled:     cfg.EnableVAD && seg != nil,
		segmenter:      seg,
		vadResetDur:    time.Duration(cfg.VADMinSilenceMs*16) * time.Millisecond,
		silenceTimeout: time.Duration(150) * time.Millisecond,
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

	if !p.vadEnabled {
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
		if err := p.recognizer.Start(p.ctx); err != nil {
			p.cancel()
			return fmt.Errorf("ASRProcessor: start recognizer: %w", err)
		}
	} else {
		p.asrStartCh = make(chan struct{}, 1)
		p.asrFrameCh = make(chan []byte, 128) // ~3.8s buffer at 30ms/frame
		p.asrFinishCh = make(chan struct{}, 1)
		p.speechCh = make(chan struct{}, 1)
		p.asrWG.Add(1)
		go func() {
			defer p.asrWG.Done()
			p.runASRLoop(p.ctx)
		}()
	}

	p.started = true
	p.lastActive = time.Now()
	logging.Infof("ASRProcessor: started (VAD=%v, silence_timeout=%v)", p.vadEnabled, p.silenceTimeout)
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
		p.lastActive = now
		p.inSpeech = true
		p.speechMu.Unlock()
		logging.Infof("ASRProcessor: VAD speech started (frame_bytes=%d)", len(audio))

		// 通知 ASR 任务开始
		select {
		case p.asrStartCh <- struct{}{}:
		default:
		}
		// 实时发首帧
		select {
		case p.asrFrameCh <- cloneBytes(audio):
		default:
			logging.Warnf("ASRProcessor: audio frame dropped (asrFrameCh full)")
		}
		// 发 speech 心跳（coalescence 等待用）
		select {
		case p.speechCh <- struct{}{}:
		default:
		}

		p.mu.Lock()
		fn := p.onSpeech
		p.mu.Unlock()
		if fn != nil {
			fn()
		}
		// started=true 时 seg 必然为 nil，直接返回
		return
	}

	if seg == nil || seg.Bytes == 0 {
		// 长静音后重置 VAD 状态，防止 RNN 状态漂移导致小声说话无法检测
		p.speechMu.Lock()
		inSpeech := p.inSpeech
		if !inSpeech && time.Since(p.lastActive) > p.vadResetDur {
			p.segmenter.Reset()
			p.lastActive = time.Now()
			logging.Infof("ASRProcessor: VAD reset (silence > %v)", p.vadResetDur)
		}
		p.speechMu.Unlock()

		if inSpeech {
			// 用户正在说话（VAD 尚未切段），实时发帧 + 心跳
			select {
			case p.asrFrameCh <- cloneBytes(audio):
			default:
				logging.Warnf("ASRProcessor: audio frame dropped (asrFrameCh full)")
			}
			select {
			case p.speechCh <- struct{}{}:
			default:
			}
		}
		return
	}

	// 切段了（静音达到阈值），语音段结束
	now := time.Now()
	p.speechMu.Lock()
	p.lastActive = now
	p.inSpeech = false
	segmentReadyAt := now
	speechStart := p.speechStart
	p.speechStart = time.Time{}
	p.speechMu.Unlock()

	if speechStart.IsZero() {
		logging.Infof("ASRProcessor: VAD segment end (bytes=%d)", seg.Bytes)
	} else {
		logging.Infof("ASRProcessor: VAD segment end (bytes=%d, speech_duration=%v)",
			seg.Bytes, segmentReadyAt.Sub(speechStart))
	}

	// 通知 ASR 结束当前任务
	select {
	case p.asrFinishCh <- struct{}{}:
	default:
	}
}

// runASRLoop 管理 ASR 会话生命周期，并在完成后执行 coalescence 等待。
func (p *asrProcessor) runASRLoop(ctx context.Context) {
	var accumulated []string
	startConsumed := false // waitForMoreSpeech 是否已经消费了下一个 start 信号

	for {
		if !startConsumed {
			select {
			case <-ctx.Done():
				return
			case <-p.asrStartCh:
			}
		}
		startConsumed = false

		text, cancelled := p.runOneSession(ctx)
		if cancelled {
			if len(accumulated) > 0 {
				p.emitResult(strings.Join(accumulated, ""))
			}
			return
		}
		if text != "" {
			accumulated = append(accumulated, text)
		}

		// 等待 silenceTimeout，期间监听人声活动；如果有新 start 则继续合并
		if p.waitForMoreSpeech(ctx) {
			startConsumed = true
			continue
		}

		if len(accumulated) > 0 {
			finalText := strings.Join(accumulated, "")
			logging.Infof("ASRProcessor: emitting result (text_len=%d, merged_segments=%d)",
				len([]rune(finalText)), len(accumulated))
			p.emitResult(finalText)
			accumulated = accumulated[:0]
		}
	}
}

// runOneSession 执行一次完整的 ASR 会话：Start → 实时 SendAudio → Finish。
// 返回识别文本，以及是否因 ctx 取消而退出。
func (p *asrProcessor) runOneSession(ctx context.Context) (string, bool) {
	var mu sync.Mutex
	var texts []string
	p.recognizer.OnResult(func(result asr.Result) {
		logging.Infof("ASRProcessor: result received (final=%v, text_len=%d, usage_duration=%s, begin_ms=%d, end_ms=%s)",
			result.IsFinal,
			len([]rune(result.Text)),
			formatOptionalInt(result.UsageDuration),
			result.BeginTimeMs,
			formatOptionalInt64(result.EndTimeMs),
		)
		if result.IsFinal && result.Text != "" {
			mu.Lock()
			texts = append(texts, result.Text)
			mu.Unlock()
		}
	})

	startAt := time.Now()
	logging.Infof("ASRProcessor: ASR task starting")
	if err := p.recognizer.Start(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			return "", true
		}
		logging.Errorf("ASRProcessor: start error: %v", err)
		p.drainUntilFinish(ctx)
		return "", false
	}
	logging.Infof("ASRProcessor: recognizer started in %v", time.Since(startAt))

	// 实时发送音频帧，直到 VAD 切段信号（asrFinishCh）
	sentBytes, sentFrames := 0, 0
	sendStart := time.Now()
loop:
	for {
		select {
		case frame := <-p.asrFrameCh:
			if err := p.recognizer.SendAudio(ctx, frame); err != nil {
				if errors.Is(err, context.Canceled) {
					return "", true
				}
				logging.Errorf("ASRProcessor: send audio error: %v", err)
			}
			sentBytes += len(frame)
			sentFrames++
		case <-p.asrFinishCh:
			break loop
		case <-ctx.Done():
			return "", true
		}
	}
	logging.Infof("ASRProcessor: sent audio in %v (bytes=%d, frames=%d)", time.Since(sendStart), sentBytes, sentFrames)

	// flush asrFrameCh 中残留的帧（finish 信号和最后几帧可能同时到达）
	flushed := 0
	for {
		select {
		case frame := <-p.asrFrameCh:
			_ = p.recognizer.SendAudio(ctx, frame)
			flushed++
		default:
			goto doFinish
		}
	}
doFinish:
	if flushed > 0 {
		logging.Infof("ASRProcessor: flushed %d trailing frames", flushed)
	}

	finishStart := time.Now()
	if err := p.recognizer.Finish(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			return "", true
		}
		logging.Errorf("ASRProcessor: finish error: %v", err)
		return "", false
	}
	logging.Infof("ASRProcessor: recognizer finished in %v", time.Since(finishStart))

	mu.Lock()
	text := strings.Join(texts, "")
	mu.Unlock()
	return text, false
}

// waitForMoreSpeech 在 ASR 完成后等待 silenceTimeout，
// 期间检测到人声活动则重置计时器，检测到新 start 则返回 true（继续合并）。
func (p *asrProcessor) waitForMoreSpeech(ctx context.Context) bool {
	timer := time.NewTimer(p.silenceTimeout)
	defer timer.Stop()

	resetTimer := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(p.silenceTimeout)
	}

	for {
		select {
		case <-p.speechCh:
			// 人声活跃，重置静音计时器
			resetTimer()
		case <-p.asrStartCh:
			// 新的人声开始，告知外层继续合并
			return true
		case <-timer.C:
			return false
		case <-ctx.Done():
			return false
		}
	}
}

// drainUntilFinish 在 ASR Start 失败时，消耗 channel 直到 VAD 切段信号。
func (p *asrProcessor) drainUntilFinish(ctx context.Context) {
	for {
		select {
		case <-p.asrFrameCh:
		case <-p.asrFinishCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (p *asrProcessor) emitResult(text string) {
	p.mu.Lock()
	fn := p.onResult
	p.mu.Unlock()
	if fn != nil {
		fn(ASRResult{Text: text, IsFinal: true})
	}
}

func cloneBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	c := make([]byte, len(b))
	copy(c, b)
	return c
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
