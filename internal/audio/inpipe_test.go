package audio

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/audio/vad"
	"github.com/liuscraft/orion-x/internal/provider/asr"
)

type mockRecognizer struct {
	startCalled  bool
	sendCalled   bool
	finishCalled bool
	closeCalled  bool
	onResult     func(asr.Result)
	startCount   int
	sendCount    int
	finishCount  int
	sentAudio    [][]byte
	mu           sync.Mutex
}

type blockingRecognizer struct {
	startCalled bool
	sendStarted chan struct{}
	sendOnce    sync.Once
}

type blockingAudioSource struct {
	readCh    chan []byte
	closeCh   chan struct{}
	closeOnce sync.Once
}

type staticVAD struct {
	detected bool
	err      error
}

func (v *staticVAD) Detect([]byte) (bool, error) {
	return v.detected, v.err
}

func (v *staticVAD) Close() error {
	return nil
}

func testInPipeConfig() *InPipeConfig {
	config := DefaultInPipeConfig()
	config.EnableVAD = false
	return config
}

func newTestInPipe(config *InPipeConfig, recognizer asr.Recognizer) *InPipe {
	return &InPipe{
		state:      InPipeStateIdle,
		config:     config,
		recognizer: recognizer,
		vadEnabled: config.EnableVAD,
	}
}

func newBlockingAudioSource() *blockingAudioSource {
	return &blockingAudioSource{
		readCh:  make(chan []byte),
		closeCh: make(chan struct{}),
	}
}

func (s *blockingAudioSource) Read(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.closeCh:
		return nil, io.EOF
	case data := <-s.readCh:
		return data, nil
	}
}

func (s *blockingAudioSource) Close() error {
	s.closeOnce.Do(func() {
		close(s.closeCh)
	})
	return nil
}

func (m *mockRecognizer) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalled = true
	m.startCount++
	return nil
}

func (m *mockRecognizer) SendAudio(ctx context.Context, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendCalled = true
	m.sendCount++
	m.sentAudio = append(m.sentAudio, cloneBytes(data))
	return nil
}

func (b *blockingRecognizer) Start(ctx context.Context) error {
	b.startCalled = true
	return nil
}

func (b *blockingRecognizer) SendAudio(ctx context.Context, data []byte) error {
	b.sendOnce.Do(func() {
		close(b.sendStarted)
	})
	<-ctx.Done()
	return ctx.Err()
}

func (b *blockingRecognizer) Finish(ctx context.Context) error {
	return nil
}

func (b *blockingRecognizer) Close() error {
	return nil
}

func (b *blockingRecognizer) OnResult(handler func(asr.Result)) {}

func (m *mockRecognizer) Finish(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finishCalled = true
	m.finishCount++
	return nil
}

func (m *mockRecognizer) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	return nil
}

func (m *mockRecognizer) OnResult(handler func(asr.Result)) {
	m.onResult = handler
}

func (m *mockRecognizer) SendResult(result asr.Result) {
	if m.onResult != nil {
		m.onResult(result)
	}
}

func (m *mockRecognizer) counts() (start, send, finish int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startCount, m.sendCount, m.finishCount
}

func (m *mockRecognizer) sentFrameCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sentAudio)
}

func (m *mockRecognizer) sentBytes() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := 0
	for _, frame := range m.sentAudio {
		total += len(frame)
	}
	return total
}

func TestInPipeStateTransitions(t *testing.T) {
	config := testInPipeConfig()
	mock := &mockRecognizer{}
	pipe := newTestInPipe(config, mock)

	ctx := context.Background()

	if pipe.state != InPipeStateIdle {
		t.Errorf("Expected initial state Idle, got %s", pipe.state)
	}

	err := pipe.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if pipe.state != InPipeStateListening {
		t.Errorf("Expected state Listening after Start, got %s", pipe.state)
	}

	err = pipe.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if pipe.state != InPipeStateIdle {
		t.Errorf("Expected state Idle after Stop, got %s", pipe.state)
	}
}

func TestInPipeStartWhenAlreadyStarted(t *testing.T) {
	config := testInPipeConfig()
	mock := &mockRecognizer{}
	pipe := newTestInPipe(config, mock)

	ctx := context.Background()

	err := pipe.Start(ctx)
	if err != nil {
		t.Fatalf("First Start failed: %v", err)
	}

	err = pipe.Start(ctx)
	if err == nil {
		t.Error("Expected error when starting already started pipe")
	}

	_ = pipe.Stop()
}

func TestInPipeSendAudioWhenNotStarted(t *testing.T) {
	config := testInPipeConfig()
	mock := &mockRecognizer{}
	pipe := newTestInPipe(config, mock)

	err := pipe.SendAudio([]byte{0x00, 0x01})
	if err == nil {
		t.Error("Expected error when sending audio before start")
	}
}

func TestInPipeSendAudio(t *testing.T) {
	config := testInPipeConfig()
	mock := &mockRecognizer{}
	pipe := newTestInPipe(config, mock)

	ctx := context.Background()

	err := pipe.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err = pipe.SendAudio([]byte{0x00, 0x01})
	if err != nil {
		t.Errorf("SendAudio failed: %v", err)
	}

	if !mock.sendCalled {
		t.Error("Recognizer SendAudio was not called")
	}

	_ = pipe.Stop()
}

func TestInPipeOnASRResult(t *testing.T) {
	config := testInPipeConfig()
	mock := &mockRecognizer{}
	pipe := newTestInPipe(config, mock)

	ctx := context.Background()

	var receivedText string
	var receivedIsFinal bool

	pipe.OnASRResult(func(text string, isFinal bool) {
		receivedText = text
		receivedIsFinal = isFinal
	})

	err := pipe.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	mock.SendResult(asr.Result{Text: "hello", IsFinal: false})
	if receivedText != "hello" || receivedIsFinal {
		t.Errorf("Expected partial result, got text=%s, isFinal=%v", receivedText, receivedIsFinal)
	}

	mock.SendResult(asr.Result{Text: "hello world", IsFinal: true})
	if receivedText != "hello world" || !receivedIsFinal {
		t.Errorf("Expected final result, got text=%s, isFinal=%v", receivedText, receivedIsFinal)
	}

	_ = pipe.Stop()
}

func TestInPipeStopWhenIdle(t *testing.T) {
	config := testInPipeConfig()
	mock := &mockRecognizer{}
	pipe := newTestInPipe(config, mock)

	err := pipe.Stop()
	if err == nil {
		t.Error("Expected error when stopping idle pipe")
	}
}

func TestInPipeContextCancellation(t *testing.T) {
	config := testInPipeConfig()
	mock := &mockRecognizer{}
	pipe := newTestInPipe(config, mock)

	ctx, cancel := context.WithCancel(context.Background())

	err := pipe.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	cancel()
	time.Sleep(50 * time.Millisecond)

	pipe.Stop()
}

func TestDefaultInPipeConfig(t *testing.T) {
	config := DefaultInPipeConfig()

	if config.SampleRate != 16000 {
		t.Errorf("Expected SampleRate 16000, got %d", config.SampleRate)
	}

	if config.Channels != 1 {
		t.Errorf("Expected Channels 1, got %d", config.Channels)
	}

	if !config.EnableVAD {
		t.Error("Expected EnableVAD true")
	}

	if config.VADThreshold != 0.5 {
		t.Errorf("Expected VADThreshold 0.5, got %f", config.VADThreshold)
	}

	if config.VADType != string(vad.TypeSilero) {
		t.Errorf("Expected VADType silero, got %s", config.VADType)
	}

	if config.ASRModel != "fun-asr-realtime" {
		t.Errorf("Expected ASRModel fun-asr-realtime, got %s", config.ASRModel)
	}
}

func TestInPipeStateString(t *testing.T) {
	tests := []struct {
		state    InPipeState
		expected string
	}{
		{InPipeStateIdle, "Idle"},
		{InPipeStateListening, "Listening"},
		{InPipeStateStopping, "Stopping"},
	}

	for _, tt := range tests {
		result := tt.state.String()
		if result != tt.expected {
			t.Errorf("State %d: expected %s, got %s", tt.state, tt.expected, result)
		}
	}
}

func TestInPipeStopDoesNotDeadlock(t *testing.T) {
	config := testInPipeConfig()
	mock := &mockRecognizer{}
	pipe := newTestInPipe(config, mock)

	source := newBlockingAudioSource()
	pipe.audioSource = source

	if err := pipe.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = pipe.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Stop should not block when audio source is closed")
	}
}

func TestInPipeStopUnblocksSendAudio(t *testing.T) {
	config := testInPipeConfig()
	recognizer := &blockingRecognizer{sendStarted: make(chan struct{})}
	pipe := newTestInPipe(config, recognizer)

	source := newBlockingAudioSource()
	pipe.audioSource = source

	if err := pipe.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	go func() {
		source.readCh <- makePCM(12000, 160)
	}()

	select {
	case <-recognizer.sendStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SendAudio should start before Stop")
	}

	done := make(chan struct{})
	go func() {
		_ = pipe.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Stop should not block when SendAudio waits on context cancel")
	}
}

func TestInPipeVADSegmentsAudioBeforeASR(t *testing.T) {
	config := DefaultInPipeConfig()
	config.EnableVAD = true
	config.VADSpeechPadMs = 0

	mock := &mockRecognizer{}
	pipe := newTestInPipe(config, mock)
	pipe.segmenter = vad.NewSegmenter(&staticVAD{detected: true}, config.SampleRate, config.VADSpeechPadMs)
	pipe.vadEnabled = true

	if err := pipe.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := pipe.SendAudio(makePCM(12000, 160)); err != nil {
		t.Fatalf("SendAudio speech failed: %v", err)
	}

	start, send, finish := mock.counts()
	if start != 0 || send != 0 || finish != 0 {
		t.Fatalf("expected active speech to be buffered before ASR, got start=%d send=%d finish=%d", start, send, finish)
	}

	_ = pipe.segmenter.Close()
	pipe.segmenter = vad.NewSegmenter(&staticVAD{detected: false}, config.SampleRate, config.VADSpeechPadMs)
	if err := pipe.SendAudio(makePCM(0, 160)); err != nil {
		t.Fatalf("SendAudio silence failed: %v", err)
	}

	waitForCondition(t, func() bool {
		start, send, finish = mock.counts()
		return start == 1 && send == 1 && finish == 1
	})

	if frames := mock.sentFrameCount(); frames != 1 {
		t.Fatalf("expected one speech frame sent to ASR, got %d", frames)
	}

	if err := pipe.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestInPipeVADIgnoresSilenceOnlyAudio(t *testing.T) {
	config := DefaultInPipeConfig()
	config.EnableVAD = true

	mock := &mockRecognizer{}
	pipe := newTestInPipe(config, mock)
	pipe.segmenter = vad.NewSegmenter(&staticVAD{detected: false}, config.SampleRate, config.VADSpeechPadMs)
	pipe.vadEnabled = true

	if err := pipe.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := pipe.SendAudio(makePCM(0, 160)); err != nil {
		t.Fatalf("SendAudio failed: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	start, send, finish := mock.counts()
	if start != 0 || send != 0 || finish != 0 {
		t.Fatalf("expected silence to not reach ASR, got start=%d send=%d finish=%d", start, send, finish)
	}

	if err := pipe.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestInPipeVADFlushesPendingSegmentOnStop(t *testing.T) {
	config := DefaultInPipeConfig()
	config.EnableVAD = true
	config.VADSpeechPadMs = 0

	mock := &mockRecognizer{}
	pipe := newTestInPipe(config, mock)
	pipe.segmenter = vad.NewSegmenter(&staticVAD{detected: true}, config.SampleRate, config.VADSpeechPadMs)
	pipe.vadEnabled = true

	if err := pipe.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := pipe.SendAudio(makePCM(12000, 160)); err != nil {
		t.Fatalf("SendAudio failed: %v", err)
	}

	if err := pipe.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	start, send, finish := mock.counts()
	if start != 1 || send != 1 || finish != 1 {
		t.Fatalf("expected Stop to flush pending segment, got start=%d send=%d finish=%d", start, send, finish)
	}
}

func TestInPipeVADIncludesSpeechPadAndFirstSpeechFrame(t *testing.T) {
	config := DefaultInPipeConfig()
	config.EnableVAD = true
	config.VADSpeechPadMs = 100

	mock := &mockRecognizer{}
	pipe := newTestInPipe(config, mock)
	pipe.segmenter = vad.NewSegmenter(&staticVAD{detected: false}, config.SampleRate, config.VADSpeechPadMs)
	pipe.vadEnabled = true

	if err := pipe.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	silence := makePCM(0, 160)
	if err := pipe.SendAudio(silence); err != nil {
		t.Fatalf("SendAudio silence failed: %v", err)
	}

	_ = pipe.segmenter.Close()
	pipe.segmenter = vad.NewSegmenter(&staticVAD{detected: true}, config.SampleRate, config.VADSpeechPadMs)
	speech := makePCM(12000, 160)
	if err := pipe.SendAudio(speech); err != nil {
		t.Fatalf("SendAudio speech failed: %v", err)
	}

	_ = pipe.segmenter.Close()
	pipe.segmenter = vad.NewSegmenter(&staticVAD{detected: false}, config.SampleRate, config.VADSpeechPadMs)
	if err := pipe.SendAudio(makePCM(0, 160)); err != nil {
		t.Fatalf("SendAudio ending silence failed: %v", err)
	}

	wantBytes := len(silence) + len(speech)
	waitForCondition(t, func() bool {
		start, _, finish := mock.counts()
		return start == 1 && finish == 1 && mock.sentBytes() == wantBytes
	})

	if err := pipe.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func makePCM(sample int16, count int) []byte {
	buf := make([]byte, count*2)
	for i := 0; i < count; i++ {
		buf[i*2] = byte(sample)
		buf[i*2+1] = byte(sample >> 8)
	}
	return buf
}

func waitForCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.After(500 * time.Millisecond)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-deadline:
			t.Fatal("condition was not met before timeout")
		case <-ticker.C:
		}
	}
}

func cloneBytes(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	copied := make([]byte, len(data))
	copy(copied, data)
	return copied
}
