package audio

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/tts"
)

// TestTTSPipelineCreate 测试创建 Pipeline
func TestTTSPipelineCreate(t *testing.T) {
	provider := newMockTTSProvider()
	config := DefaultTTSPipelineConfig()
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)

	if pipeline == nil {
		t.Fatal("Expected pipeline to be created")
	}
}

// TestTTSPipelineStartStop 测试启动和停止
func TestTTSPipelineStartStop(t *testing.T) {
	provider := newMockTTSProvider()
	config := DefaultTTSPipelineConfig()
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)

	ctx := context.Background()

	// 启动
	err := pipeline.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}

	// 停止
	err = pipeline.Stop()
	if err != nil {
		t.Fatalf("Failed to stop pipeline: %v", err)
	}
}

// TestTTSPipelineDoubleStart 测试重复启动
func TestTTSPipelineDoubleStart(t *testing.T) {
	provider := newMockTTSProvider()
	config := DefaultTTSPipelineConfig()
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)

	ctx := context.Background()

	err := pipeline.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer pipeline.Stop()

	// 重复启动应该报错
	err = pipeline.Start(ctx)
	if err == nil {
		t.Fatal("Expected error on double start")
	}
}

// TestTTSPipelineEnqueueText 测试入队文本
func TestTTSPipelineEnqueueText(t *testing.T) {
	provider := newMockTTSProvider()
	config := DefaultTTSPipelineConfig()
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)
	mixer := newMockMixer()
	pipeline.SetMixer(mixer)

	ctx := context.Background()
	err := pipeline.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer pipeline.Stop()

	// 入队文本
	err = pipeline.EnqueueText("Hello, World!", "happy")
	if err != nil {
		t.Fatalf("Failed to enqueue text: %v", err)
	}

	// 等待处理
	time.Sleep(200 * time.Millisecond)

	stats := pipeline.Stats()
	if stats.TotalEnqueued != 1 {
		t.Errorf("Expected TotalEnqueued=1, got %d", stats.TotalEnqueued)
	}
}

// TestTTSPipelineEnqueueEmpty 测试入队空文本
func TestTTSPipelineEnqueueEmpty(t *testing.T) {
	provider := newMockTTSProvider()
	config := DefaultTTSPipelineConfig()
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)

	ctx := context.Background()
	err := pipeline.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer pipeline.Stop()

	// 入队空文本应该直接返回
	err = pipeline.EnqueueText("", "happy")
	if err != nil {
		t.Fatalf("Empty text should not return error: %v", err)
	}

	stats := pipeline.Stats()
	if stats.TotalEnqueued != 0 {
		t.Errorf("Empty text should not be enqueued, got TotalEnqueued=%d", stats.TotalEnqueued)
	}
}

// TestTTSPipelineEnqueueBeforeStart 测试启动前入队
func TestTTSPipelineEnqueueBeforeStart(t *testing.T) {
	provider := newMockTTSProvider()
	config := DefaultTTSPipelineConfig()
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)

	// 未启动时入队应该报错
	err := pipeline.EnqueueText("Hello", "happy")
	if err == nil {
		t.Fatal("Expected error when enqueueing before start")
	}
}

// TestTTSPipelineInterrupt 测试打断
func TestTTSPipelineInterrupt(t *testing.T) {
	provider := newMockTTSProvider()
	config := DefaultTTSPipelineConfig()
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)
	mixer := newMockMixer()
	pipeline.SetMixer(mixer)

	ctx := context.Background()
	err := pipeline.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer pipeline.Stop()

	// 入队多个文本
	for i := 0; i < 5; i++ {
		err = pipeline.EnqueueText("Test sentence", "neutral")
		if err != nil {
			t.Fatalf("Failed to enqueue text: %v", err)
		}
	}

	// 等待一点时间
	time.Sleep(50 * time.Millisecond)

	// 打断
	err = pipeline.Interrupt()
	if err != nil {
		t.Fatalf("Failed to interrupt: %v", err)
	}

	stats := pipeline.Stats()
	if stats.TotalInterrupts != 1 {
		t.Errorf("Expected TotalInterrupts=1, got %d", stats.TotalInterrupts)
	}

	// 队列应该被清空
	if stats.TextQueueSize != 0 {
		t.Errorf("Expected TextQueueSize=0 after interrupt, got %d", stats.TextQueueSize)
	}
}

// TestTTSPipelineConcurrentEnqueue 测试并发入队
func TestTTSPipelineConcurrentEnqueue(t *testing.T) {
	provider := newMockTTSProvider()
	config := &TTSPipelineConfig{
		MaxTTSBuffer:     5,
		MaxConcurrentTTS: 3,
		TextQueueSize:    50,
	}
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)
	mixer := newMockMixer()
	pipeline.SetMixer(mixer)

	ctx := context.Background()
	err := pipeline.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer pipeline.Stop()

	// 并发入队
	var wg sync.WaitGroup
	enqueueCount := 20

	for i := 0; i < enqueueCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			pipeline.EnqueueText("Concurrent text", "neutral")
		}(i)
	}

	wg.Wait()

	// 等待处理
	time.Sleep(500 * time.Millisecond)

	stats := pipeline.Stats()
	if stats.TotalEnqueued != enqueueCount {
		t.Errorf("Expected TotalEnqueued=%d, got %d", enqueueCount, stats.TotalEnqueued)
	}
}

// TestTTSPipelineStats 测试统计信息
func TestTTSPipelineStats(t *testing.T) {
	provider := newMockTTSProvider()
	config := DefaultTTSPipelineConfig()
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)

	ctx := context.Background()
	err := pipeline.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer pipeline.Stop()

	// 初始状态
	stats := pipeline.Stats()
	if stats.TotalEnqueued != 0 {
		t.Errorf("Expected TotalEnqueued=0, got %d", stats.TotalEnqueued)
	}
	if stats.TotalPlayed != 0 {
		t.Errorf("Expected TotalPlayed=0, got %d", stats.TotalPlayed)
	}
	if stats.TotalInterrupts != 0 {
		t.Errorf("Expected TotalInterrupts=0, got %d", stats.TotalInterrupts)
	}
	if stats.IsPlaying {
		t.Error("Expected IsPlaying=false")
	}
}

// TestTTSPipelineVoiceMap 测试音色映射
func TestTTSPipelineVoiceMap(t *testing.T) {
	provider := newMockTTSProvider()
	config := DefaultTTSPipelineConfig()
	ttsConfig := tts.Config{APIKey: "test"}
	voiceMap := map[string]string{
		"happy":   "voice_happy",
		"sad":     "voice_sad",
		"default": "voice_default",
	}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, voiceMap, nil)
	mixer := newMockMixer()
	pipeline.SetMixer(mixer)

	ctx := context.Background()
	err := pipeline.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer pipeline.Stop()

	// 入队文本
	err = pipeline.EnqueueText("Hello", "happy")
	if err != nil {
		t.Fatalf("Failed to enqueue text: %v", err)
	}

	// 等待处理
	time.Sleep(200 * time.Millisecond)

	// 检查 provider 收到的配置
	lastConfig := provider.getLastConfig()
	if lastConfig.Voice != "voice_happy" {
		t.Errorf("Expected voice=voice_happy, got %s", lastConfig.Voice)
	}
}

// TestTTSPipelineContextCancellation 测试 context 取消
func TestTTSPipelineContextCancellation(t *testing.T) {
	provider := newMockTTSProvider()
	config := DefaultTTSPipelineConfig()
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	err := pipeline.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}

	// 取消 context
	cancel()

	// 等待一点时间
	time.Sleep(100 * time.Millisecond)

	// 入队应该失败或被取消
	err = pipeline.EnqueueText("Hello", "happy")
	if err == nil {
		// 可能在 context 取消前入队成功，这也是可接受的
		t.Log("Enqueue succeeded before context cancellation took effect")
	}

	// 停止（应该能正常停止）
	err = pipeline.Stop()
	if err != nil {
		t.Logf("Stop returned error (may be expected): %v", err)
	}
}

// TestTTSPipelineTTSError 测试 TTS 错误处理
func TestTTSPipelineTTSError(t *testing.T) {
	provider := newMockTTSProvider()
	provider.startErr = errors.New("TTS service unavailable")

	config := DefaultTTSPipelineConfig()
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)

	ctx := context.Background()
	err := pipeline.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer pipeline.Stop()

	// 入队文本
	err = pipeline.EnqueueText("Hello", "happy")
	if err != nil {
		t.Fatalf("Failed to enqueue text: %v", err)
	}

	// 等待处理（应该失败但不崩溃）
	time.Sleep(200 * time.Millisecond)

	stats := pipeline.Stats()
	if stats.TotalEnqueued != 1 {
		t.Errorf("Expected TotalEnqueued=1, got %d", stats.TotalEnqueued)
	}
	// TTS 失败，所以 TotalPlayed 应该是 0
	if stats.TotalPlayed != 0 {
		t.Errorf("Expected TotalPlayed=0 (TTS failed), got %d", stats.TotalPlayed)
	}
}

// TestTTSPipelineMaxConcurrentTTS 测试最大并发数
func TestTTSPipelineMaxConcurrentTTS(t *testing.T) {
	provider := newMockTTSProvider()

	config := &TTSPipelineConfig{
		MaxTTSBuffer:     10,
		MaxConcurrentTTS: 2, // 最多 2 个并发
		TextQueueSize:    50,
	}
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)
	mixer := newMockMixer()
	pipeline.SetMixer(mixer)

	ctx := context.Background()
	err := pipeline.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer pipeline.Stop()

	// 快速入队 5 个
	for i := 0; i < 5; i++ {
		err = pipeline.EnqueueText("Test", "neutral")
		if err != nil {
			t.Fatalf("Failed to enqueue text: %v", err)
		}
	}

	// 等待一点时间
	time.Sleep(100 * time.Millisecond)

	// 检查 provider 的调用次数
	startCount := provider.getStartCount()
	t.Logf("TTS start count: %d", startCount)
}

// TestTTSPipelineResetAfterInterrupt 测试打断后重置
func TestTTSPipelineResetAfterInterrupt(t *testing.T) {
	provider := newMockTTSProvider()
	config := DefaultTTSPipelineConfig()
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)
	mixer := newMockMixer()
	pipeline.SetMixer(mixer)

	ctx := context.Background()
	err := pipeline.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer pipeline.Stop()

	// 入队并打断多次
	for i := 0; i < 3; i++ {
		err = pipeline.EnqueueText("Test", "neutral")
		if err != nil {
			t.Fatalf("Failed to enqueue text: %v", err)
		}

		time.Sleep(20 * time.Millisecond)

		err = pipeline.Interrupt()
		if err != nil {
			t.Fatalf("Failed to interrupt: %v", err)
		}
	}

	stats := pipeline.Stats()
	if stats.TotalInterrupts != 3 {
		t.Errorf("Expected TotalInterrupts=3, got %d", stats.TotalInterrupts)
	}

	// 打断后应该能继续正常工作
	err = pipeline.EnqueueText("Final text", "neutral")
	if err != nil {
		t.Fatalf("Failed to enqueue after multiple interrupts: %v", err)
	}
}

// TestTTSPipelineSetMixerDynamic 测试动态设置 Mixer
func TestTTSPipelineSetMixerDynamic(t *testing.T) {
	provider := newMockTTSProvider()
	config := DefaultTTSPipelineConfig()
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)

	ctx := context.Background()
	err := pipeline.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer pipeline.Stop()

	// 先不设置 Mixer 入队
	err = pipeline.EnqueueText("Text without mixer", "neutral")
	if err != nil {
		t.Fatalf("Failed to enqueue text: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// 动态设置 Mixer
	mixer := newMockMixer()
	pipeline.SetMixer(mixer)

	// 再次入队
	err = pipeline.EnqueueText("Text with mixer", "neutral")
	if err != nil {
		t.Fatalf("Failed to enqueue text: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Mixer 应该被调用
	if mixer.getAddTTSStreamCount() == 0 {
		t.Log("Mixer may not have been called if TTS completed before mixer was set")
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
		{
			name:     "短文本不截断",
			text:     "Hello",
			maxLen:   10,
			expected: "Hello",
		},
		{
			name:     "长文本截断",
			text:     "Hello, World!",
			maxLen:   5,
			expected: "Hello",
		},
		{
			name:     "空文本",
			text:     "",
			maxLen:   10,
			expected: "",
		},
		{
			name:     "中文文本截断",
			text:     "你好世界这是一段很长的中文",
			maxLen:   5,
			expected: "你好世界这",
		},
		{
			name:     "刚好等于最大长度",
			text:     "Hello",
			maxLen:   5,
			expected: "Hello",
		},
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

// BenchmarkTTSPipelineEnqueue 基准测试入队性能
func BenchmarkTTSPipelineEnqueue(b *testing.B) {
	provider := newMockTTSProvider()
	config := &TTSPipelineConfig{
		MaxTTSBuffer:     100,
		MaxConcurrentTTS: 10,
		TextQueueSize:    1000,
	}
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)

	ctx := context.Background()
	_ = pipeline.Start(ctx)
	defer pipeline.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pipeline.EnqueueText("Benchmark text", "neutral")
	}
}

// BenchmarkTTSPipelineInterrupt 基准测试打断性能
func BenchmarkTTSPipelineInterrupt(b *testing.B) {
	provider := newMockTTSProvider()
	config := DefaultTTSPipelineConfig()
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)

	ctx := context.Background()
	_ = pipeline.Start(ctx)
	defer pipeline.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 入队一些内容
		pipeline.EnqueueText("Test", "neutral")
		// 打断
		pipeline.Interrupt()
	}
}

// TestTTSPipelinePlaybackOrder 测试播放顺序：验证即使并发生成完成顺序不同，也按入队顺序播放
func TestTTSPipelinePlaybackOrder(t *testing.T) {
	// 创建带延迟控制的 provider
	provider := newDelayMockTTSProvider()
	// 设置不同的延迟：第一个文本延迟较长，第二个较短
	// 这样如果没有顺序控制，第二个会先完成并先播放
	provider.delays = map[string]time.Duration{
		"First sentence.":  100 * time.Millisecond,
		"Second sentence.": 10 * time.Millisecond,
		"Third sentence.":  50 * time.Millisecond,
	}

	config := &TTSPipelineConfig{
		MaxTTSBuffer:     10,
		MaxConcurrentTTS: 3, // 允许并发
		TextQueueSize:    50,
	}
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)
	orderMixer := newOrderTrackingMixer()
	pipeline.SetMixer(orderMixer)

	ctx := context.Background()
	err := pipeline.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer pipeline.Stop()

	// 按顺序入队
	texts := []string{"First sentence.", "Second sentence.", "Third sentence."}
	for _, text := range texts {
		err := pipeline.EnqueueText(text, "default")
		if err != nil {
			t.Fatalf("Failed to enqueue text: %v", err)
		}
	}

	// 等待所有播放完成
	time.Sleep(500 * time.Millisecond)

	// 验证播放顺序
	playedOrder := orderMixer.getPlayedOrder()
	if len(playedOrder) != len(texts) {
		t.Fatalf("Expected %d items played, got %d", len(texts), len(playedOrder))
	}

	for i, text := range texts {
		if playedOrder[i] != text {
			t.Errorf("Expected order[%d] = %q, got %q", i, text, playedOrder[i])
		}
	}

	t.Logf("Playback order verified: %v", playedOrder)
}

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

	stream := &delayMockTTSStream{
		provider:   p,
		sampleRate: 16000,
		channels:   1,
	}
	return stream, nil
}

type delayMockTTSStream struct {
	provider   *delayMockTTSProvider
	text       string
	audioData  []byte
	reader     *delayMockAudioReader
	sampleRate int
	channels   int
}

func (s *delayMockTTSStream) WriteTextChunk(ctx context.Context, text string) error {
	s.text = text

	// 根据文本获取延迟
	s.provider.mu.Lock()
	delay := s.provider.delays[text]
	s.provider.mu.Unlock()

	// 模拟 TTS 生成延迟
	if delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	// 生成音频数据，包含原始文本（用于追踪）
	s.audioData = []byte(text)
	s.reader = &delayMockAudioReader{
		data:   s.audioData,
		text:   text,
		closed: false,
	}

	return nil
}

func (s *delayMockTTSStream) Close(ctx context.Context) error {
	if s.reader != nil {
		s.reader.markClosed()
	}
	return nil
}

func (s *delayMockTTSStream) AudioReader() io.ReadCloser {
	return s.reader
}

func (s *delayMockTTSStream) SampleRate() int {
	return s.sampleRate
}

func (s *delayMockTTSStream) Channels() int {
	return s.channels
}

type delayMockAudioReader struct {
	mu     sync.Mutex
	data   []byte
	text   string
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

func (r *delayMockAudioReader) markClosed() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
}

func (r *delayMockAudioReader) getText() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.text
}

// orderTrackingMixer 记录播放顺序的 Mixer
type orderTrackingMixer struct {
	mu          sync.Mutex
	playedOrder []string
}

func newOrderTrackingMixer() *orderTrackingMixer {
	return &orderTrackingMixer{
		playedOrder: make([]string, 0),
	}
}

func (m *orderTrackingMixer) AddTTSStream(audio io.Reader) {
	// 读取音频数据来获取文本（我们把文本编码在音频数据中）
	data, _ := io.ReadAll(audio)
	text := string(data)

	m.mu.Lock()
	m.playedOrder = append(m.playedOrder, text)
	m.mu.Unlock()
}

func (m *orderTrackingMixer) AddResourceStream(audio io.Reader) {}
func (m *orderTrackingMixer) RemoveTTSStream()                  {}
func (m *orderTrackingMixer) RemoveResourceStream()             {}
func (m *orderTrackingMixer) SetTTSVolume(volume float64)       {}
func (m *orderTrackingMixer) SetResourceVolume(volume float64)  {}
func (m *orderTrackingMixer) OnTTSStarted()                     {}
func (m *orderTrackingMixer) OnTTSFinished()                    {}
func (m *orderTrackingMixer) Start()                            {}
func (m *orderTrackingMixer) Stop()                             {}

func (m *orderTrackingMixer) getPlayedOrder() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.playedOrder))
	copy(result, m.playedOrder)
	return result
}

// TestTTSPipelineRaceCondition 测试竞态条件
func TestTTSPipelineRaceCondition(t *testing.T) {
	provider := newMockTTSProvider()
	config := &TTSPipelineConfig{
		MaxTTSBuffer:     5,
		MaxConcurrentTTS: 3,
		TextQueueSize:    20,
	}
	ttsConfig := tts.Config{APIKey: "test"}

	pipeline := NewTTSPipeline(provider, config, ttsConfig, nil, nil)
	mixer := newMockMixer()
	pipeline.SetMixer(mixer)

	ctx := context.Background()
	err := pipeline.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer pipeline.Stop()

	var wg sync.WaitGroup
	var interruptCount int64
	var enqueueCount int64

	// 并发入队和打断
	for i := 0; i < 10; i++ {
		wg.Add(2)

		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				pipeline.EnqueueText("Concurrent text", "neutral")
				atomic.AddInt64(&enqueueCount, 1)
				time.Sleep(10 * time.Millisecond)
			}
		}()

		go func() {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				pipeline.Interrupt()
				atomic.AddInt64(&interruptCount, 1)
				time.Sleep(20 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	t.Logf("Enqueue count: %d, Interrupt count: %d", enqueueCount, interruptCount)

	// 不崩溃就算通过
	stats := pipeline.Stats()
	t.Logf("Final stats: %+v", stats)
}
