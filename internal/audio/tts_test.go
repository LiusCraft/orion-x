package audio

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/provider/tts"
)

// --- mock ---

type mockTTSProvider struct {
	mu        sync.Mutex
	callCount int
	callErr   error
	// per-call 延迟，用于测试播放顺序
	delays map[string]time.Duration
}

func newMockTTSProvider() *mockTTSProvider {
	return &mockTTSProvider{delays: make(map[string]time.Duration)}
}

func (p *mockTTSProvider) Synthesize(ctx context.Context, text string, _ tts.SynthesisOptions) (io.ReadCloser, error) {
	p.mu.Lock()
	p.callCount++
	err := p.callErr
	delay := p.delays[text]
	p.mu.Unlock()

	if err != nil {
		return nil, err
	}
	if delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	// 返回文本的字节作为 mock 音频
	return io.NopCloser(strings.NewReader(text)), nil
}

func (p *mockTTSProvider) getCallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.callCount
}

// --- helpers ---

func newTestTTSProcessor(provider tts.Provider) TTSProcessor {
	cfg := DefaultTTSConfig()
	cfg.Provider = provider
	proc, _ := NewTTSProcessor(cfg)
	return proc
}

func newTestTTSProcessorWithConfig(provider tts.Provider, maxConcurrent, queueSize, maxRunes int) TTSProcessor {
	cfg := DefaultTTSConfig()
	cfg.Provider = provider
	cfg.MaxConcurrent = maxConcurrent
	cfg.QueueSize = queueSize
	cfg.MaxRunes = maxRunes
	proc, _ := NewTTSProcessor(cfg)
	return proc
}

var defaultOpts = tts.SynthesisOptions{Emotion: "default"}

// --- tests ---

func TestTTSProcessorCreate(t *testing.T) {
	proc := newTestTTSProcessor(newMockTTSProvider())
	if proc == nil {
		t.Fatal("expected non-nil processor")
	}

	_, err := NewTTSProcessor(&TTSConfig{Provider: nil})
	if err == nil {
		t.Fatal("expected error when Provider is nil")
	}
}

func TestTTSProcessorStartStop(t *testing.T) {
	proc := newTestTTSProcessor(newMockTTSProvider())
	if err := proc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if err := proc.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestTTSProcessorDoubleStart(t *testing.T) {
	proc := newTestTTSProcessor(newMockTTSProvider())
	ctx := context.Background()
	if err := proc.Start(ctx); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	if err := proc.Start(ctx); err == nil {
		t.Fatal("expected error on double Start")
	}
}

func TestTTSProcessorWriteBeforeStart(t *testing.T) {
	proc := newTestTTSProcessor(newMockTTSProvider())
	if err := proc.Write("hello", defaultOpts); err == nil {
		t.Fatal("expected error when writing before Start")
	}
}

// TestTTSProcessorOnChunk_StrongBoundary 验证强停顿切句并触发 OnChunk。
func TestTTSProcessorOnChunk_StrongBoundary(t *testing.T) {
	proc := newTestTTSProcessor(newMockTTSProvider())

	chunkCh := make(chan TTSChunk, 5)
	proc.OnChunk(func(c TTSChunk) { chunkCh <- c })

	if err := proc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	_ = proc.Write("今天天气很好。", defaultOpts)

	select {
	case got := <-chunkCh:
		if got.Text != "今天天气很好。" {
			t.Errorf("unexpected text: %q", got.Text)
		}
		if len(got.Audio) == 0 {
			t.Error("expected non-empty audio")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for OnChunk")
	}
}

// TestTTSProcessorFirstSentenceWeakBoundary 验证首句在弱停顿处切句。
func TestTTSProcessorFirstSentenceWeakBoundary(t *testing.T) {
	proc := newTestTTSProcessor(newMockTTSProvider())

	chunkCh := make(chan TTSChunk, 5)
	proc.OnChunk(func(c TTSChunk) { chunkCh <- c })

	if err := proc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	// 逗号是弱停顿，首句应在此触发
	_ = proc.Write("今天天气很好，", defaultOpts)

	select {
	case got := <-chunkCh:
		if got.Text != "今天天气很好，" {
			t.Errorf("unexpected text: %q", got.Text)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout: first sentence should trigger at weak boundary")
	}
}

// TestTTSProcessorSubsequentSentenceStrongOnly 验证首句后只有强停顿才切句。
func TestTTSProcessorSubsequentSentenceStrongOnly(t *testing.T) {
	proc := newTestTTSProcessor(newMockTTSProvider())

	var chunks []TTSChunk
	var mu sync.Mutex
	proc.OnChunk(func(c TTSChunk) {
		mu.Lock()
		chunks = append(chunks, c)
		mu.Unlock()
	})

	if err := proc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	// 首句在逗号触发，第二句必须等句号
	_ = proc.Write("首句，", defaultOpts)
	time.Sleep(200 * time.Millisecond) // 等首句合成

	_ = proc.Write("第二句，逗号不切，", defaultOpts) // 不应切
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	countAfterComma := len(chunks)
	mu.Unlock()

	if countAfterComma != 1 {
		t.Errorf("expected 1 chunk after comma (only first sentence), got %d", countAfterComma)
	}

	_ = proc.Write("句号切。", defaultOpts) // 强停顿，应切
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	total := len(chunks)
	mu.Unlock()

	if total != 2 {
		t.Errorf("expected 2 chunks total, got %d", total)
	}
}

// TestTTSProcessorFlush 验证 Flush 输出剩余文本。
func TestTTSProcessorFlush(t *testing.T) {
	proc := newTestTTSProcessor(newMockTTSProvider())

	chunkCh := make(chan TTSChunk, 5)
	proc.OnChunk(func(c TTSChunk) { chunkCh <- c })

	if err := proc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	_ = proc.Write("没有停顿的文本", defaultOpts)

	// 还没有切句，Flush 强制输出
	if err := proc.Flush(defaultOpts); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	select {
	case got := <-chunkCh:
		if got.Text != "没有停顿的文本" {
			t.Errorf("unexpected text: %q", got.Text)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for Flush output")
	}
}

// TestTTSProcessorInterrupt 验证 Interrupt 清空队列并重置状态。
func TestTTSProcessorInterrupt(t *testing.T) {
	provider := newMockTTSProvider()
	provider.delays["第一句，"] = 200 * time.Millisecond

	proc := newTestTTSProcessorWithConfig(provider, 1, 50, 100)

	var mu sync.Mutex
	var chunks []TTSChunk
	proc.OnChunk(func(c TTSChunk) {
		mu.Lock()
		chunks = append(chunks, c)
		mu.Unlock()
	})

	if err := proc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	_ = proc.Write("第一句，第二句，第三句，", defaultOpts)
	time.Sleep(50 * time.Millisecond)

	if err := proc.Interrupt(); err != nil {
		t.Fatalf("Interrupt failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	got := len(chunks)
	mu.Unlock()

	// Interrupt 后不应再有新的 chunk
	if got != 0 {
		t.Errorf("expected 0 chunks after interrupt, got %d", got)
	}
}

// TestTTSProcessorPlaybackOrder 验证并发合成时按入队顺序回调。
func TestTTSProcessorPlaybackOrder(t *testing.T) {
	provider := newMockTTSProvider()
	// 第一句慢，第二句快，验证顺序不变
	provider.delays["First."] = 100 * time.Millisecond
	provider.delays["Second."] = 10 * time.Millisecond

	proc := newTestTTSProcessorWithConfig(provider, 2, 50, 100)

	var mu sync.Mutex
	var order []string
	proc.OnChunk(func(c TTSChunk) {
		mu.Lock()
		order = append(order, c.Text)
		mu.Unlock()
	})

	if err := proc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	_ = proc.Write("First.", defaultOpts)
	_ = proc.Write("Second.", defaultOpts)

	time.Sleep(400 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(order) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(order))
	}
	if order[0] != "First." || order[1] != "Second." {
		t.Errorf("wrong playback order: %v", order)
	}
}

// TestTTSProcessorContextCancel 验证 context 取消后 Stop 正常完成。
func TestTTSProcessorContextCancel(t *testing.T) {
	proc := newTestTTSProcessor(newMockTTSProvider())

	ctx, cancel := context.WithCancel(context.Background())
	if err := proc.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	cancel()
	time.Sleep(50 * time.Millisecond)

	if err := proc.Stop(); err != nil {
		t.Logf("Stop returned (may be expected): %v", err)
	}
}

// TestTTSProcessorSynthesizeError 验证合成失败时不回调（不 panic）。
func TestTTSProcessorSynthesizeError(t *testing.T) {
	provider := newMockTTSProvider()
	provider.callErr = fmt.Errorf("TTS service unavailable")

	proc := newTestTTSProcessor(provider)

	chunkCh := make(chan TTSChunk, 1)
	proc.OnChunk(func(c TTSChunk) { chunkCh <- c })

	if err := proc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	_ = proc.Write("测试。", defaultOpts)

	select {
	case <-chunkCh:
		t.Fatal("should not receive chunk on error")
	case <-time.After(300 * time.Millisecond):
		// 正确：没有回调
	}
}

// TestTTSProcessorConcurrentWrite 验证并发 Write 不 panic。
func TestTTSProcessorConcurrentWrite(t *testing.T) {
	proc := newTestTTSProcessor(newMockTTSProvider())
	if err := proc.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = proc.Write("并发文本。", defaultOpts)
		}()
	}
	wg.Wait()
}

// TestSentenceSplitter_FirstWeakThenStrong 验证分句器两阶段逻辑。
func TestSentenceSplitter_FirstWeakThenStrong(t *testing.T) {
	s := newSentenceSplitter(100)

	// 首句：遇到弱停顿触发
	out := s.feed("你好，")
	if len(out) != 1 || out[0] != "你好，" {
		t.Errorf("expected first sentence at weak boundary, got %v", out)
	}

	// 第二句：弱停顿不触发
	out = s.feed("世界，")
	if len(out) != 0 {
		t.Errorf("subsequent sentence should not trigger at weak boundary, got %v", out)
	}

	// 强停顿触发
	out = s.feed("再见。")
	if len(out) != 1 || out[0] != "世界，再见。" {
		t.Errorf("expected second sentence at strong boundary, got %v", out)
	}
}

// TestSentenceSplitter_MaxRunes 验证超过最大字数时强制切句。
func TestSentenceSplitter_MaxRunes(t *testing.T) {
	s := newSentenceSplitter(5)

	out := s.feed("一二三四五六") // 6 个字，到第 5 个时触发
	if len(out) == 0 {
		t.Fatal("expected at least one sentence from maxRunes trigger")
	}
	if len([]rune(out[0])) > 5 {
		t.Errorf("sentence exceeds maxRunes: %q", out[0])
	}
}

// TestSentenceSplitter_Flush 验证 Flush 输出剩余。
func TestSentenceSplitter_Flush(t *testing.T) {
	s := newSentenceSplitter(100)
	_ = s.feed("没有停顿")
	got := s.flush()
	if got != "没有停顿" {
		t.Errorf("unexpected flush result: %q", got)
	}
	if s.flush() != "" {
		t.Error("second flush should return empty")
	}
}

// TestTruncateText 验证 truncate 辅助函数。
func TestTruncateText(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"你好世界", 2, "你好..."},
	}
	for _, tt := range tests {
		got := truncate(tt.in, tt.n)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}
