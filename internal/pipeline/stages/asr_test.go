package stages_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/pipeline"
	"github.com/liuscraft/orion-x/internal/pipeline/stages"
)

// mockASRProcessor is a minimal audio.ASRProcessor for testing ASRStage in
// isolation.
type mockASRProcessor struct {
	mu       sync.Mutex
	onResult func(audio.ASRResult)
	onSpeech func()
	startErr error
}

func (m *mockASRProcessor) Write(_ []byte) error { return nil }

func (m *mockASRProcessor) OnResult(fn func(audio.ASRResult)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onResult = fn
}

func (m *mockASRProcessor) OnSpeechStart(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onSpeech = fn
}

func (m *mockASRProcessor) Start(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startErr
}

func (m *mockASRProcessor) Stop() error { return nil }

func (m *mockASRProcessor) BeginTurn(_ context.Context) error { return nil }
func (m *mockASRProcessor) EndTurn(_ context.Context) error   { return nil }

func (m *mockASRProcessor) emitResult(r audio.ASRResult) {
	m.mu.Lock()
	fn := m.onResult
	m.mu.Unlock()
	if fn != nil {
		fn(r)
	}
}

func (m *mockASRProcessor) waitOnResultRegistered(t *testing.T) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		m.mu.Lock()
		fn := m.onResult
		m.mu.Unlock()
		if fn != nil {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for OnResult registration")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestASRStage_ForwardsInterruptFromInput 验证外部通过 pipeline.Input()
// 注入的 Interrupt 消息（例如 WS 连接收到 abort）会被转发到 output，
// 从而能沿 DAG 传播给下游 AgentStage/TTSStage。
func TestASRStage_ForwardsInterruptFromInput(t *testing.T) {
	proc := &mockASRProcessor{}
	stage := stages.NewASRStage(proc, nil)

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
}

// TestASRStage_ForwardsDataFromInput 验证 input 里的 Data 类型消息（例如
// WS "listen state:detect" 直接注入的文本）会被转发到 output，跳过 ASR
// 直接驱动下游 AgentStage。
func TestASRStage_ForwardsDataFromInput(t *testing.T) {
	proc := &mockASRProcessor{}
	stage := stages.NewASRStage(proc, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan pipeline.Message, 1)
	output := stage.Process(ctx, input)

	input <- pipeline.NewMessage(pipeline.MessageTypeData, "injected text")

	select {
	case msg := <-output:
		text, ok := msg.Payload.(string)
		if !ok || text != "injected text" {
			t.Errorf("unexpected forwarded message: %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for forwarded data message")
	}
}

// TestASRStage_IgnoresFinishedAndErrorFromInput 验证 Finished/Error 类型的
// input 消息不会被转发——这两种只应该由 pipeline 内部产生，不是外部注入的
// 合法控制信号。
func TestASRStage_IgnoresFinishedAndErrorFromInput(t *testing.T) {
	proc := &mockASRProcessor{}
	stage := stages.NewASRStage(proc, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan pipeline.Message, 2)
	output := stage.Process(ctx, input)

	input <- pipeline.Message{Type: pipeline.MessageTypeFinished}
	input <- pipeline.Message{Type: pipeline.MessageTypeError}

	select {
	case msg := <-output:
		t.Fatalf("expected no message forwarded, got %+v", msg)
	case <-time.After(200 * time.Millisecond):
		// 正确：Finished/Error 被丢弃
	}
}

// TestASRStage_CLIScenario_UnusedInputDoesNotBlockResults 验证 CLI 场景
// （从不调用 pipeline.Input()，input channel 永远无人写入）下，ASR 结果
// 消息仍然正常产出——新增的 forwardInterrupt 转发逻辑必须是完全惰性的。
func TestASRStage_CLIScenario_UnusedInputDoesNotBlockResults(t *testing.T) {
	proc := &mockASRProcessor{}
	stage := stages.NewASRStage(proc, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan pipeline.Message) // 从不写入，模拟线性 Builder 下的 CLI 场景
	output := stage.Process(ctx, input)

	proc.waitOnResultRegistered(t)
	proc.emitResult(audio.ASRResult{Text: "hello", IsFinal: true})

	select {
	case msg := <-output:
		text, ok := msg.Payload.(string)
		if !ok || text != "hello" {
			t.Errorf("unexpected message: %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ASR result message")
	}
}

// TestASRStage_StartErrorClosesOutputWithoutDeadlock 验证 proc.Start 失败时
// output 会被正确关闭，且不依赖外部 ctx 被 cancel——forwardInterrupt
// goroutine 必须能在这种提前退出路径下也被唤醒，否则 wg.Wait() 会死锁。
func TestASRStage_StartErrorClosesOutputWithoutDeadlock(t *testing.T) {
	proc := &mockASRProcessor{startErr: errors.New("boom")}
	stage := stages.NewASRStage(proc, nil)

	// 故意不 cancel：验证 output 的关闭不依赖外部 ctx.Done()。
	ctx := context.Background()
	input := make(chan pipeline.Message)
	output := stage.Process(ctx, input)

	var gotError bool
	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg, ok := <-output:
			if !ok {
				if !gotError {
					t.Fatal("output closed without an error message")
				}
				return
			}
			if msg.IsError() {
				gotError = true
			}
		case <-deadline:
			t.Fatal("timeout: output was not closed after Start error (possible deadlock)")
		}
	}
}
