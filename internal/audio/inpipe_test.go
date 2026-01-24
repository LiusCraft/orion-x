package audio

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/asr"
)

type mockRecognizer struct {
	startCalled  bool
	sendCalled   bool
	finishCalled bool
	closeCalled  bool
	onResult     func(asr.Result)
}

type blockingAudioSource struct {
	readCh    chan []byte
	closeCh   chan struct{}
	closeOnce sync.Once
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
	m.startCalled = true
	return nil
}

func (m *mockRecognizer) SendAudio(ctx context.Context, data []byte) error {
	m.sendCalled = true
	return nil
}

func (m *mockRecognizer) Finish(ctx context.Context) error {
	m.finishCalled = true
	return nil
}

func (m *mockRecognizer) Close() error {
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

func TestNewInPipeWithRecognizer(t *testing.T) {
	config := DefaultInPipeConfig()
	mock := &mockRecognizer{}

	pipe := NewInPipeWithRecognizer(config, mock)
	if pipe == nil {
		t.Fatal("NewInPipeWithRecognizer returned nil")
	}
}

func TestInPipeStateTransitions(t *testing.T) {
	config := DefaultInPipeConfig()
	mock := &mockRecognizer{}
	pipe := NewInPipeWithRecognizer(config, mock)

	ctx := context.Background()

	if impl, ok := pipe.(*inPipeImpl); ok {
		if impl.state != InPipeStateIdle {
			t.Errorf("Expected initial state Idle, got %s", impl.state)
		}
	}

	err := pipe.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if impl, ok := pipe.(*inPipeImpl); ok {
		if impl.state != InPipeStateListening {
			t.Errorf("Expected state Listening after Start, got %s", impl.state)
		}
	}

	err = pipe.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if impl, ok := pipe.(*inPipeImpl); ok {
		if impl.state != InPipeStateIdle {
			t.Errorf("Expected state Idle after Stop, got %s", impl.state)
		}
	}
}

func TestInPipeStartWhenAlreadyStarted(t *testing.T) {
	config := DefaultInPipeConfig()
	mock := &mockRecognizer{}
	pipe := NewInPipeWithRecognizer(config, mock)

	ctx := context.Background()

	err := pipe.Start(ctx)
	if err != nil {
		t.Fatalf("First Start failed: %v", err)
	}

	err = pipe.Start(ctx)
	if err == nil {
		t.Error("Expected error when starting already started pipe")
	}

	pipe.Stop()
}

func TestInPipeSendAudioWhenNotStarted(t *testing.T) {
	config := DefaultInPipeConfig()
	mock := &mockRecognizer{}
	pipe := NewInPipeWithRecognizer(config, mock)

	err := pipe.SendAudio([]byte{0x00, 0x01})
	if err == nil {
		t.Error("Expected error when sending audio before start")
	}
}

func TestInPipeSendAudio(t *testing.T) {
	config := DefaultInPipeConfig()
	mock := &mockRecognizer{}
	pipe := NewInPipeWithRecognizer(config, mock)

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

	pipe.Stop()
}

func TestInPipeOnASRResult(t *testing.T) {
	config := DefaultInPipeConfig()
	mock := &mockRecognizer{}
	pipe := NewInPipeWithRecognizer(config, mock)

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

	pipe.Stop()
}

func TestInPipeStopWhenIdle(t *testing.T) {
	config := DefaultInPipeConfig()
	mock := &mockRecognizer{}
	pipe := NewInPipeWithRecognizer(config, mock)

	err := pipe.Stop()
	if err == nil {
		t.Error("Expected error when stopping idle pipe")
	}
}

func TestInPipeContextCancellation(t *testing.T) {
	config := DefaultInPipeConfig()
	mock := &mockRecognizer{}
	pipe := NewInPipeWithRecognizer(config, mock)

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
	config := DefaultInPipeConfig()
	mock := &mockRecognizer{}
	pipe := NewInPipeWithRecognizer(config, mock)

	impl, ok := pipe.(*inPipeImpl)
	if !ok {
		t.Fatal("expected inPipeImpl")
	}

	source := newBlockingAudioSource()
	impl.SetAudioSource(source)

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

func TestInPipeDetectSpeech(t *testing.T) {
	impl := &inPipeImpl{vadThreshold: 0.2}

	silence := makePCM(0, 160)
	if impl.detectSpeech(silence) {
		t.Fatal("expected silence to not trigger VAD")
	}

	voice := makePCM(12000, 160)
	if !impl.detectSpeech(voice) {
		t.Fatal("expected voice to trigger VAD")
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
