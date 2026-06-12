package pipeline

import (
	"context"
	"fmt"
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
	defer p.Stop()

	p.Input() <- NewMessage(MessageTypeTextChunk, "hello")

	msg := <-p.Output()
	result, ok := msg.Payload.(string)
	if !ok {
		t.Fatalf("Expected string payload, got %T", msg.Payload)
	}
	if result != "hello-A-B-C" {
		t.Errorf("Expected 'hello-A-B-C', got '%s'", result)
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
	defer p.Stop()

	p.Input() <- NewMessage(MessageTypeTextChunk, "hello")
	p.Input() <- NewMessage(MessageTypeTextChunk, "world")

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
	defer p.Stop()

	// 通过 input 发送消息（两个 source 节点都从 input 读取）
	p.Input() <- NewMessage(MessageTypeTextChunk, "hello")

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
	defer p.Stop()

	p.Input() <- NewMessage(MessageTypeTextChunk, "hello")

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

func TestDAGPipelineReportScenario(t *testing.T) {
	// 用户的场景：Agent → TTS，TTS → Output + TTS → Report（异步上报）
	var reportCount int32
	var reportMu sync.Mutex
	var reports []string

	// Report 是 sink 节点：接收消息但不产出到 output
	reportStage := newMockStage("report", func(ctx context.Context, input <-chan Message) <-chan Message {
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
					// 异步上报：不阻塞下游
					atomic.AddInt32(&reportCount, 1)
					reportMu.Lock()
					reports = append(reports, msg.Payload.(string))
					reportMu.Unlock()
				}
			}
		}()
		return output
	})

	// TTS 输出：先发句子文本，再发音频标记
	ttsStage := newMockStage("tts", func(ctx context.Context, input <-chan Message) <-chan Message {
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
					text := msg.Payload.(string)
					// 1. 先发句子
					sentenceMsg := NewMessage(MessageTypeTTSStart, text)
					sentenceMsg.Metadata = msg.Metadata
					select {
					case output <- sentenceMsg:
					case <-ctx.Done():
						return
					}
					// 2. 再发音频（模拟）
					audioMsg := NewMessage(MessageTypeAudioData, text+"-audio")
					select {
					case output <- audioMsg:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
		return output
	})

	p, err := NewDAGBuilder().
		AddStage(newPassthroughStage("agent", nil)).
		AddStage(ttsStage).
		AddStage(newPassthroughStage("output", nil)). // 显式输出 sink
		AddStage(reportStage).
		Connect("agent", "tts").
		Connect("tts", "output"). // 主路径
		Connect("tts", "report"). // 上报分支（fan-out）
		SetFanoutBufferSize(8).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer p.Stop()

	// 发送 3 个文本
	for i := 0; i < 3; i++ {
		p.Input() <- NewMessage(MessageTypeTextChunk, fmt.Sprintf("sentence-%d", i))
	}

	// output 应收到 6 条消息（3 个句子 + 3 个音频，sink 只有 tts）
	results := collectAll(t, p.Output(), 6, 500*time.Millisecond)

	sentenceCount := 0
	audioCount := 0
	for _, m := range results {
		switch m.Type {
		case MessageTypeTTSStart:
			sentenceCount++
		case MessageTypeAudioData:
			audioCount++
		}
	}
	if sentenceCount != 3 {
		t.Errorf("Expected 3 sentences, got %d", sentenceCount)
	}
	if audioCount != 3 {
		t.Errorf("Expected 3 audio messages, got %d", audioCount)
	}

	// report 应收到 6 条消息（异步分支，需要等待）
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&reportCount) != 6 {
		t.Errorf("Expected 6 report messages, got %d", reportCount)
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

func TestDAGPipelineUnknownNode(t *testing.T) {
	_, err := NewDAGBuilder().
		AddStage(newPassthroughStage("a", nil)).
		Connect("a", "nonexistent").
		Build()

	if err == nil {
		t.Fatal("Expected unknown node error")
	}
}

func TestDAGPipelineEmptyDAG(t *testing.T) {
	_, err := NewDAGBuilder().Build()
	if err == nil {
		t.Fatal("Expected empty DAG error")
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
	p.Input() <- NewMessage(MessageTypeTextChunk, "slow")

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
	defer p.Stop()

	p.Input() <- NewMessage(MessageTypeTextChunk, "hello")
	p.Input() <- NewMessage(MessageTypeTextChunk, "world")

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
	defer p.Stop()

	// 快速发送 2 条消息
	p.Input() <- NewMessage(MessageTypeTextChunk, "msg-1")
	p.Input() <- NewMessage(MessageTypeTextChunk, "msg-2")

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

func TestDAGBuilderImmutable(t *testing.T) {
	// 验证 Build 后修改 builder 不影响已构建的 pipeline
	builder := NewDAGBuilder().
		AddStage(newPassthroughStage("a", func(s string) string { return s + "-A" })).
		AddStage(newPassthroughStage("b", func(s string) string { return s + "-B" })).
		Connect("a", "b")

	p1, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// 修改 builder
	builder.AddStage(newPassthroughStage("c", nil))
	builder.Connect("b", "c")

	p2, err := builder.Build()
	if err != nil {
		t.Fatalf("Second build failed: %v", err)
	}

	// p1 不应看到 "c" 节点
	ctx := context.Background()
	if err := p1.Start(ctx); err != nil {
		t.Fatalf("p1 Start failed: %v", err)
	}
	defer p1.Stop()

	p1.Input() <- NewMessage(MessageTypeTextChunk, "hello")
	msg := <-p1.Output()
	if msg.Payload.(string) != "hello-A-B" {
		t.Errorf("p1: Expected 'hello-A-B', got '%s'", msg.Payload)
	}

	// p2 应该包含 c
	if err := p2.Start(ctx); err != nil {
		t.Fatalf("p2 Start failed: %v", err)
	}
	defer p2.Stop()

	p2.Input() <- NewMessage(MessageTypeTextChunk, "hello")
	msg = <-p2.Output()
	if msg.Payload.(string) != "hello-A-B" {
		t.Errorf("p2: Expected 'hello-A-B', got '%s' (c is passthrough)", msg.Payload)
	}
}
