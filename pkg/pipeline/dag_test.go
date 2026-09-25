package pipeline

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---- 测试辅助函数 ----

func newPassthroughStage(name string, modifier func(string) string) Stage {
	return newMockStage(name, func(ctx context.Context, input <-chan Message) <-chan Message {
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
					if modifier != nil {
						if s, ok := msg.Payload.(string); ok {
							msg.Payload = modifier(s)
						}
					}
					select {
					case output <- msg:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
		return output
	})
}

func newSinkStage(name string, fn func(Message)) Stage {
	return newMockStage(name, func(ctx context.Context, input <-chan Message) <-chan Message {
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
					if fn != nil {
						fn(msg)
					}
				}
			}
		}()
		return output
	})
}

func collectAll(t *testing.T, ch <-chan Message, expectedCount int, timeout time.Duration) []Message {
	t.Helper()
	var result []Message
	deadline := time.After(timeout)
	for i := 0; i < expectedCount; i++ {
		select {
		case msg, ok := <-ch:
			if !ok {
				return result
			}
			result = append(result, msg)
		case <-deadline:
			t.Fatalf("timeout waiting for message %d/%d", i+1, expectedCount)
		}
	}
	return result
}

// ---- 测试用例 ----

func TestDAGPipelineLinear(t *testing.T) {
	p, err := NewDAGBuilder().
		AddStage(newPassthroughStage("a", func(s string) string { return s + "-A" })).
		AddStage(newPassthroughStage("b", func(s string) string { return s + "-B" })).
		AddStage(newPassthroughStage("c", func(s string) string { return s + "-C" })).
		Connect("a", "b").
		Connect("b", "c").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = p.Stop() }()

	p.Input() <- NewMessage(MessageTypeData, "hello")

	msg := <-p.Output()
	result, ok := msg.Payload.(string)
	if !ok {
		t.Fatalf("Expected string payload, got %T", msg.Payload)
	}
	if result != "hello-A-B-C" {
		t.Errorf("Expected 'hello-A-B-C', got '%s'", result)
	}
}

func TestDAGPipelineEmitBypassesStages(t *testing.T) {
	p, err := NewDAGBuilder().
		AddStage(newPassthroughStage("source", func(s string) string { return s + "-processed" })).
		Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if err := p.Emit(NewMessage(MessageTypeData, "before-start")); err != ErrNotStarted {
		t.Fatalf("Emit() before Start = %v, want %v", err, ErrNotStarted)
	}
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = p.Stop() }()

	if err := p.Emit(NewMessage(MessageTypeData, "mounted-result")); err != nil {
		t.Fatalf("Emit() error = %v", err)
	}
	select {
	case msg := <-p.Output():
		if msg.Payload != "mounted-result" {
			t.Fatalf("emitted payload = %v", msg.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for emitted output")
	}
}

func TestDAGPipelineFanOut(t *testing.T) {
	var mu sync.Mutex
	var received []string

	// A 输出同时到 B 和 C（fan-out）
	p, err := NewDAGBuilder().
		AddStage(newPassthroughStage("a", func(s string) string { return s + "-A" })).
		AddStage(newSinkStage("b", func(m Message) {
			mu.Lock()
			received = append(received, "B:"+m.Payload.(string))
			mu.Unlock()
		})).
		AddStage(newSinkStage("c", func(m Message) {
			mu.Lock()
			received = append(received, "C:"+m.Payload.(string))
			mu.Unlock()
		})).
		Connect("a", "b").
		Connect("a", "c").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = p.Stop() }()

	p.Input() <- NewMessage(MessageTypeData, "hello")
	p.Input() <- NewMessage(MessageTypeData, "world")

	// 等待所有分支处理完毕
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 4 {
		t.Errorf("Expected 4 messages (2 per branch), got %d: %v", len(received), received)
	}

	bCount, cCount := 0, 0
	for _, r := range received {
		if len(r) > 2 && r[:2] == "B:" {
			bCount++
		}
		if len(r) > 2 && r[:2] == "C:" {
			cCount++
		}
	}
	if bCount != 2 {
		t.Errorf("Expected 2 B messages, got %d", bCount)
	}
	if cCount != 2 {
		t.Errorf("Expected 2 C messages, got %d", cCount)
	}
}

func TestDAGPipelineFanIn(t *testing.T) {
	// A 和 B 的输出合并到 C（fan-in）
	p, err := NewDAGBuilder().
		AddStage(newPassthroughStage("a", func(s string) string { return s + "-A" })).
		AddStage(newPassthroughStage("b", func(s string) string { return s + "-B" })).
		AddStage(newPassthroughStage("c", func(s string) string { return s + "-C" })).
		Connect("a", "c").
		Connect("b", "c").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = p.Stop() }()

	// 通过 input 发送消息（两个 source 节点都从 input 读取）
	p.Input() <- NewMessage(MessageTypeData, "hello")

	msg := <-p.Output()

	// 两个 source 节点都收到相同消息，输出可能重复
	result, _ := msg.Payload.(string)
	if result != "hello-A-C" && result != "hello-B-C" {
		t.Errorf("Expected 'hello-A-C' or 'hello-B-C', got '%s'", result)
	}
}

func TestDAGPipelineDiamond(t *testing.T) {
	// A → B → D
	// A → C → D    （钻石拓扑）
	p, err := NewDAGBuilder().
		AddStage(newPassthroughStage("a", func(s string) string { return s + "-A" })).
		AddStage(newPassthroughStage("b", func(s string) string { return s + "-B" })).
		AddStage(newPassthroughStage("c", func(s string) string { return s + "-C" })).
		AddStage(newPassthroughStage("d", func(s string) string { return s + "-D" })).
		Connect("a", "b").
		Connect("a", "c").
		Connect("b", "d").
		Connect("c", "d").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = p.Stop() }()

	p.Input() <- NewMessage(MessageTypeData, "hello")

	// D 会收到两条消息（来自 B 和 C）
	results := collectAll(t, p.Output(), 2, 500*time.Millisecond)

	received := make(map[string]bool)
	for _, m := range results {
		received[m.Payload.(string)] = true
	}

	if !received["hello-A-B-D"] {
		t.Errorf("Expected 'hello-A-B-D', got %v", results)
	}
	if !received["hello-A-C-D"] {
		t.Errorf("Expected 'hello-A-C-D', got %v", results)
	}
}

func TestDAGPipelineCycleDetection(t *testing.T) {
	_, err := NewDAGBuilder().
		AddStage(newPassthroughStage("a", nil)).
		AddStage(newPassthroughStage("b", nil)).
		Connect("a", "b").
		Connect("b", "a"). // 环！
		Build()

	if err == nil {
		t.Fatal("Expected cycle detection error")
	}
	if err.Error() != "cycle detected in pipeline DAG" {
		t.Errorf("Expected cycle error, got: %v", err)
	}
}

func TestDAGPipelineInterrupt(t *testing.T) {
	// 创建一个会长时间阻塞的节点
	blockingStage := newMockStage("blocking", func(ctx context.Context, input <-chan Message) <-chan Message {
		output := make(chan Message)
		go func() {
			defer close(output)
			for {
				select {
				case <-ctx.Done():
					return
				case _, ok := <-input:
					if !ok {
						return
					}
					time.Sleep(500 * time.Millisecond) // 慢处理
				}
			}
		}()
		return output
	})

	p, err := NewDAGBuilder().
		AddStage(blockingStage).
		Connect("blocking", "report"). // 需要至少一条边让它不是 source+sink
		AddStage(newSinkStage("report", nil)).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 发送消息触发慢处理
	p.Input() <- NewMessage(MessageTypeData, "slow")

	// 立即打断
	time.Sleep(50 * time.Millisecond)
	if err := p.Interrupt(); err != nil {
		t.Fatalf("Interrupt failed: %v", err)
	}

	// 等待清理
	time.Sleep(100 * time.Millisecond)

	// 尝试 Stop
	if err := p.Stop(); err != nil {
		t.Errorf("Stop after interrupt failed: %v", err)
	}
}

func TestDAGPipelineObserver(t *testing.T) {
	observed := make(map[string]int)
	var muObs sync.Mutex
	observer := &testObserver{
		onMessage: func(stageName string, msg Message) {
			muObs.Lock()
			observed[stageName]++
			muObs.Unlock()
		},
	}

	p, err := NewDAGBuilder().
		AddStage(newPassthroughStage("a", nil)).
		AddStage(newPassthroughStage("b", nil)).
		Connect("a", "b").
		SetObserver(observer).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = p.Stop() }()

	p.Input() <- NewMessage(MessageTypeData, "hello")
	p.Input() <- NewMessage(MessageTypeData, "world")

	// 收集输出
	_ = collectAll(t, p.Output(), 2, 200*time.Millisecond)

	muObs.Lock()
	aObs, bObs := observed["a"], observed["b"]
	muObs.Unlock()

	if aObs != 2 {
		t.Errorf("Expected 2 observer calls for 'a', got %d", aObs)
	}
	if bObs != 2 {
		t.Errorf("Expected 2 observer calls for 'b', got %d", bObs)
	}
}

func TestDAGPipelineAsyncReportNonBlocking(t *testing.T) {
	// 验证 report 分支不阻塞主链路
	var reportProcessed sync.WaitGroup
	reportProcessed.Add(2)
	var mainReceived int32

	// 慢 report（模拟网络上报延迟）
	slowReport := newMockStage("slow-report", func(ctx context.Context, input <-chan Message) <-chan Message {
		output := make(chan Message)
		go func() {
			defer close(output)
			for {
				select {
				case <-ctx.Done():
					return
				case _, ok := <-input:
					if !ok {
						return
					}
					// 模拟 100ms 上报延迟
					time.Sleep(100 * time.Millisecond)
					reportProcessed.Done()
				}
			}
		}()
		return output
	})

	// 主链路快速消费
	fastOutput := newMockStage("output", func(ctx context.Context, input <-chan Message) <-chan Message {
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
					atomic.AddInt32(&mainReceived, 1)
					select {
					case output <- msg:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
		return output
	})

	p, err := NewDAGBuilder().
		AddStage(newPassthroughStage("source", nil)).
		AddStage(fastOutput).
		AddStage(slowReport).
		Connect("source", "output").
		Connect("source", "slow-report").
		SetFanoutBufferSize(8).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = p.Stop() }()

	// 快速发送 2 条消息
	p.Input() <- NewMessage(MessageTypeData, "msg-1")
	p.Input() <- NewMessage(MessageTypeData, "msg-2")

	// 主链路应快速收到（不等待 report）
	_ = collectAll(t, p.Output(), 2, 200*time.Millisecond)

	if atomic.LoadInt32(&mainReceived) != 2 {
		t.Errorf("Main path should receive 2 messages quickly, got %d", mainReceived)
	}

	// report 最终也会完成（等待异步上报）
	reportDone := make(chan struct{})
	go func() {
		reportProcessed.Wait()
		close(reportDone)
	}()
	select {
	case <-reportDone:
		// ok
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Report did not finish in time")
	}
}
