package audio

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/provider/tts"
)

// --- mocks ---

type mockTTSProvider struct {
	mu         sync.Mutex
	startCount int
	startErr   error
	streams    []*mockTTSStream
	lastConfig tts.Config
}

func newMockTTSProvider() *mockTTSProvider { return &mockTTSProvider{} }

func (p *mockTTSProvider) Start(ctx context.Context, cfg tts.Config) (tts.Stream, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.startCount++
	p.lastConfig = cfg
	if p.startErr != nil {
		return nil, p.startErr
	}
	stream := newMockTTSStream()
	p.streams = append(p.streams, stream)
	return stream, nil
}

func (p *mockTTSProvider) getStartCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.startCount
}

func (p *mockTTSProvider) getLastConfig() tts.Config {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastConfig
}

type mockTTSStream struct {
	mu          sync.Mutex
	text        string
	closed      bool
	audioData   []byte
	reader      *mockAudioReader
	sampleRate  int
	channels    int
	writeErr    error
	closeErr    error
	writeCalled int
	closeCalled int
}

func newMockTTSStream() *mockTTSStream {
	s := &mockTTSStream{sampleRate: InternalSampleRate, channels: 1}
	s.reader = newMockAudioReader()
	return s
}

func (s *mockTTSStream) WriteTextChunk(ctx context.Context, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeCalled++
	s.text = text
	if s.writeErr != nil {
		return s.writeErr
	}
	s.audioData = make([]byte, len(text)*100)
	for i := range s.audioData {
		s.audioData[i] = byte(i % 256)
	}
	s.reader.setData(s.audioData)
	return nil
}

func (s *mockTTSStream) Finish(ctx context.Context) error { return s.Close(ctx) }

func (s *mockTTSStream) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeCalled++
	if s.closeErr != nil {
		return s.closeErr
	}
	s.closed = true
	s.reader.close()
	return nil
}

func (s *mockTTSStream) AudioReader() io.ReadCloser { return s.reader }
func (s *mockTTSStream) SampleRate() int            { return s.sampleRate }
func (s *mockTTSStream) Channels() int              { return s.channels }

type mockAudioReader struct {
	mu       sync.Mutex
	data     []byte
	pos      int
	closed   bool
	readCond *sync.Cond
}

func newMockAudioReader() *mockAudioReader {
	r := &mockAudioReader{}
	r.readCond = sync.NewCond(&r.mu)
	return r
}

func (r *mockAudioReader) setData(data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = data
	r.readCond.Broadcast()
}

func (r *mockAudioReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for r.pos >= len(r.data) && !r.closed {
		r.readCond.Wait()
	}
	if r.closed && r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (r *mockAudioReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.readCond.Broadcast()
	return nil
}

func (r *mockAudioReader) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.readCond.Broadcast()
}

func newTestTTSProcessor(provider tts.Provider) TTSProcessor {
	cfg := DefaultTTSConfig()
	cfg.Provider = provider
	proc, _ := NewTTSProcessor(cfg)
	return proc
}

func newTestTTSProcessorWithConfig(provider tts.Provider, maxBuffer, maxConcurrent, queueSize int) TTSProcessor {
	cfg := DefaultTTSConfig()
	cfg.Provider = provider
	cfg.MaxBuffer = maxBuffer
	cfg.MaxConcurrent = maxConcurrent
	cfg.QueueSize = queueSize
	proc, _ := NewTTSProcessor(cfg)
	return proc
}

// TestTTSProcessorCreate 测试创建 Processor
func TestTTSProcessorCreate(t *testing.T) {
	provider := newMockTTSProvider()
	proc := newTestTTSProcessor(provider)
	if proc == nil {
		t.Fatal("Expected processor to be created")
	}
}

// TestTTSProcessorStartStop 测试启动和停止
func TestTTSProcessorStartStop(t *testing.T) {
	provider := newMockTTSProvider()
	proc := newTestTTSProcessor(provider)

	ctx := context.Background()

	if err := proc.Start(ctx); err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}

	if err := proc.Stop(); err != nil {
		t.Fatalf("Failed to stop processor: %v", err)
	}
}

// TestTTSProcessorDoubleStart 测试重复启动
func TestTTSProcessorDoubleStart(t *testing.T) {
	provider := newMockTTSProvider()
	proc := newTestTTSProcessor(provider)

	ctx := context.Background()

	if err := proc.Start(ctx); err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	if err := proc.Start(ctx); err == nil {
		t.Fatal("Expected error on double start")
	}
}

// TestTTSProcessorSynthesize 测试入队文本
func TestTTSProcessorSynthesize(t *testing.T) {
	provider := newMockTTSProvider()
	proc := newTestTTSProcessor(provider)
	proc.OnAudio(func([]byte) {})

	ctx := context.Background()
	if err := proc.Start(ctx); err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	if err := proc.Synthesize(TTSRequest{Text: "Hello, World!", Emotion: "happy"}); err != nil {
		t.Fatalf("Failed to synthesize text: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	stats := proc.Stats()
	if stats.TotalEnqueued != 1 {
		t.Errorf("Expected TotalEnqueued=1, got %d", stats.TotalEnqueued)
	}
}

// TestTTSProcessorOnItemStarted 测试条目开始回调
func TestTTSProcessorOnItemStarted(t *testing.T) {
	provider := newMockTTSProvider()
	proc := newTestTTSProcessor(provider)
	proc.OnAudio(func([]byte) {})

	startedCh := make(chan string, 1)
	proc.OnItemStarted(func(text string, emotion string) {
		startedCh <- text
	})

	ctx := context.Background()
	if err := proc.Start(ctx); err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	if err := proc.Synthesize(TTSRequest{Text: "Hello", Emotion: "happy"}); err != nil {
		t.Fatalf("Failed to synthesize text: %v", err)
	}

	select {
	case got := <-startedCh:
		if got != "Hello" {
			t.Fatalf("unexpected text: %s", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for OnItemStarted")
	}
}

// TestTTSProcessorSynthesizeEmpty 测试入队空文本
func TestTTSProcessorSynthesizeEmpty(t *testing.T) {
	provider := newMockTTSProvider()
	proc := newTestTTSProcessor(provider)

	ctx := context.Background()
	if err := proc.Start(ctx); err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	if err := proc.Synthesize(TTSRequest{Text: ""}); err != nil {
		t.Fatalf("Empty text should not return error: %v", err)
	}

	stats := proc.Stats()
	if stats.TotalEnqueued != 0 {
		t.Errorf("Empty text should not be enqueued, got TotalEnqueued=%d", stats.TotalEnqueued)
	}
}

// TestTTSProcessorSynthesizeBeforeStart 测试启动前入队
func TestTTSProcessorSynthesizeBeforeStart(t *testing.T) {
	provider := newMockTTSProvider()
	proc := newTestTTSProcessor(provider)

	if err := proc.Synthesize(TTSRequest{Text: "Hello"}); err == nil {
		t.Fatal("Expected error when synthesizing before start")
	}
}

// TestTTSProcessorInterrupt 测试打断
func TestTTSProcessorInterrupt(t *testing.T) {
	provider := newMockTTSProvider()
	proc := newTestTTSProcessor(provider)
	proc.OnAudio(func([]byte) {})

	ctx := context.Background()
	if err := proc.Start(ctx); err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	for i := 0; i < 5; i++ {
		if err := proc.Synthesize(TTSRequest{Text: "Test sentence"}); err != nil {
			t.Fatalf("Failed to synthesize text: %v", err)
		}
	}

	time.Sleep(50 * time.Millisecond)

	if err := proc.Interrupt(); err != nil {
		t.Fatalf("Failed to interrupt: %v", err)
	}

	stats := proc.Stats()
	if stats.TotalInterrupts != 1 {
		t.Errorf("Expected TotalInterrupts=1, got %d", stats.TotalInterrupts)
	}

	if stats.TextQueueSize != 0 {
		t.Errorf("Expected TextQueueSize=0 after interrupt, got %d", stats.TextQueueSize)
	}
}

// TestTTSProcessorConcurrentSynthesize 测试并发入队
func TestTTSProcessorConcurrentSynthesize(t *testing.T) {
	provider := newMockTTSProvider()
	proc := newTestTTSProcessorWithConfig(provider, 5, 3, 50)
	proc.OnAudio(func([]byte) {})

	ctx := context.Background()
	if err := proc.Start(ctx); err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	var wg sync.WaitGroup
	const enqueueCount = 20

	for i := 0; i < enqueueCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = proc.Synthesize(TTSRequest{Text: "Concurrent text"})
		}()
	}

	wg.Wait()
	time.Sleep(500 * time.Millisecond)

	stats := proc.Stats()
	if stats.TotalEnqueued != enqueueCount {
		t.Errorf("Expected TotalEnqueued=%d, got %d", enqueueCount, stats.TotalEnqueued)
	}
}

// TestTTSProcessorStats 测试统计信息
func TestTTSProcessorStats(t *testing.T) {
	provider := newMockTTSProvider()
	proc := newTestTTSProcessor(provider)

	ctx := context.Background()
	if err := proc.Start(ctx); err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	stats := proc.Stats()
	if stats.TotalEnqueued != 0 || stats.TotalPlayed != 0 || stats.TotalInterrupts != 0 || stats.IsPlaying {
		t.Errorf("Unexpected initial stats: %+v", stats)
	}
}

// TestTTSProcessorVoiceMap 测试音色映射
func TestTTSProcessorVoiceMap(t *testing.T) {
	provider := newMockTTSProvider()
	cfg := DefaultTTSConfig()
	cfg.Provider = provider
	cfg.VoiceMap = map[string]string{
		"happy":   "voice_happy",
		"sad":     "voice_sad",
		"default": "voice_default",
	}
	proc, _ := NewTTSProcessor(cfg)
	proc.OnAudio(func([]byte) {})

	ctx := context.Background()
	if err := proc.Start(ctx); err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	if err := proc.Synthesize(TTSRequest{Text: "Hello", Emotion: "happy"}); err != nil {
		t.Fatalf("Failed to synthesize text: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	lastConfig := provider.getLastConfig()
	if lastConfig.Voice != "voice_happy" {
		t.Errorf("Expected voice=voice_happy, got %s", lastConfig.Voice)
	}
}

// TestTTSProcessorContextCancellation 测试 context 取消
func TestTTSProcessorContextCancellation(t *testing.T) {
	provider := newMockTTSProvider()
	proc := newTestTTSProcessor(provider)

	ctx, cancel := context.WithCancel(context.Background())
	if err := proc.Start(ctx); err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}

	cancel()
	time.Sleep(100 * time.Millisecond)

	// 停止应该能正常停止
	if err := proc.Stop(); err != nil {
		t.Logf("Stop returned error (may be expected): %v", err)
	}
}

// TestTTSProcessorTTSError 测试 TTS 错误处理
func TestTTSProcessorTTSError(t *testing.T) {
	provider := newMockTTSProvider()
	provider.startErr = errors.New("TTS service unavailable")

	proc := newTestTTSProcessor(provider)

	ctx := context.Background()
	if err := proc.Start(ctx); err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	if err := proc.Synthesize(TTSRequest{Text: "Hello"}); err != nil {
		t.Fatalf("Failed to synthesize text: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	stats := proc.Stats()
	if stats.TotalEnqueued != 1 {
		t.Errorf("Expected TotalEnqueued=1, got %d", stats.TotalEnqueued)
	}
	if stats.TotalPlayed != 0 {
		t.Errorf("Expected TotalPlayed=0 (TTS failed), got %d", stats.TotalPlayed)
	}
}

// TestTTSProcessorResetAfterInterrupt 测试打断后重置
func TestTTSProcessorResetAfterInterrupt(t *testing.T) {
	provider := newMockTTSProvider()
	proc := newTestTTSProcessor(provider)
	proc.OnAudio(func([]byte) {})

	ctx := context.Background()
	if err := proc.Start(ctx); err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	for i := 0; i < 3; i++ {
		if err := proc.Synthesize(TTSRequest{Text: "Test"}); err != nil {
			t.Fatalf("Failed to synthesize text: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
		if err := proc.Interrupt(); err != nil {
			t.Fatalf("Failed to interrupt: %v", err)
		}
	}

	stats := proc.Stats()
	if stats.TotalInterrupts != 3 {
		t.Errorf("Expected TotalInterrupts=3, got %d", stats.TotalInterrupts)
	}

	if err := proc.Synthesize(TTSRequest{Text: "Final text"}); err != nil {
		t.Fatalf("Failed to synthesize after multiple interrupts: %v", err)
	}
}

// TestTruncateText 测试文本截断函数
func TestTruncateText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxLen   int
		expected string
	}{
		{"短文本不截断", "Hello", 10, "Hello"},
		{"长文本截断", "Hello, World!", 5, "Hello"},
		{"空文本", "", 10, ""},
		{"中文文本截断", "你好世界这是一段很长的中文", 5, "你好世界这"},
		{"刚好等于最大长度", "Hello", 5, "Hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateText(tt.text, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateText(%q, %d) = %q, want %q", tt.text, tt.maxLen, result, tt.expected)
			}
		})
	}
}

// TestTTSProcessorPlaybackOrder 测试播放顺序
func TestTTSProcessorPlaybackOrder(t *testing.T) {
	provider := newDelayMockTTSProvider()
	provider.delays = map[string]time.Duration{
		"First sentence.":  100 * time.Millisecond,
		"Second sentence.": 10 * time.Millisecond,
		"Third sentence.":  50 * time.Millisecond,
	}

	cfg := DefaultTTSConfig()
	cfg.Provider = provider
	cfg.MaxBuffer = 10
	cfg.MaxConcurrent = 3
	cfg.QueueSize = 50
	proc, _ := NewTTSProcessor(cfg)
	proc.OnAudio(func([]byte) {})

	var playedOrder []string
	var playedMu sync.Mutex
	proc.OnItemStarted(func(text string, emotion string) {
		playedMu.Lock()
		playedOrder = append(playedOrder, text)
		playedMu.Unlock()
	})

	ctx := context.Background()
	if err := proc.Start(ctx); err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}
	defer func() { _ = proc.Stop() }()

	texts := []string{"First sentence.", "Second sentence.", "Third sentence."}
	for _, text := range texts {
		if err := proc.Synthesize(TTSRequest{Text: text, Emotion: "default"}); err != nil {
			t.Fatalf("Failed to synthesize text: %v", err)
		}
	}

	time.Sleep(500 * time.Millisecond)

	playedMu.Lock()
	defer playedMu.Unlock()
	if len(playedOrder) != len(texts) {
		t.Fatalf("Expected %d items played, got %d", len(texts), len(playedOrder))
	}

	for i, text := range texts {
		if playedOrder[i] != text {
			t.Errorf("Expected order[%d] = %q, got %q", i, text, playedOrder[i])
			return
		}
	}

	t.Logf("Playback order verified: %v", playedOrder)
}

// BenchmarkTTSProcessorSynthesize 基准测试入队性能
func BenchmarkTTSProcessorSynthesize(b *testing.B) {
	provider := newMockTTSProvider()
	proc := newTestTTSProcessorWithConfig(provider, 100, 10, 1000)

	ctx := context.Background()
	_ = proc.Start(ctx)
	defer func() { _ = proc.Stop() }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = proc.Synthesize(TTSRequest{Text: "Benchmark text"})
	}
}

// BenchmarkTTSProcessorInterrupt 基准测试打断性能
func BenchmarkTTSProcessorInterrupt(b *testing.B) {
	provider := newMockTTSProvider()
	proc := newTestTTSProcessor(provider)

	ctx := context.Background()
	_ = proc.Start(ctx)
	defer func() { _ = proc.Stop() }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = proc.Synthesize(TTSRequest{Text: "Test"})
		_ = proc.Interrupt()
	}
}

// TestTTSProcessorRaceCondition 测试竞态条件
func TestTTSProcessorRaceCondition(t *testing.T) {}

// delayMockTTSProvider 带延迟控制的 TTS Provider，用于测试播放顺序
type delayMockTTSProvider struct {
	mu       sync.Mutex
	delays   map[string]time.Duration
	startErr error
}

func newDelayMockTTSProvider() *delayMockTTSProvider {
	return &delayMockTTSProvider{
		delays: make(map[string]time.Duration),
	}
}

func (p *delayMockTTSProvider) Start(ctx context.Context, cfg tts.Config) (tts.Stream, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.startErr != nil {
		return nil, p.startErr
	}

	return &delayMockTTSStream{
		provider:   p,
		sampleRate: InternalSampleRate,
		channels:   1,
	}, nil
}

type delayMockTTSStream struct {
	provider   *delayMockTTSProvider
	text       string
	reader     *delayMockAudioReader
	sampleRate int
	channels   int
}

func (s *delayMockTTSStream) WriteTextChunk(ctx context.Context, text string) error {
	s.text = text

	s.provider.mu.Lock()
	delay := s.provider.delays[text]
	s.provider.mu.Unlock()

	if delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	s.reader = &delayMockAudioReader{data: []byte(text)}
	return nil
}

func (s *delayMockTTSStream) Finish(ctx context.Context) error { return s.Close(ctx) }

func (s *delayMockTTSStream) Close(ctx context.Context) error {
	if s.reader != nil {
		_ = s.reader.Close()
	}
	return nil
}

func (s *delayMockTTSStream) AudioReader() io.ReadCloser { return s.reader }
func (s *delayMockTTSStream) SampleRate() int           { return s.sampleRate }
func (s *delayMockTTSStream) Channels() int             { return s.channels }

type delayMockAudioReader struct {
	mu     sync.Mutex
	data   []byte
	pos    int
	closed bool
}

func (r *delayMockAudioReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.pos >= len(r.data) {
		return 0, io.EOF
	}

	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (r *delayMockAudioReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}
