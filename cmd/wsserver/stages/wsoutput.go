package stages

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/liuscraft/orion-x/cmd/wsserver/wsproto"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/audio/codec"
	"github.com/liuscraft/orion-x/internal/logging"
	textutil "github.com/liuscraft/orion-x/internal/text"
	"github.com/liuscraft/orion-x/pkg/pipeline"
)

// defaultAudioFrameDurationMs is the fallback used when NewWSOutputStage is
// given a non-positive frameDurationMs. It matches internal/audio/codec's
// default Opus frame duration so PCM and Opus share one pacing scheme by
// default. WSOutputStage always buffers incoming PCM up to exactly one
// frame's worth of samples before calling codec.Encode, so every non-empty
// result corresponds to exactly the connection's negotiated frame duration,
// which is what makes pacing by a constant sleep interval correct (see
// audioPacer).
const defaultAudioFrameDurationMs = 60

// defaultAudioPreBufferFrames is the fallback used when NewWSOutputStage is
// given a non-positive preBufferFrames: how many frames at the start of a
// turn are sent immediately (no pacing delay) before switching to
// steady-rate sending — trades a little burstiness at the very start of
// playback for lower perceived latency. Mirrors xiaozhi-esp32-server's
// PRE_BUFFER_COUNT.
const defaultAudioPreBufferFrames = 3

// SafeConn wraps a *websocket.Conn with a mutex. gorilla/websocket only
// allows one concurrent writer per connection, but a connection's hello
// handshake response and its WSOutputStage both need to write to it, so
// they share one SafeConn instance instead of writing to the raw conn
// directly.
type SafeConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// NewSafeConn wraps conn for serialized writes.
func NewSafeConn(conn *websocket.Conn) *SafeConn {
	return &SafeConn{conn: conn}
}

// WriteJSON serializes v as a text frame.
func (c *SafeConn) WriteJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(v)
}

// WriteBinary sends data as a binary frame.
func (c *SafeConn) WriteBinary(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

// WSOutputStage is a sink stage that forwards pipeline messages to a
// WebSocket client: ASR text (as "stt" messages) and TTS audio (as "tts"
// state messages plus binary audio frames). It's the fan-in point for the
// asr->ws_output and tts->ws_output edges in a connection's DAG pipeline —
// see docs on the connection assembly for the full topology.
//
// TTS audio is sent through an audioPacer at a fixed rate rather than as
// fast as TTSStage produces it — the upstream TTS provider can deliver
// audio in network-driven bursts (see docs/wsserver-protocol-design.md),
// and forwarding those bursts as-is makes playback stutter unless the
// client does its own large jitter-buffering.
//
// Sentence-level "sentence_end" state is intentionally not emitted: the
// underlying audio.TTSChunk only marks the final chunk of a turn (Final),
// not individual sentence boundaries, so only "start"/"sentence_start"/
// "stop" are produced.
type WSOutputStage struct {
	*pipeline.BaseStage
	conn      *SafeConn
	sessionID string
	codec     codec.Codec
	frameSize int // samples per frameDurationMs at the connection's TTS sample rate
	pacer     *audioPacer

	mu         sync.Mutex
	ttsStarted bool
	pendingBuf []int16 // PCM samples buffered until they fill one frame

	// currentEmotion tracks the leading emoji from the ongoing LLM turn
	// (reset on interrupt, same semantics as TTSProcessor.currentEmotion).
	currentEmotion string

	// debug: when WS_DEBUG_DUMP=1, raw PCM (before codec Encode) is written
	// to /tmp/ws_debug_<sessionID>.pcm for offline analysis with ffplay.
	debugDump    *os.File // raw PCM before codec
	debugEncoded *os.File // after codec Encode (e.g. Opus packets)
}

// NewWSOutputStage creates a WSOutputStage. c encodes outgoing PCM audio to
// the format negotiated with the client (pcm passthrough or opus).
// sampleRate is the connection's TTS output sample rate — used to size the
// fixed-duration frames fed to c and to the pacing goroutine. A
// non-positive sampleRate falls back to audio.InternalSampleRate rather
// than yielding a zero frameSize, which would make handleTTSChunk's buffer
// loop spin forever without consuming input. frameDurationMs and
// preBufferFrames come from the connection's negotiated audio_params
// (frame_duration/play_buffer_duration); non-positive values fall back to
// defaultAudioFrameDurationMs/defaultAudioPreBufferFrames.
func NewWSOutputStage(conn *SafeConn, sessionID string, c codec.Codec, sampleRate, frameDurationMs, preBufferFrames int) pipeline.Stage {
	if sampleRate <= 0 {
		sampleRate = audio.InternalSampleRate
	}
	if frameDurationMs <= 0 {
		frameDurationMs = defaultAudioFrameDurationMs
	}
	if preBufferFrames <= 0 {
		preBufferFrames = defaultAudioPreBufferFrames
	}
	s := &WSOutputStage{
		BaseStage: pipeline.NewBaseStage("ws_output"),
		conn:      conn,
		sessionID: sessionID,
		codec:     c,
		frameSize: sampleRate * frameDurationMs / 1000,
	}
	if os.Getenv("WS_DEBUG_DUMP") == "1" {
		f, err := os.Create(fmt.Sprintf("/tmp/ws_debug_%s.pcm", sessionID))
		if err == nil {
			s.debugDump = f
			logging.Infof("WSOutputStage[%s]: debug dump enabled -> %s", sessionID, f.Name())
		}
		ef, err2 := os.Create(fmt.Sprintf("/tmp/ws_debug_%s.opus", sessionID))
		if err2 == nil {
			s.debugEncoded = ef
		}
	}
	s.pacer = newAudioPacer(time.Duration(frameDurationMs)*time.Millisecond, preBufferFrames, conn.WriteBinary, s.sendTTSStop, s.sendSentenceStart, s.sendSentenceEnd)
	return s
}

func (s *WSOutputStage) Process(ctx context.Context, input <-chan pipeline.Message) <-chan pipeline.Message {
	output := make(chan pipeline.Message)

	go func() {
		defer close(output)
		defer s.pacer.stop()
		if s.debugDump != nil {
			defer func() {
				_ = s.debugDump.Close()
				logging.Infof("WSOutputStage[%s]: debug dump closed", s.sessionID)
			}()
		}
		if s.debugEncoded != nil {
			defer func() { _ = s.debugEncoded.Close() }()
		}
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-input:
				if !ok {
					return
				}
				s.handleMessage(msg)
			}
		}
	}()

	return output
}

func (s *WSOutputStage) handleMessage(msg pipeline.Message) {
	switch msg.Type {
	case pipeline.MessageTypeInterrupt:
		s.handleInterrupt()
	case pipeline.MessageTypeData:
		switch payload := msg.Payload.(type) {
		case string:
			if source, _ := msg.Metadata.Extra["source"].(string); source == "llm" {
				s.handleLLMText(payload)
			} else {
				s.sendSTT(payload)
			}
		case audio.TTSChunk:
			s.handleTTSChunk(payload)
		}
	}
	// MessageTypeFinished/MessageTypeError: nothing to forward to the
	// client — the real "turn is over" signal for TTS is TTSChunk.Final,
	// and errors are only logged, not exposed over the wire.
}

// handleLLMText inspects the leading emoji of an incoming LLM text chunk.
// When the emotion changes (new emoji differs from currentEmotion), it sends a
// single "llm" message carrying only the emotion — the text itself is not
// forwarded. Chunks without a leading emoji, or whose emoji matches the
// current emotion, are silently ignored.
func (s *WSOutputStage) handleLLMText(text string) {
	emo, _ := textutil.ExtractLeadingEmoji(text)
	if emo == "" {
		return
	}
	s.mu.Lock()
	changed := emo != s.currentEmotion
	if changed {
		s.currentEmotion = emo
	}
	s.mu.Unlock()

	if !changed {
		return
	}
	if err := s.conn.WriteJSON(wsproto.NewLLMMessage(s.sessionID, "", emo)); err != nil {
		logging.Warnf("WSOutputStage: send llm emotion error: %v", err)
	}
}

func (s *WSOutputStage) sendSTT(text string) {
	if text == "" {
		return
	}
	if err := s.conn.WriteJSON(wsproto.NewSTTMessage(s.sessionID, text)); err != nil {
		logging.Warnf("WSOutputStage: send stt message error: %v", err)
	}
}

// handleTTSChunk buffers incoming PCM until it has a full frame's worth of
// samples (frameSize, i.e. the connection's negotiated frame duration at its
// sample rate), then encodes and enqueues it with the pacer. Buffering to a fixed
// size — rather than encoding whatever arrived in one audio.TTSChunk, which
// can be an arbitrary amount — keeps every enqueued frame's duration fixed
// and known, which is what makes the pacer's constant sleep interval
// correct.
func (s *WSOutputStage) handleTTSChunk(chunk audio.TTSChunk) {
	if chunk.Final {
		s.flushAndStop(chunk.SentenceEnd, chunk.SentenceText)
		return
	}

	s.ensureTTSStarted(chunk.Text)

	if len(chunk.Audio) > 0 {
		s.pendingBuf = append(s.pendingBuf, audio.BytesToInt16LE(chunk.Audio)...)

		for len(s.pendingBuf) >= s.frameSize {
			s.encodeAndEnqueue(s.pendingBuf[:s.frameSize])
			s.pendingBuf = s.pendingBuf[s.frameSize:]
		}
		s.compactPendingBuf()
	}

	// 句子结束标记在音频帧之后入队，确保 "tts sentence_end" 消息
	// 不会早于该句的最后一个音频帧到达客户端。
	if chunk.SentenceEnd {
		s.pacer.enqueueSentenceEnd(chunk.SentenceText)
	}
}

// compactPendingBuf copies the remaining carry-over samples into a freshly
// sized slice so repeated re-slicing in handleTTSChunk doesn't keep the
// backing array of a much larger, mostly-consumed allocation alive.
func (s *WSOutputStage) compactPendingBuf() {
	if len(s.pendingBuf) == 0 {
		s.pendingBuf = nil
		return
	}
	remaining := make([]int16, len(s.pendingBuf))
	copy(remaining, s.pendingBuf)
	s.pendingBuf = remaining
}

func (s *WSOutputStage) encodeAndEnqueue(samples []int16) {
	// Debug: dump raw PCM before encoding for offline analysis.
	if s.debugDump != nil {
		buf := audio.Int16ToBytesLE(samples)
		_, _ = s.debugDump.Write(buf)
	}

	frames, err := s.codec.Encode(samples)
	if err != nil {
		logging.Warnf("WSOutputStage: encode audio error: %v", err)
		return
	}
	for _, f := range frames {
		if len(f) == 0 {
			continue
		}
		if s.debugEncoded != nil {
			// Prepend 2-byte big-endian length so the dump is self-delimiting.
			lenBuf := []byte{byte(len(f) >> 8), byte(len(f))}
			_, _ = s.debugEncoded.Write(lenBuf)
			_, _ = s.debugEncoded.Write(f)
		}
		s.pacer.enqueue(f)
	}
}

// ensureTTSStarted sends "tts start" once per turn (on the first non-Final
// chunk) and "tts sentence_start" whenever a chunk carries the leading text
// of a new sentence (audio.TTSProcessor only sets TTSChunk.Text on a
// sentence's first frame). It also restarts the pacer's pre-buffer window
// so each new turn gets the same low-latency start as the first one.
func (s *WSOutputStage) ensureTTSStarted(text string) {
	s.mu.Lock()
	alreadyStarted := s.ttsStarted
	s.ttsStarted = true
	s.mu.Unlock()

	if !alreadyStarted {
		s.pacer.resetPreBuffer()
		if err := s.conn.WriteJSON(wsproto.NewTTSMessage(s.sessionID, wsproto.TTSStateStart, "")); err != nil {
			logging.Warnf("WSOutputStage: send tts start error: %v", err)
		}
	}
	// sentence_start 通过 pacer 排队而非立即发送：TTSStage
	// 一次性推送多句话的全部 chunk 到 pipeline channel，
	// 立即发送会让客户端在第一句音频还没播完时就看到所有
	// 后续句子的 text。
	if text != "" {
		s.pacer.enqueueSentenceStart(text)
	}
}

// flushAndStop drains the pending PCM buffer and any codec-internal
// trailing partial frame (e.g. a partial Opus frame), then queues a
// turn-end marker. The pacer only invokes sendTTSStop once every frame
// queued ahead of that marker has actually been sent — not merely
// enqueued — so "tts stop" never arrives before the last byte of audio.
func (s *WSOutputStage) flushAndStop(hasSentenceEnd bool, sentenceText string) {
	if len(s.pendingBuf) > 0 {
		s.encodeAndEnqueue(s.pendingBuf)
		s.pendingBuf = nil
	}

	frames, err := s.codec.Flush()
	if err != nil {
		logging.Warnf("WSOutputStage: flush codec error: %v", err)
	} else {
		for _, f := range frames {
			if len(f) == 0 {
				continue
			}
			s.pacer.enqueue(f)
		}
	}

	s.mu.Lock()
	wasStarted := s.ttsStarted
	s.ttsStarted = false
	s.mu.Unlock()

	if !wasStarted {
		return // 本轮从未真正播报过音频，没有 start 与之配对，不发 stop
	}
	// sentence_end 入队在所有音频帧之后、turn_end 之前，保证客户端收到顺序正确
	if hasSentenceEnd {
		s.pacer.enqueueSentenceEnd(sentenceText)
	}
	s.pacer.enqueueTurnEnd()
}

// sendTTSStop is the pacer's onTurnEnd callback — invoked from the pacer's
// own goroutine once every frame queued before the turn-end marker has
// actually been written to the connection.
func (s *WSOutputStage) sendTTSStop() {
	if err := s.conn.WriteJSON(wsproto.NewTTSMessage(s.sessionID, wsproto.TTSStateStop, "")); err != nil {
		logging.Warnf("WSOutputStage: send tts stop error: %v", err)
	}
}

// sendSentenceStart is the pacer's onSentenceStart callback — invoked once
// the sentence-start marker reaches the front of the queue, just before the
// first audio frame of that sentence is sent. Unlike the old direct
// WriteJSON in ensureTTSStarted, this ensures the client sees
// "tts sentence_start" at the right point in the audio stream.
func (s *WSOutputStage) sendSentenceStart(text string) {
	if err := s.conn.WriteJSON(wsproto.NewTTSMessage(s.sessionID, wsproto.TTSStateSentenceStart, text)); err != nil {
		logging.Warnf("WSOutputStage: send tts sentence_start error: %v", err)
	}
}

// sendSentenceEnd is the pacer's onSentenceEnd callback — invoked once every
// sentence-end marker in the queue has actually been reached (i.e. all audio
// frames ahead of it have been sent).
func (s *WSOutputStage) sendSentenceEnd(text string) {
	if err := s.conn.WriteJSON(wsproto.NewTTSMessage(s.sessionID, wsproto.TTSStateSentenceEnd, text)); err != nil {
		logging.Warnf("WSOutputStage: send tts sentence_end error: %v", err)
	}
}

// handleInterrupt discards any audio still queued in the pacer — on
// barge-in, playing out stale audio would be worse than cutting it off —
// and sends "tts stop" immediately, without waiting for a Final chunk
// (there may not be one: TTSProcessor.Interrupt aborts the in-flight
// stream rather than finishing it normally).
func (s *WSOutputStage) handleInterrupt() {
	s.pendingBuf = nil
	s.pacer.clear()

	s.mu.Lock()
	wasStarted := s.ttsStarted
	s.ttsStarted = false
	s.currentEmotion = ""
	s.mu.Unlock()

	if !wasStarted {
		return
	}
	s.sendTTSStop()
}
