package xstages_test

import (
	"context"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/audio/codec"
	xstages "github.com/liuscraft/orion-x/internal/connector/xiaozhi/stages"
)

func newPCMCodecForTest(t *testing.T) codec.Codec {
	t.Helper()
	c, err := codec.New(codec.FormatPCM, audio.InternalSampleRate, audio.InternalChannels, 60)
	if err != nil {
		t.Fatalf("codec.New(pcm) failed: %v", err)
	}
	return c
}

func TestWSAudioSource_PCMPassthrough(t *testing.T) {
	src := xstages.NewWSAudioSource(newPCMCodecForTest(t), audio.InternalSampleRate)

	in := []int16{1, 2, 3, 4}
	src.PushBinaryFrame(audio.Int16ToBytesLE(in))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	data, err := src.Read(ctx)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	out := audio.BytesToInt16LE(data)
	if len(out) != len(in) {
		t.Fatalf("expected %d samples, got %d", len(in), len(out))
	}
	for i := range in {
		if in[i] != out[i] {
			t.Errorf("sample %d: want %d, got %d", i, in[i], out[i])
		}
	}
}

func TestWSAudioSource_ResamplesWhenRateMismatched(t *testing.T) {
	// 客户端 48kHz，内部标准 16kHz：应产生约 1/3 长度的样本。
	src := xstages.NewWSAudioSource(newPCMCodecForTest(t), 48000)

	in := make([]int16, 480) // 10ms @ 48kHz
	for i := range in {
		in[i] = int16(i)
	}
	src.PushBinaryFrame(audio.Int16ToBytesLE(in))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	data, err := src.Read(ctx)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	out := audio.BytesToInt16LE(data)
	wantLen := 160 // 10ms @ 16kHz
	if out == nil || len(out) < wantLen-5 || len(out) > wantLen+5 {
		t.Errorf("expected ~%d resampled samples, got %d", wantLen, len(out))
	}
}

func TestWSAudioSource_DropsUnderBackpressure(t *testing.T) {
	src := xstages.NewWSAudioSource(newPCMCodecForTest(t), audio.InternalSampleRate)

	// 疯狂推送，远超内部缓冲容量；不应该阻塞。
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			src.PushBinaryFrame(audio.Int16ToBytesLE([]int16{int16(i)}))
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("PushBinaryFrame blocked under backpressure instead of dropping")
	}
}

func TestWSAudioSource_CloseUnblocksRead(t *testing.T) {
	src := xstages.NewWSAudioSource(newPCMCodecForTest(t), audio.InternalSampleRate)

	errCh := make(chan error, 1)
	go func() {
		_, err := src.Read(context.Background())
		errCh <- err
	}()

	if err := src.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	// Close 应该是幂等的。
	if err := src.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected an error (EOF) from Read after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for Read to unblock after Close")
	}
}

func TestWSAudioSource_ContextCancelUnblocksRead(t *testing.T) {
	src := xstages.NewWSAudioSource(newPCMCodecForTest(t), audio.InternalSampleRate)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := src.Read(ctx)
		errCh <- err
	}()

	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected an error from Read after ctx cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for Read to unblock after ctx cancel")
	}
}

func TestWSAudioSource_DecodeErrorDoesNotPanic(t *testing.T) {
	opusCodec, err := codec.New(codec.FormatOpus, audio.InternalSampleRate, audio.InternalChannels, 60)
	if err != nil {
		t.Fatalf("codec.New(opus) failed: %v", err)
	}
	src := xstages.NewWSAudioSource(opusCodec, audio.InternalSampleRate)

	// 塞入不是合法 opus 数据的垃圾字节，解码应该失败但不能 panic，也不能
	// 塞入任何数据。
	src.PushBinaryFrame([]byte{0xff, 0x00, 0x01, 0x02})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := src.Read(ctx); err == nil {
		t.Fatal("expected Read to time out (no data enqueued after decode error)")
	}
}
