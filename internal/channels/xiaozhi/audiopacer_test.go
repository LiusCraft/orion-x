package xiaozhi

import (
	"sync"
	"testing"
	"time"
)

// recordingSender collects every frame passed to sendFn along with the
// time it arrived, for asserting on pacing behavior.
type recordingSender struct {
	mu    sync.Mutex
	sent  [][]byte
	times []time.Time
}

func (r *recordingSender) send(data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	r.sent = append(r.sent, cp)
	r.times = append(r.times, time.Now())
	return nil
}

func (r *recordingSender) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sent)
}

func waitForCount(t *testing.T, r *recordingSender, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for r.count() < n {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for %d frames, got %d", n, r.count())
		case <-time.After(2 * time.Millisecond):
		}
	}
}

func TestAudioPacer_ResetPreBufferRestartsWindow(t *testing.T) {
	sender := &recordingSender{}
	const interval = 100 * time.Millisecond
	p := newAudioPacer(interval, 1, sender.send, nil, nil, nil)
	defer p.stop()

	// First frame: pre-buffered, immediate.
	p.enqueue([]byte{1})
	waitForCount(t, sender, 1, time.Second)

	// Second frame without reset would be paced (interval delay). Instead,
	// reset the window so it behaves like a fresh turn's first frame.
	p.resetPreBuffer()
	start := time.Now()
	p.enqueue([]byte{2})
	waitForCount(t, sender, 2, time.Second)

	if elapsed := time.Since(start); elapsed > interval/2 {
		t.Errorf("expected frame after resetPreBuffer to be sent immediately, took %v", elapsed)
	}
}

// TestAudioPacer_NeverDropsFramesUnderBurst is the regression test for a
// bug where the queue had a fixed capacity: enqueueing more frames than it
// could hold (e.g. a long TTS reply, which can produce far more audio than
// fits in a few seconds' worth of buffering) silently dropped the excess —
// truncating what the user hears, which is worse than the stuttering the
// pacer exists to fix. The queue must be unbounded: every enqueued frame is
// eventually sent, no matter how far ahead of playback synthesis gets.
func TestAudioPacer_NeverDropsFramesUnderBurst(t *testing.T) {
	sender := &recordingSender{}
	// An interval large enough that sending all frames one at a time would
	// take far longer than the test should run, proving the frames were
	// queued (not dropped) rather than actually paced out during the test.
	p := newAudioPacer(50*time.Millisecond, 1, sender.send, nil, nil, nil)
	defer p.stop()

	const frameCount = 500 // would need a queue capacity this large (or a fixed cap) to lose frames
	for i := 0; i < frameCount; i++ {
		p.enqueue([]byte{byte(i % 256)})
	}

	// Don't wait for all 500 to actually be paced out (that would take
	// 500*50ms = 25s) — just confirm none were dropped by the time enough
	// have arrived to prove the queue accepted all of them.
	waitForCount(t, sender, 5, time.Second)

	p.mu.Lock()
	queued := len(p.items)
	p.mu.Unlock()
	sentSoFar := sender.count()
	// Allow slack for the one frame the run goroutine may have already
	// popped (so it's no longer in p.items) but not yet finished handing to
	// sendFn (so it's not yet reflected in sentSoFar either) — that's an
	// expected in-flight state, not a drop. A gap larger than that would
	// mean frames were actually lost.
	const inFlightSlack = 2
	if accounted := queued + sentSoFar; frameCount-accounted > inFlightSlack {
		t.Fatalf("expected ~%d frames accounted for (queued+sent, +/-%d in-flight), got queued=%d sent=%d — frames were dropped",
			frameCount, inFlightSlack, queued, sentSoFar)
	}
}

func TestAudioPacer_StopIsIdempotentToCallers(t *testing.T) {
	sender := &recordingSender{}
	p := newAudioPacer(10*time.Millisecond, 1, sender.send, nil, nil, nil)

	p.enqueue([]byte{1})
	waitForCount(t, sender, 1, time.Second)

	p.stop()

	// After stop, enqueue should not panic or block (queue send races with
	// a closed run loop, but the channel itself is still open — only the
	// consuming goroutine has exited).
	done := make(chan struct{})
	go func() {
		p.enqueue([]byte{2})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("enqueue after stop blocked unexpectedly")
	}
}
