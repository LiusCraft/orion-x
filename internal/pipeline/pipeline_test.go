package pipeline

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mockStage 用于测试的 Mock Stage
type mockStage struct {
	*BaseStage
	processFunc func(ctx context.Context, input <-chan Message) <-chan Message
}

func newMockStage(name string, processFunc func(context.Context, <-chan Message) <-chan Message) *mockStage {
	return &mockStage{
		BaseStage:   NewBaseStage(name),
		processFunc: processFunc,
	}
}

func (s *mockStage) Process(ctx context.Context, input <-chan Message) <-chan Message {
	if s.processFunc != nil {
		return s.processFunc(ctx, input)
	}
	return input // 默认透传
}

func TestPipelineBasic(t *testing.T) {
	// 创建简单的 Pipeline: Stage1 -> Stage2 -> Stage3
	stage1 := newMockStage("stage1", func(ctx context.Context, input <-chan Message) <-chan Message {
		output := make(chan Message)
		go func() {
			defer close(output)
			for msg := range input {
				msg.Payload = fmt.Sprintf("%s-stage1", msg.Payload)
				select {
				case output <- msg:
				case <-ctx.Done():
					return
				}
			}
		}()
		return output
	})

	stage2 := newMockStage("stage2", func(ctx context.Context, input <-chan Message) <-chan Message {
		output := make(chan Message)
		go func() {
			defer close(output)
			for msg := range input {
				msg.Payload = fmt.Sprintf("%s-stage2", msg.Payload)
				select {
				case output <- msg:
				case <-ctx.Done():
					return
				}
			}
		}()
		return output
	})

	pipeline := NewBuilder().
		AddStage(stage1).
		AddStage(stage2).
		Build()

	ctx := context.Background()
	if err := pipeline.Start(ctx); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer func() { _ = pipeline.Stop() }()

	// 发送消息并等待 goroutine 完成
	done := make(chan struct{})
	go func() {
		defer close(done)
		pipeline.Input() <- NewMessage(MessageTypeTextChunk, "test")
	}()
	<-done

	// 接收输出
	msg := <-pipeline.Output()
	if msg.Payload != "input-stage1-stage2" {
		t.Errorf("Expected 'input-stage1-stage2', got '%s'", msg.Payload)
	}
}

func TestPipelineInterrupt(t *testing.T) {
	// 创建会阻塞的 Stage
	blockingStage := newMockStage("blocking", func(ctx context.Context, input <-chan Message) <-chan Message {
		output := make(chan Message)
		go func() {
			defer close(output)
			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-input:
					if !ok {
						return
					}
					// 模拟耗时操作
					time.Sleep(100 * time.Millisecond)
					output <- msg
				}
			}
		}()
		return output
	})

	pipeline := NewBuilder().
		AddStage(blockingStage).
		Build()

	ctx := context.Background()
	if err := pipeline.Start(ctx); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer func() { _ = pipeline.Stop() }()

	// 发送消息并等待 goroutine 完成
	msgSent := make(chan struct{})
	go func() {
		pipeline.Input() <- NewMessage(MessageTypeTextChunk, "test")
		close(msgSent)
	}()

	// 确保消息已发送
	select {
	case <-msgSent:
	case <-time.After(1 * time.Second):
		t.Fatal("message not sent before timeout")
	}

	// 立即打断
	time.Sleep(10 * time.Millisecond)
	if err := pipeline.Interrupt(); err != nil {
		t.Fatalf("Failed to interrupt: %v", err)
	}

	// 验证输出 channel 最终会关闭
	timeout := time.After(500 * time.Millisecond)
	select {
	case <-pipeline.Output():
		// 可能收到消息或 channel 关闭，都是正常的
	case <-timeout:
		t.Fatal("Pipeline did not stop after interrupt")
	}
}

func TestPipelineErrorPropagation(t *testing.T) {
	// 创建会产生错误的 Stage
	errorStage := newMockStage("error", func(ctx context.Context, input <-chan Message) <-chan Message {
		output := make(chan Message)
		go func() {
			defer close(output)
			for msg := range input {
				// 模拟错误
				msg = msg.WithError(fmt.Errorf("stage error"))
				select {
				case output <- msg:
				case <-ctx.Done():
					return
				}
			}
		}()
		return output
	})

	pipeline := NewBuilder().
		AddStage(errorStage).
		Build()

	ctx := context.Background()
	if err := pipeline.Start(ctx); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer func() { _ = pipeline.Stop() }()

	// 发送消息
	go func() {
		pipeline.Input() <- NewMessage(MessageTypeTextChunk, "test")
	}()

	// 接收错误消息
	msg := <-pipeline.Output()
	if !msg.IsError() {
		t.Error("Expected error message")
	}
	if msg.Metadata.Error.Error() != "stage error" {
		t.Errorf("Expected 'stage error', got '%v'", msg.Metadata.Error)
	}
}

func TestPipelineWithObserver(t *testing.T) {
	observed := make(map[string]int)
	observer := &testObserver{
		onMessage: func(stageName string, msg Message) {
			observed[stageName]++
		},
	}

	stage := newMockStage("test", nil) // 透传

	pipeline := NewBuilder().
		AddStage(stage).
		SetObserver(observer).
		Build()

	ctx := context.Background()
	if err := pipeline.Start(ctx); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer func() { _ = pipeline.Stop() }()

	// 发送多条消息
	go func() {
		for i := 0; i < 3; i++ {
			pipeline.Input() <- NewMessage(MessageTypeTextChunk, fmt.Sprintf("msg-%d", i))
		}
	}()

	// 接收输出
	for i := 0; i < 3; i++ {
		<-pipeline.Output()
	}

	if observed["test"] != 3 {
		t.Errorf("Expected 3 messages observed, got %d", observed["test"])
	}
}

// testObserver 测试用观察者
type testObserver struct {
	onMessage    func(string, Message)
	onError      func(string, error)
	onStageStart func(string)
	onStageStop  func(string)
}

func (o *testObserver) OnMessage(stageName string, msg Message) {
	if o.onMessage != nil {
		o.onMessage(stageName, msg)
	}
}

func (o *testObserver) OnError(stageName string, err error) {
	if o.onError != nil {
		o.onError(stageName, err)
	}
}

func (o *testObserver) OnStageStart(stageName string) {
	if o.onStageStart != nil {
		o.onStageStart(stageName)
	}
}

func (o *testObserver) OnStageStop(stageName string) {
	if o.onStageStop != nil {
		o.onStageStop(stageName)
	}
}
