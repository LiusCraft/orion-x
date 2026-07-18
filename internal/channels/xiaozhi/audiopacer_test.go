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

func (r *recordingSender) snapshot() ([][]byte, []time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	frames := make([][]byte, len(r.sent))
	copy(frames, r.sent)
	times := make([]time.Time, len(r.times))
	copy(times, r.times)
	return frames, times
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

func TestAudioPacer_PreBufferSentImmediately(t *testing.T) {
	sender := &recordingSender{}
	p := newAudioPacer(10*time.Millisecond, 3, sender.send, nil, nil, nil)
	defer p.stop()

	start := time.Now()
	for i := 0; i < 3; i++ {
		p.enqueue([]byte{byte(i)})
	}
	waitForCount(t, sender, 3, time.Second)

	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("expected pre-buffer frames to be sent immediately, took %v", elapsed)
	}
}

func TestAudioPacer_PacesFramesAfterPreBuffer(t *testing.T) {
	sender := &recordingSender{}
	const interval = 60 * time.Millisecond
	p := newAudioPacer(interval, 2, sender.send, nil, nil, nil)
	defer p.stop()

	for i := 0; i < 4; i++ {
		p.enqueue([]byte{byte(i)})
	}
	waitForCount(t, sender, 4, 2*time.Second)

	_, times := sender.snapshot()
	// frames[0], frames[1] are pre-buffered (immediate); frames[2], frames[3]
	// should each be paced by ~interval.
	for i := 2; i < len(times); i++ {
		gap := times[i].Sub(times[i-1])
		if gap < interval/2 {
			t.Errorf("frame %d: expected gap >= ~%v, got %v", i, interval/2, gap)
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

func TestAudioPacer_ClearDropsQueuedFrames(t *testing.T) {
	sender := &recordingSender{}
	const interval = 500 * time.Millisecond // long enough that undropped frames would visibly delay the test
	p := newAudioPacer(interval, 1, sender.send, nil, nil, nil)
	defer p.stop()

	p.enqueue([]byte{1}) // pre-buffer: sent immediately
	waitForCount(t, sender, 1, time.Second)

	// These would each wait ~interval if not cleared.
	p.enqueue([]byte{2})
	p.enqueue([]byte{3})
	p.enqueue([]byte{4})

	p.clear()

	// Give the pacer a brief moment in case one frame was already dequeued
	// before clear ran (documented as an acceptable race in audioPacer.clear).
	time.Sleep(50 * time.Millisecond)
	if got := sender.count(); got > 2 {
		t.Errorf("expected at most 1 pre-buffer frame + 1 possibly in-flight frame after clear, got %d frames sent", got)
	}
}

func TestAudioPacer_TurnEndInvokesCallbackAfterQueuedFrames(t *testing.T) {
	sender := &recordingSender{}
	var turnEnded bool
	var mu sync.Mutex
	onTurnEnd := func() {
		mu.Lock()
		turnEnded = true
		mu.Unlock()
	}

	p := newAudioPacer(10*time.Millisecond, 2, sender.send, onTurnEnd, nil, nil)
	defer p.stop()

	p.enqueue([]byte{1})
	p.enqueue([]byte{2})
	p.enqueueTurnEnd()

	waitForCount(t, sender, 2, time.Second)

	deadline := time.After(time.Second)
	for {
		mu.Lock()
		ended := turnEnded
		mu.Unlock()
		if ended {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for onTurnEnd callback")
		case <-time.After(2 * time.Millisecond):
		}
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
