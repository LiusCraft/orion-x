package audio

import (
	"context"
	"testing"
	"time"
)

func TestNewOutPipe(t *testing.T) {
	pipe := NewOutPipe("test-api-key")
	if pipe == nil {
		t.Fatal("NewOutPipe returned nil")
	}
}

func TestNewOutPipeWithConfig(t *testing.T) {
	cfg := DefaultOutPipeConfig()
	cfg.TTS.APIKey = "config-key"
	cfg.VoiceMap = map[string]string{
		"default": "voice-x",
	}

	pipe := NewOutPipeWithConfig(cfg)
	if pipe == nil {
		t.Fatal("NewOutPipeWithConfig returned nil")
	}
}

func TestOutPipe_SetMixer(t *testing.T) {
	pipe := NewOutPipe("test-api-key")
	mixer := newMockMixer()
	pipe.SetMixer(mixer)
}

func TestOutPipe_SetReferenceSink(t *testing.T) {
	pipe := NewOutPipe("test-api-key")
	sink := newMockReferenceSink()
	pipe.SetReferenceSink(sink)
}

func TestOutPipe_StartStop(t *testing.T) {
	pipe := NewOutPipe("test-api-key")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := pipe.Start(ctx)
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	err = pipe.Stop()
	if err != nil {
		t.Fatalf("Stop error: %v", err)
	}
}

func TestOutPipe_PlayTTS_EmptyText(t *testing.T) {
	pipe := NewOutPipe("test-api-key")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := pipe.Start(ctx)
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer pipe.Stop()

	err = pipe.PlayTTS("", "happy")
	if err != nil {
		t.Errorf("PlayTTS with empty text should return nil, got: %v", err)
	}
}

func TestOutPipe_PlayTTS_Async(t *testing.T) {
	pipe := NewOutPipe("test-api-key")
	mixer := newMockMixer()
	pipe.SetMixer(mixer)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := pipe.Start(ctx)
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer pipe.Stop()

	// PlayTTS 应该是异步的，立即返回
	start := time.Now()
	err = pipe.PlayTTS("Hello, World!", "happy")
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("PlayTTS error: %v", err)
	}

	// 异步调用应该很快返回（不等待 TTS 完成）
	if duration > 100*time.Millisecond {
		t.Errorf("PlayTTS took too long: %v (expected < 100ms for async)", duration)
	}
}

func TestOutPipe_Interrupt(t *testing.T) {
	pipe := NewOutPipe("test-api-key")
	mixer := newMockMixer()
	pipe.SetMixer(mixer)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := pipe.Start(ctx)
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer pipe.Stop()

	// 入队一些文本
	for i := 0; i < 3; i++ {
		err = pipe.PlayTTS("Test sentence", "neutral")
		if err != nil {
			t.Fatalf("PlayTTS error: %v", err)
		}
	}

	// 打断
	err = pipe.Interrupt()
	if err != nil {
		t.Fatalf("Interrupt error: %v", err)
	}

	// 打断后应该能继续工作
	err = pipe.PlayTTS("After interrupt", "happy")
	if err != nil {
		t.Fatalf("PlayTTS after interrupt error: %v", err)
	}
}

func TestOutPipe_Stats(t *testing.T) {
	pipe := NewOutPipe("test-api-key")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := pipe.Start(ctx)
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer pipe.Stop()

	stats := pipe.Stats()
	if stats.TotalEnqueued != 0 {
		t.Errorf("Expected TotalEnqueued=0, got %d", stats.TotalEnqueued)
	}

	// 入队一些文本
	err = pipe.PlayTTS("Test", "neutral")
	if err != nil {
		t.Fatalf("PlayTTS error: %v", err)
	}

	// 等待一点时间让队列处理
	time.Sleep(50 * time.Millisecond)

	stats = pipe.Stats()
	if stats.TotalEnqueued != 1 {
		t.Errorf("Expected TotalEnqueued=1, got %d", stats.TotalEnqueued)
	}
}

func TestOutPipe_PlayResource(t *testing.T) {
	pipe := NewOutPipe("test-api-key")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := pipe.Start(ctx)
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer pipe.Stop()

	// 没有设置 mixer 时应该报错
	err = pipe.PlayResource(nil)
	if err == nil {
		t.Error("PlayResource without mixer should return error")
	}

	// 设置 mixer
	mixer := newMockMixer()
	pipe.SetMixer(mixer)

	// 现在应该成功
	reader := newMockAudioReader()
	reader.setData([]byte{0x00, 0x01, 0x02, 0x03})
	err = pipe.PlayResource(reader)
	if err != nil {
		t.Errorf("PlayResource with mixer should succeed: %v", err)
	}
}

func TestOutPipe_DoubleStart(t *testing.T) {
	pipe := NewOutPipe("test-api-key")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := pipe.Start(ctx)
	if err != nil {
		t.Fatalf("First Start error: %v", err)
	}
	defer pipe.Stop()

	// 第二次启动应该报错（因为 Pipeline 已经启动）
	err = pipe.Start(ctx)
	if err == nil {
		t.Error("Second Start should return error")
	}
}

func TestOutPipe_InterruptBeforeStart(t *testing.T) {
	pipe := NewOutPipe("test-api-key")

	// 未启动时打断应该不会崩溃
	err := pipe.Interrupt()
	// 可能返回错误或 nil，但不应该 panic
	_ = err
}

func TestOutPipe_MultipleInterrupts(t *testing.T) {
	pipe := NewOutPipe("test-api-key")
	mixer := newMockMixer()
	pipe.SetMixer(mixer)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := pipe.Start(ctx)
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer pipe.Stop()

	// 多次打断应该正常工作
	for i := 0; i < 5; i++ {
		err = pipe.PlayTTS("Test", "neutral")
		if err != nil {
			t.Fatalf("PlayTTS error: %v", err)
		}

		err = pipe.Interrupt()
		if err != nil {
			t.Fatalf("Interrupt error: %v", err)
		}
	}

	stats := pipe.Stats()
	if stats.TotalInterrupts != 5 {
		t.Errorf("Expected TotalInterrupts=5, got %d", stats.TotalInterrupts)
	}
}
