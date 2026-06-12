package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/provider/asr"
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
	readWG      sync.WaitGroup
	asrWG       sync.WaitGroup
	mu          sync.Mutex

	vadEnabled     bool
	vadDetector    VADDetector
	levelMonitor   *AudioLevelMonitor
	vadMinInterval time.Duration
	lastVADTime    time.Time
	lastLevelLog   time.Time

	segments chan audioSegment

	segmentMu       sync.Mutex
	segmentActive   bool
	segmentFrames   [][]byte
	segmentBytes    int
	preSpeechFrames [][]byte
	preSpeechBytes  int
	preSpeechMax    int
}

type audioSegment struct {
	frames [][]byte
	bytes  int
}

func NewInPipeWithRecognizer(config *InPipeConfig, recognizer asr.Recognizer) AudioInPipe {
	if config == nil {
		config = DefaultInPipeConfig()
	}
	vadDetector, err := NewVADDetector(config)
	if err != nil {
		logging.Warnf("AudioInPipe: failed to create VAD detector: %v, VAD will be disabled", err)
		vadDetector = &noopVAD{}
	}
	return &inPipeImpl{
		state:          InPipeStateIdle,
		config:         config,
		recognizer:     recognizer,
		vadEnabled:     config.EnableVAD && !isNoopVAD(vadDetector),
		vadDetector:    vadDetector,
		levelMonitor:   NewAudioLevelMonitor(config.SampleRate),
		vadMinInterval: 300 * time.Millisecond,
	}
}

func (p *inPipeImpl) SetAudioSource(source AudioSource) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.audioSource = source
}

// OnTTSPlaybackStarted 冻结噪声基准，防止 TTS 播放音频泄漏污染估计
func (p *inPipeImpl) OnTTSPlaybackStarted() {
	if p.levelMonitor != nil {
		p.levelMonitor.PauseForTTS()
	}
}

// OnTTSPlaybackStopped 恢复噪声基准更新
func (p *inPipeImpl) OnTTSPlaybackStopped() {
	if p.levelMonitor != nil {
		p.levelMonitor.ResumeFromTTS()
	}
}

func (p *inPipeImpl) Start(ctx context.Context) error {
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
		p.resetVADSegmentLocked()
		p.segments = make(chan audioSegment, 8)
		p.asrWG.Add(1)
		go p.processASRSegments(p.ctx, p.segments)
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
	vadDetector := p.vadDetector
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
		p.flushPendingVADSegment(ctx)
		p.closeASRSegments()
		logging.Infof("AudioInPipe: waiting for ASR segment worker to finish...")
		p.waitForASRWorker(cancel, recognizer)
		if cancel != nil {
			logging.Infof("AudioInPipe: canceling context...")
			cancel()
		}
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
	if vadDetector != nil {
		if err := vadDetector.Close(); err != nil {
			logging.Warnf("AudioInPipe: close VAD detector failed: %v", err)
		}
	}

	logging.Infof("AudioInPipe: all goroutines finished")

	p.mu.Lock()
	p.state = InPipeStateIdle
	logging.Infof("AudioInPipe: stopped, state: %s", p.state)
	p.mu.Unlock()
	return nil
}

func (p *inPipeImpl) waitForASRWorker(cancel context.CancelFunc, recognizer asr.Recognizer) {
	done := make(chan struct{})
	go func() {
		p.asrWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		return
	case <-time.After(5 * time.Second):
		logging.Warnf("AudioInPipe: ASR segment worker did not stop in time, canceling context")
		if cancel != nil {
			cancel()
		}
		if recognizer != nil {
			_ = recognizer.Close()
		}
		<-done
	}
}

func (p *inPipeImpl) SendAudio(audio []byte) error {
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

	p.observeAudioLevel(audio)

	if err := recognizer.SendAudio(ctx, audio); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
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
	defer p.readWG.Done()

	logging.Infof("AudioInPipe: audio reader goroutine started")
	defer logging.Infof("AudioInPipe: audio reader goroutine stopped")
	defer func() {
		if p.vadEnabled {
			p.flushPendingVADSegment(ctx)
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

func (p *inPipeImpl) processASRSegments(ctx context.Context, segments <-chan audioSegment) {
	defer p.asrWG.Done()

	logging.Infof("AudioInPipe: ASR segment worker started")
	defer logging.Infof("AudioInPipe: ASR segment worker stopped")

	for segment := range segments {
		if segment.bytes == 0 || len(segment.frames) == 0 {
			continue
		}
		if err := p.recognizeSegment(ctx, segment); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				return
			}
			logging.Errorf("AudioInPipe: ASR segment failed: %v", err)
		}
	}
}

func (p *inPipeImpl) recognizeSegment(ctx context.Context, segment audioSegment) error {
	p.mu.Lock()
	recognizer := p.recognizer
	p.mu.Unlock()

	if recognizer == nil {
		return logError("AudioInPipe: recognizer not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	logging.Infof("AudioInPipe: recognizing VAD segment frames=%d bytes=%d", len(segment.frames), segment.bytes)
	if err := recognizer.Start(ctx); err != nil {
		return fmt.Errorf("start ASR segment: %w", err)
	}

	for _, frame := range segment.frames {
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

func (p *inPipeImpl) handleASRResult(result asr.Result) {
	p.mu.Lock()
	handler := p.asrHandler
	p.mu.Unlock()

	if handler != nil {
		handler(result.Text, result.IsFinal)
	}
}

func (p *inPipeImpl) handleVADAudio(ctx context.Context, audio []byte) {
	level := p.observeAudioLevel(audio)
	if !p.vadEnabled {
		return
	}

	p.segmentMu.Lock()
	active := p.segmentActive
	p.segmentMu.Unlock()

	if level.Silent && !active {
		p.rememberPreSpeech(audio)
		return
	}

	isSpeech := p.detectSpeech(audio)
	started, segment := p.updateVADSegment(audio, isSpeech)
	if started {
		p.triggerVADHandler()
	}
	if segment.bytes > 0 {
		p.enqueueASRSegment(ctx, segment)
	}
}

func (p *inPipeImpl) triggerVADHandler() {
	logging.Infof("AudioInPipe: VAD detected speech")

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
		logging.Infof("AudioInPipe: VAD handler is nil")
		return
	}
	if now.Sub(last) < minInterval {
		logging.Infof("AudioInPipe: VAD throttled (last: %v, interval: %v)", now.Sub(last), minInterval)
		return
	}

	logging.Infof("AudioInPipe: VAD triggering user speaking detected")
	handler()
}

func (p *inPipeImpl) updateVADSegment(audio []byte, isSpeech bool) (bool, audioSegment) {
	p.segmentMu.Lock()
	defer p.segmentMu.Unlock()

	if isSpeech {
		if !p.segmentActive {
			p.segmentActive = true
			if p.preSpeechBytes > 0 {
				p.segmentFrames = append(p.segmentFrames, p.preSpeechFrames...)
				p.segmentBytes += p.preSpeechBytes
			}
			p.preSpeechFrames = nil
			p.preSpeechBytes = 0
			p.appendSegmentFrameLocked(audio)
			return true, audioSegment{}
		}
		p.appendSegmentFrameLocked(audio)
		return false, audioSegment{}
	}

	if p.segmentActive {
		segment := p.takeSegmentLocked()
		return false, segment
	}

	p.rememberPreSpeechLocked(audio)
	return false, audioSegment{}
}

func (p *inPipeImpl) appendSegmentFrameLocked(audio []byte) {
	if len(audio) == 0 {
		return
	}
	frame := cloneBytes(audio)
	p.segmentFrames = append(p.segmentFrames, frame)
	p.segmentBytes += len(frame)
}

func (p *inPipeImpl) rememberPreSpeech(audio []byte) {
	p.segmentMu.Lock()
	defer p.segmentMu.Unlock()
	p.rememberPreSpeechLocked(audio)
}

func (p *inPipeImpl) rememberPreSpeechLocked(audio []byte) {
	if len(audio) == 0 || p.preSpeechMax <= 0 {
		return
	}

	frame := cloneBytes(audio)
	p.preSpeechFrames = append(p.preSpeechFrames, frame)
	p.preSpeechBytes += len(frame)
	for p.preSpeechBytes > p.preSpeechMax && len(p.preSpeechFrames) > 0 {
		p.preSpeechBytes -= len(p.preSpeechFrames[0])
		p.preSpeechFrames[0] = nil
		p.preSpeechFrames = p.preSpeechFrames[1:]
	}
}

func (p *inPipeImpl) takeSegmentLocked() audioSegment {
	segment := audioSegment{
		frames: p.segmentFrames,
		bytes:  p.segmentBytes,
	}
	p.segmentActive = false
	p.segmentFrames = nil
	p.segmentBytes = 0
	return segment
}

func (p *inPipeImpl) flushPendingVADSegment(ctx context.Context) {
	p.segmentMu.Lock()
	if !p.segmentActive {
		p.segmentMu.Unlock()
		return
	}
	segment := p.takeSegmentLocked()
	p.segmentMu.Unlock()

	if segment.bytes > 0 {
		logging.Infof("AudioInPipe: flushing pending VAD segment frames=%d bytes=%d", len(segment.frames), segment.bytes)
		p.enqueueASRSegment(ctx, segment)
	}
}

func (p *inPipeImpl) enqueueASRSegment(ctx context.Context, segment audioSegment) {
	if segment.bytes == 0 || len(segment.frames) == 0 {
		return
	}

	p.mu.Lock()
	segments := p.segments
	p.mu.Unlock()
	if segments == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case segments <- segment:
		logging.Infof("AudioInPipe: queued VAD segment frames=%d bytes=%d", len(segment.frames), segment.bytes)
	case <-ctx.Done():
	}
}

func (p *inPipeImpl) closeASRSegments() {
	p.mu.Lock()
	segments := p.segments
	p.segments = nil
	p.mu.Unlock()
	if segments != nil {
		close(segments)
	}
}

func (p *inPipeImpl) resetVADSegmentLocked() {
	p.segmentMu.Lock()
	defer p.segmentMu.Unlock()

	p.segmentActive = false
	p.segmentFrames = nil
	p.segmentBytes = 0
	p.preSpeechFrames = nil
	p.preSpeechBytes = 0
	p.preSpeechMax = p.preSpeechMaxBytes()
}

func (p *inPipeImpl) preSpeechMaxBytes() int {
	if p.config == nil || p.config.VADSpeechPadMs <= 0 {
		return 0
	}
	sampleRate := p.config.SampleRate
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	channels := p.config.Channels
	if channels <= 0 {
		channels = 1
	}
	return sampleRate * channels * 2 * p.config.VADSpeechPadMs / 1000
}

func (p *inPipeImpl) observeAudioLevel(audio []byte) AudioLevelSnapshot {
	if p.levelMonitor == nil {
		sampleRate := 16000
		if p.config != nil && p.config.SampleRate > 0 {
			sampleRate = p.config.SampleRate
		}
		p.levelMonitor = NewAudioLevelMonitor(sampleRate)
	}

	level := p.levelMonitor.Observe(audio)

	p.mu.Lock()
	now := time.Now()
	shouldLog := now.Sub(p.lastLevelLog) >= 3*time.Second
	if shouldLog {
		p.lastLevelLog = now
	}
	p.mu.Unlock()

	if shouldLog || level.Clipping || level.Noisy {
		logging.Infof(
			"AudioInPipe: audio level rms=%.4f peak=%.4f noise_floor=%.4f clipping=%.4f silent=%v above_noise=%v noisy=%v",
			level.RMS,
			level.Peak,
			level.NoiseFloor,
			level.ClippingRatio,
			level.Silent,
			level.AboveNoiseFloor,
			level.Noisy,
		)
	}
	if level.Clipping {
		logging.Warnf("AudioInPipe: audio input appears clipped (peak=%.4f clipping=%.4f)", level.Peak, level.ClippingRatio)
	}
	if level.Noisy {
		logging.Warnf("AudioInPipe: audio input noise floor is high (noise_floor=%.4f)", level.NoiseFloor)
	}

	return level
}

func (p *inPipeImpl) detectSpeech(audio []byte) bool {
	if p.vadDetector == nil {
		return false
	}

	detected, err := p.vadDetector.Detect(audio)
	if err != nil {
		logging.Warnf("AudioInPipe: VAD detection error: %v", err)
		return false
	}

	// Throttled logging
	p.mu.Lock()
	now := time.Now()
	if detected && now.Sub(p.lastVADTime) >= 1*time.Second {
		logging.Infof("AudioInPipe: VAD detected speech")
	}
	p.mu.Unlock()

	return detected
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

func cloneBytes(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	copied := make([]byte, len(data))
	copy(copied, data)
	return copied
}

func isNoopVAD(detector VADDetector) bool {
	_, ok := detector.(*noopVAD)
	return ok
}
