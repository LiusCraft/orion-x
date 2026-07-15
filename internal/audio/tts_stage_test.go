package audio

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/provider/tts"
	"github.com/liuscraft/orion-x/pkg/pipeline"
)

// mockTTSProcessor is a minimal TTSProcessor for testing TTSStage in
// isolation from the real TTS pipeline.
type mockTTSProcessor struct {
	mu             sync.Mutex
	onChunk        func(TTSChunk)
	writeCalls     []string
	writeErr       error
	flushCalls     int
	interruptCalls int
}

func (m *mockTTSProcessor) Write(text string, _ tts.SynthesisOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeCalls = append(m.writeCalls, text)
	return m.writeErr
}

func (m *mockTTSProcessor) Flush(_ tts.SynthesisOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flushCalls++
	return nil
}

func (m *mockTTSProcessor) OnChunk(fn func(TTSChunk)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChunk = fn
}

func (m *mockTTSProcessor) Interrupt() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.interruptCalls++
	return nil
}

func (m *mockTTSProcessor) Start(_ context.Context) error { return nil }
func (m *mockTTSProcessor) Stop() error                   { return nil }

// emitChunk simulates the TTSProcessor internally producing an audio chunk
// (as if from its dispatcher/playAudio goroutine).
func (m *mockTTSProcessor) emitChunk(c TTSChunk) {
	m.mu.Lock()
	fn := m.onChunk
	m.mu.Unlock()
	if fn != nil {
		fn(c)
	}
}

func (m *mockTTSProcessor) writeCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.writeCalls)
}

func (m *mockTTSProcessor) flushCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.flushCalls
}

func (m *mockTTSProcessor) interruptCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.interruptCalls
}

// TestTTSStage_TextInputWritesToProcessor 验证文本消息驱动 proc.Write。
func TestTTSStage_TextInputWritesToProcessor(t *testing.T) {
	proc := &mockTTSProcessor{}
	stage := NewTTSStage(proc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan pipeline.Message, 1)
	output := stage.Process(ctx, input)

	input <- pipeline.NewMessage(pipeline.MessageTypeData, "你好")

	// 没有下游消费 output，但 proc.Write 应该已经同步发生。
	deadline := time.After(time.Second)
	for proc.writeCallCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for proc.Write to be called")
		case <-time.After(5 * time.Millisecond):
		}
	}

	_ = output
}

// TestTTSStage_OnChunkProducesMessage 验证 proc 内部产生的音频块经由
// pipeline Message 总线输出，而不是旁路回调。
func TestTTSStage_OnChunkProducesMessage(t *testing.T) {
	proc := &mockTTSProcessor{}
	stage := NewTTSStage(proc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan pipeline.Message)
	output := stage.Process(ctx, input)

	want := TTSChunk{Text: "你好", Audio: []byte{1, 2, 3}}
	proc.emitChunk(want)

	select {
	case msg := <-output:
		chunk, ok := msg.Payload.(TTSChunk)
		if !ok {
			t.Fatalf("expected payload type TTSChunk, got %T", msg.Payload)
		}
		if chunk.Text != want.Text || string(chunk.Audio) != string(want.Audio) {
			t.Errorf("unexpected chunk: %+v", chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for chunk message")
	}
}

// TestTTSStage_FinishedTriggersFlush 验证 Finished 消息触发 proc.Flush。
func TestTTSStage_FinishedTriggersFlush(t *testing.T) {
	proc := &mockTTSProcessor{}
	stage := NewTTSStage(proc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan pipeline.Message, 1)
	_ = stage.Process(ctx, input)

	input <- pipeline.Message{Type: pipeline.MessageTypeFinished}

	deadline := time.After(time.Second)
	for proc.flushCallCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for proc.Flush to be called")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestTTSStage_InterruptForwarded 验证 Interrupt 消息触发 proc.Interrupt 并
// 转发给下游，供 output stage 感知打断以便立即停止播报。
func TestTTSStage_InterruptForwarded(t *testing.T) {
	proc := &mockTTSProcessor{}
	stage := NewTTSStage(proc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan pipeline.Message, 1)
	output := stage.Process(ctx, input)

	input <- pipeline.Message{Type: pipeline.MessageTypeInterrupt}

	select {
	case msg := <-output:
		if msg.Type != pipeline.MessageTypeInterrupt {
			t.Errorf("expected Interrupt message, got %s", msg.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for forwarded interrupt")
	}

	if proc.interruptCallCount() != 1 {
		t.Errorf("expected 1 proc.Interrupt call, got %d", proc.interruptCallCount())
	}
}

// TestTTSStage_WriteErrorEmitsErrorMessage 验证 Write 失败时输出 Error 消息，
// 而不是把原始文本 payload 透传下去（避免与 asr 的识别文本混淆）。
func TestTTSStage_WriteErrorEmitsErrorMessage(t *testing.T) {
	wantErr := errors.New("boom")
	proc := &mockTTSProcessor{writeErr: wantErr}
	stage := NewTTSStage(proc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan pipeline.Message, 1)
	output := stage.Process(ctx, input)

	input <- pipeline.NewMessage(pipeline.MessageTypeData, "文本")

	select {
	case msg := <-output:
		if !msg.IsError() {
			t.Fatalf("expected error message, got %+v", msg)
		}
		if !errors.Is(msg.Metadata.Error, wantErr) {
			t.Errorf("unexpected error: %v", msg.Metadata.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for error message")
	}
}

// TestTTSStage_CloseAfterCtxCancelDoesNotPanic 验证 ctx 取消后 output 被关闭，
// 且并发的 OnChunk 回调不会因写入已关闭 channel 而 panic（用 -race 运行）。
func TestTTSStage_CloseAfterCtxCancelDoesNotPanic(t *testing.T) {
	proc := &mockTTSProcessor{}
	stage := NewTTSStage(proc)

	ctx, cancel := context.WithCancel(context.Background())
	input := make(chan pipeline.Message)
	output := stage.Process(ctx, input)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				proc.emitChunk(TTSChunk{Audio: []byte{9}})
			}
		}
	}()

	cancel()

	// output 应该最终被关闭。
	select {
	case _, ok := <-output:
		if ok {
			// 可能还会收到几条在 cancel 前已入队的消息，继续排空直到关闭。
			for range output {
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for output to close")
	}

	close(stop)
	wg.Wait()
}
