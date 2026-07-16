package xstages

import (
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/logging"
)

// pacedFrame is one item in an audioPacer's send queue.
type pacedFrame struct {
	data []byte
	// turnEnd: after sending data (if any), invoke onTurnEnd and restart
	// the pre-buffer window for the next turn.
	turnEnd bool
	// reset: restart the pre-buffer window without sending or ending a
	// turn (used when a new turn's first frame arrives, so it gets the
	// same low-latency start as the very first turn ever sent).
	reset bool
	// sentenceStart: before sending data (if any), invoke onSentenceStart
	// with sentenceText. Like sentenceEnd, this does not reset the pre-buffer
	// window or the sent counter. Enqueued immediately before the first
	// audio frame of a sentence so the client receives "tts sentence_start"
	// at the right point in the stream — not as early as the chunk arrives
	// from the upstream TTS pipeline.
	sentenceStart bool
	// sentenceEnd: after sending data (if any), invoke onSentenceEnd with
	// sentenceText. Does not reset the pre-buffer window or the sent counter
	// — sentence boundaries are internal markers, not turn boundaries.
	sentenceEnd  bool
	sentenceText string
}

// audioPacer sends audio frames to a destination (typically a WebSocket
// connection) at a steady rate — one frame per interval — instead of as
// fast as they're produced.
//
// Without this, a burst of frames arriving together (e.g. because the
// upstream TTS provider delivered a few hundred milliseconds of audio over
// the network in one go, then paused) gets forwarded to the client in the
// same burst-then-stall pattern. Unless the client does its own large
// jitter-buffering, that plays back as stuttering rather than smooth audio.
//
// The first preBuffer frames of a turn are sent immediately to keep
// perceived latency low; subsequent frames are paced at `interval`. This
// mirrors xiaozhi-esp32-server's AudioRateController/PRE_BUFFER_COUNT
// design.
//
// The internal queue is unbounded (a plain slice that grows as needed)
// rather than a fixed-capacity buffer. TTS synthesis can outpace real-time
// playback by a wide margin — a provider can deliver ten seconds of audio
// over the network in a second or two — so a fixed-capacity queue that
// drops frames once full would silently truncate what the user hears once
// a reply runs long, which is worse than the stuttering this pacer exists
// to fix. Memory use is bounded by how much audio a single turn produces,
// which in practice is at most a few hundred KB even for a long reply.
type audioPacer struct {
	interval      time.Duration
	preBuffer     int
	sendFn          func([]byte) error
	onTurnEnd       func()
	onSentenceStart func(text string)
	onSentenceEnd   func(text string)

	mu     sync.Mutex
	items  []pacedFrame
	notify chan struct{} // signaled (non-blocking) whenever items becomes non-empty

	stopCh chan struct{}
	doneCh chan struct{}
}

func newAudioPacer(interval time.Duration, preBuffer int, sendFn func([]byte) error, onTurnEnd func(), onSentenceStart func(text string), onSentenceEnd func(text string)) *audioPacer {
	p := &audioPacer{
		interval:        interval,
		preBuffer:       preBuffer,
		sendFn:          sendFn,
		onTurnEnd:       onTurnEnd,
		onSentenceStart: onSentenceStart,
		onSentenceEnd:   onSentenceEnd,
		notify:        make(chan struct{}, 1),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
	go p.run()
	return p
}

func (p *audioPacer) run() {
	defer close(p.doneCh)

	sent := 0
	for {
		item, ok := p.pop()
		if !ok {
			select {
			case <-p.notify:
				continue
			case <-p.stopCh:
				return
			}
		}

		if item.reset {
			sent = 0
			continue
		}

		if item.sentenceStart && p.onSentenceStart != nil {
			p.onSentenceStart(item.sentenceText)
		}

		if len(item.data) > 0 {
			if sent >= p.preBuffer {
				select {
				case <-time.After(p.interval):
				case <-p.stopCh:
					return
				}
			}
			if err := p.sendFn(item.data); err != nil {
				logging.Warnf("audioPacer: send error: %v", err)
			}
			sent++
		}

		if item.sentenceEnd && p.onSentenceEnd != nil {
			p.onSentenceEnd(item.sentenceText)
		}

		if item.turnEnd {
			sent = 0
			if p.onTurnEnd != nil {
				p.onTurnEnd()
			}
		}
	}
}

// push appends an item to the unbounded queue and wakes run() if it was
// waiting for one.
func (p *audioPacer) push(item pacedFrame) {
	p.mu.Lock()
	p.items = append(p.items, item)
	p.mu.Unlock()

	select {
	case p.notify <- struct{}{}:
	default:
	}
}

// pop removes and returns the head of the queue, or (zero, false) if empty.
func (p *audioPacer) pop() (pacedFrame, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.items) == 0 {
		return pacedFrame{}, false
	}
	item := p.items[0]
	p.items = p.items[1:]
	return item, true
}

// enqueue queues a frame for pacing. Never drops: see the type doc for why
// the queue is unbounded.
func (p *audioPacer) enqueue(data []byte) {
	p.push(pacedFrame{data: data})
}

// enqueueTurnEnd queues the turn-end marker, which triggers onTurnEnd (e.g.
// sending "tts stop") once every frame queued ahead of it has been sent.
func (p *audioPacer) enqueueTurnEnd() {
	p.push(pacedFrame{turnEnd: true})
}

// enqueueSentenceStart queues a sentence-start marker that fires right before
// the next audio frame — the sentence_text arrives at the client just ahead of
// the first byte of the sentence's actual audio, rather than as early as the
// chunk arrives from the upstream TTS pipeline.
func (p *audioPacer) enqueueSentenceStart(text string) {
	p.push(pacedFrame{sentenceStart: true, sentenceText: text})
}

// enqueueSentenceEnd queues a sentence-end marker. Once every frame queued
// ahead of it has been sent, the pacer invokes onSentenceEnd(text) — the
// caller (WSOutputStage) maps that to a "tts sentence_end" JSON message.
// Unlike turnEnd, this does not reset the pre-buffer window or sent counter.
func (p *audioPacer) enqueueSentenceEnd(text string) {
	p.push(pacedFrame{sentenceEnd: true, sentenceText: text})
}

// resetPreBuffer restarts the pre-buffer window (the next preBuffer frames
// after it will be sent immediately) without sending or discarding
// anything already queued. Called when a new turn's first frame arrives.
func (p *audioPacer) resetPreBuffer() {
	p.push(pacedFrame{reset: true})
}

// clear discards every frame currently queued — used on barge-in, where
// playing out stale audio after the user interrupts would be worse than
// cutting it off. A frame the run goroutine has already dequeued (and may
// be mid-send) is not affected; at most one frame's worth of audio can
// still reach the client after clear returns.
func (p *audioPacer) clear() {
	p.mu.Lock()
	p.items = nil
	p.mu.Unlock()
}

// stop terminates the pacer's goroutine and waits for it to exit. Callers
// must not invoke this concurrently with itself; WSOutputStage calls it
// exactly once, from the single goroutine that owns the pacer's lifecycle.
func (p *audioPacer) stop() {
	close(p.stopCh)
	<-p.doneCh
}
