package audio

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/audio/vad"
	"github.com/liuscraft/orion-x/internal/provider/asr"
)

// --- mocks ---

type mockRecognizer struct {
	mu              sync.Mutex
	startCalls      int
	finishCalls     int
	closeCalls      int
	sendAudioCalls  [][]byte
	startErr        error
	sendErr         error
	finishErr       error
	onResultHandler func(asr.Result)
	emitOnFinish    bool // if true, Finish emits a final result before returning
}

func newMockRecognizer() *mockRecognizer { return &mockRecognizer{} }

func (r *mockRecognizer) Start(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startCalls++
	return r.startErr
}

func (r *mockRecognizer) SendAudio(_ context.Context, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	r.sendAudioCalls = append(r.sendAudioCalls, cp)
	return r.sendErr
}

func (r *mockRecognizer) Finish(_ context.Context) error {
	r.mu.Lock()
	r.finishCalls++
	handler := r.onResultHandler
	emit := r.emitOnFinish
	err := r.finishErr
	r.mu.Unlock()

	if emit && handler != nil {
		handler(asr.Result{Text: "hello", IsFinal: true})
	}
	return err
}

func (r *mockRecognizer) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closeCalls++
	return nil
}

func (r *mockRecognizer) OnResult(handler func(asr.Result)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onResultHandler = handler
}

func (r *mockRecognizer) emitResult(result asr.Result) {
	r.mu.Lock()
	handler := r.onResultHandler
	r.mu.Unlock()
	if handler != nil {
		handler(result)
	}
}

func (r *mockRecognizer) getStartCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.startCalls
}

func (r *mockRecognizer) getFinishCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.finishCalls
}

func (r *mockRecognizer) getSendAudioCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sendAudioCalls)
}

type mockSegmenter struct {
	mu          sync.Mutex
	processFunc func(audio []byte) (*vad.Segment, bool)
	flushResult *vad.Segment
	closeCalled bool
}

func (s *mockSegmenter) Process(audio []byte) (*vad.Segment, bool) {
	s.mu.Lock()
	fn := s.processFunc
	s.mu.Unlock()
	if fn != nil {
		return fn(audio)
	}
	return nil, false
}

func (s *mockSegmenter) Flush() *vad.Segment {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flushResult
}

func (s *mockSegmenter) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeCalled = true
	return nil
}

func (s *mockSegmenter) Reset() {}

// --- helpers ---

func newTestASRConfig(r *mockRecognizer) *ASRConfig {
	return &ASRConfig{EnableVAD: false, Recognizer: r}
}

// newTestASRProcessorWithVAD constructs an asrProcessor with VAD enabled and
// a pre-injected mockSegmenter, bypassing Silero model loading.
func newTestASRProcessorWithVAD(r *mockRecognizer, seg *mockSegmenter) *asrProcessor {
	return &asrProcessor{
		cfg:        &ASRConfig{EnableVAD: true, Recognizer: r},
		recognizer: r,
		vadEnabled: true,
		segmenter:  seg,
	}
}

// --- tests ---

func TestASRProcessorCreate(t *testing.T) {
	r := newMockRecognizer()
	proc, err := NewASRProcessor(newTestASRConfig(r))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proc == nil {
		t.Fatal("expected non-nil processor")
	}

	_, err = NewASRProcessor(&ASRConfig{Recognizer: nil})
	if err == nil {
		t.Fatal("expected error when Recognizer is nil")
	}
}

func TestASRProcessorStartStop(t *testing.T) {
	r := newMockRecognizer()
	proc, _ := NewASRProcessor(newTestASRConfig(r))

	if err := proc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if err := proc.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestASRProcessorDoubleStart(t *testing.T) {
	r := newMockRecognizer()
	proc, _ := NewASRProcessor(newTestASRConfig(r))
	ctx := context.Background()

	if err := proc.Start(ctx); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	if err := proc.Start(ctx); err == nil {
		t.Fatal("expected error on double Start")
	}
}

func TestASRProcessorWriteBeforeStart(t *testing.T) {
	r := newMockRecognizer()
	proc, _ := NewASRProcessor(newTestASRConfig(r))

	if err := proc.Write([]byte{0x00, 0x01}); err != nil {
		t.Fatalf("Write before Start should not error: %v", err)
	}
	if r.getSendAudioCount() != 0 {
		t.Fatal("expected no SendAudio calls before Start")
	}
}

// TestASRProcessorOnResult_NoVAD verifies the OnResult callback fires when the
// recognizer emits a result (VAD disabled, direct SendAudio path).
func TestASRProcessorOnResult_NoVAD(t *testing.T) {
	r := newMockRecognizer()
	proc, _ := NewASRProcessor(newTestASRConfig(r))

	resultCh := make(chan ASRResult, 1)
	proc.OnResult(func(res ASRResult) { resultCh <- res })

	if err := proc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	if err := proc.Write(make([]byte, 320)); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	r.emitResult(asr.Result{Text: "hello world", IsFinal: true})

	select {
	case got := <-resultCh:
		if got.Text != "hello world" || !got.IsFinal {
			t.Errorf("unexpected result: %+v", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for OnResult")
	}
}

// TestASRProcessorOnResult_VAD verifies the OnResult callback fires when a VAD
// segment triggers recognizeSegment internally.
func TestASRProcessorOnResult_VAD(t *testing.T) {
	r := newMockRecognizer()
	r.emitOnFinish = true

	var once sync.Once
	seg := &mockSegmenter{
		processFunc: func(audio []byte) (*vad.Segment, bool) {
			var s *vad.Segment
			once.Do(func() {
				s = &vad.Segment{Frames: [][]byte{audio}, Bytes: len(audio)}
			})
			return s, false
		},
	}

	proc := newTestASRProcessorWithVAD(r, seg)

	resultCh := make(chan ASRResult, 1)
	proc.OnResult(func(res ASRResult) {
		select {
		case resultCh <- res:
		default:
		}
	})

	if err := proc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	if err := proc.Write(make([]byte, 320)); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	select {
	case got := <-resultCh:
		if got.Text != "hello" || !got.IsFinal {
			t.Errorf("unexpected result: %+v", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for OnResult via VAD segment")
	}
}

// TestASRProcessorOnSpeechStart verifies OnSpeechStart fires when the segmenter
// signals a new speech segment has started.
func TestASRProcessorOnSpeechStart(t *testing.T) {
	r := newMockRecognizer()
	seg := &mockSegmenter{
		processFunc: func(audio []byte) (*vad.Segment, bool) {
			return nil, true // always signal speech start
		},
	}

	proc := newTestASRProcessorWithVAD(r, seg)

	speechCh := make(chan struct{}, 1)
	proc.OnSpeechStart(func() {
		select {
		case speechCh <- struct{}{}:
		default:
		}
	})

	if err := proc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	if err := proc.Write(make([]byte, 320)); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	select {
	case <-speechCh:
		// ok
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for OnSpeechStart")
	}
}

// TestASRProcessorStop_FlushesSegment verifies that Stop flushes any buffered
// VAD segment and runs ASR on it before returning.
func TestASRProcessorStop_FlushesSegment(t *testing.T) {
	r := newMockRecognizer()
	r.emitOnFinish = true

	seg := &mockSegmenter{
		flushResult: &vad.Segment{Frames: [][]byte{make([]byte, 320)}, Bytes: 320},
	}

	proc := newTestASRProcessorWithVAD(r, seg)

	resultCh := make(chan ASRResult, 1)
	proc.OnResult(func(res ASRResult) {
		select {
		case resultCh <- res:
		default:
		}
	})

	if err := proc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if err := proc.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// recognizeSegment is synchronous inside Stop, so the channel should be
	// ready before the select.
	select {
	case got := <-resultCh:
		if !got.IsFinal {
			t.Errorf("expected IsFinal result, got %+v", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout: Flush segment did not trigger recognizeSegment")
	}
}

// TestASRProcessorContextCancel verifies that Write after context cancellation
// does not return an error (context.Canceled is silently swallowed).
func TestASRProcessorContextCancel(t *testing.T) {
	r := newMockRecognizer()
	proc, _ := NewASRProcessor(newTestASRConfig(r))

	ctx, cancel := context.WithCancel(context.Background())
	if err := proc.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	cancel()
	time.Sleep(50 * time.Millisecond)

	if err := proc.Write(make([]byte, 320)); err != nil {
		t.Errorf("Write after context cancel should not error: %v", err)
	}

	_ = proc.Stop()
}

// TestASRProcessorRecognizerStartError verifies that Start returns a wrapped
// error when the underlying recognizer fails to start (VAD disabled path).
func TestASRProcessorRecognizerStartError(t *testing.T) {
	r := newMockRecognizer()
	r.startErr = fmt.Errorf("connection refused")

	proc, _ := NewASRProcessor(newTestASRConfig(r))
	if err := proc.Start(context.Background()); err == nil {
		t.Fatal("expected error when recognizer.Start fails")
	}
}

// TestASRProcessorConcurrentWrite verifies concurrent Write calls do not panic
// or produce data races (run with -race).
func TestASRProcessorConcurrentWrite(t *testing.T) {
	r := newMockRecognizer()
	proc, _ := NewASRProcessor(newTestASRConfig(r))

	if err := proc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = proc.Write(make([]byte, 320))
		}()
	}
	wg.Wait()
}
