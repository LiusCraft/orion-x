package stages_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/pipeline"
	"github.com/liuscraft/orion-x/internal/pipeline/stages"
	"github.com/liuscraft/orion-x/internal/wsproto"
)

// newTestWSConnPair spins up a real WebSocket server (httptest) and dials
// it, returning the server-side and client-side *websocket.Conn. This lets
// WSOutputStage tests exercise the real gorilla/websocket read/write path
// instead of a hand-rolled fake.
func newTestWSConnPair(t *testing.T) (server, client *websocket.Conn) {
	t.Helper()

	upgrader := websocket.Upgrader{}
	connCh := make(chan *websocket.Conn, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connCh <- c
	}))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	t.Cleanup(func() { _ = clientConn.Close() })

	select {
	case serverConn := <-connCh:
		t.Cleanup(func() { _ = serverConn.Close() })
		return serverConn, clientConn
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for server-side connection")
		return nil, nil
	}
}

func readJSONWithTimeout(t *testing.T, conn *websocket.Conn, timeout time.Duration) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	var raw map[string]any
	if err := conn.ReadJSON(&raw); err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}
	return raw
}

func readBinaryWithTimeout(t *testing.T, conn *websocket.Conn, timeout time.Duration) []byte {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	msgType, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if msgType != websocket.BinaryMessage {
		t.Fatalf("expected binary message, got type %d", msgType)
	}
	return data
}

// testWSSampleRate is used for most WSOutputStage tests so a "one frame"
// worth of PCM (60ms at this rate, the frameDurationMs passed to
// NewWSOutputStage in these tests) is a small, easy-to-construct 60
// samples, instead of the production 960 (@16kHz). PCM passthrough doesn't
// constrain the sample rate, so this is safe to pick purely for test
// convenience.
const testWSSampleRate = 1000

// testFrameSamples builds n identical fixed-duration frames worth of PCM,
// with sample values counting up from start so tests can identify which
// frame they received.
func testFrameSamples(start int16, frames int) []int16 {
	const samplesPerFrame = testWSSampleRate * 60 / 1000 // 60ms frameDurationMs
	out := make([]int16, samplesPerFrame*frames)
	for i := range out {
		out[i] = start + int16(i/samplesPerFrame)
	}
	return out
}

func TestWSOutputStage_STTMessage(t *testing.T) {
	server, client := newTestWSConnPair(t)
	stage := stages.NewWSOutputStage(stages.NewSafeConn(server), "sess-1", newPCMCodecForTest(t), testWSSampleRate, 60, 3)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan pipeline.Message, 1)
	_ = stage.Process(ctx, input)

	input <- pipeline.NewMessage(pipeline.MessageTypeData, "你好世界")

	raw := readJSONWithTimeout(t, client, time.Second)
	if raw["type"] != string(wsproto.TypeSTT) {
		t.Errorf("expected stt message, got %v", raw["type"])
	}
	if raw["text"] != "你好世界" {
		t.Errorf("unexpected text: %v", raw["text"])
	}
	if raw["session_id"] != "sess-1" {
		t.Errorf("unexpected session_id: %v", raw["session_id"])
	}
}

// TestWSOutputStage_TTSChunkFlow 验证完整的 start/sentence_start/音频帧/stop
// 流程。音频块大小正好是一个完整 frame（60 samples @ testWSSampleRate），
// 这样每个 TTSChunk 恰好产生一次编码+发送，不依赖任何跨 chunk 的缓冲行为。
func TestWSOutputStage_TTSChunkFlow(t *testing.T) {
	server, client := newTestWSConnPair(t)
	stage := stages.NewWSOutputStage(stages.NewSafeConn(server), "sess-1", newPCMCodecForTest(t), testWSSampleRate, 60, 3)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan pipeline.Message, 4)
	_ = stage.Process(ctx, input)

	firstChunk := audio.TTSChunk{Text: "你好", Audio: audio.Int16ToBytesLE(testFrameSamples(1, 1))}
	midChunk := audio.TTSChunk{Audio: audio.Int16ToBytesLE(testFrameSamples(4, 1))}
	finalChunk := audio.TTSChunk{Final: true}

	input <- pipeline.Message{Type: pipeline.MessageTypeData, Payload: firstChunk}

	start := readJSONWithTimeout(t, client, time.Second)
	if start["type"] != string(wsproto.TypeTTS) || start["state"] != string(wsproto.TTSStateStart) {
		t.Fatalf("expected tts start, got %+v", start)
	}
	sentenceStart := readJSONWithTimeout(t, client, time.Second)
	if sentenceStart["state"] != string(wsproto.TTSStateSentenceStart) || sentenceStart["text"] != "你好" {
		t.Fatalf("expected tts sentence_start with text, got %+v", sentenceStart)
	}
	audioFrame := readBinaryWithTimeout(t, client, time.Second)
	if got := audio.BytesToInt16LE(audioFrame); len(got) == 0 || got[0] != 1 {
		t.Fatalf("unexpected first audio frame: %v", got)
	}

	input <- pipeline.Message{Type: pipeline.MessageTypeData, Payload: midChunk}
	audioFrame2 := readBinaryWithTimeout(t, client, time.Second)
	if got := audio.BytesToInt16LE(audioFrame2); len(got) == 0 || got[0] != 4 {
		t.Fatalf("unexpected second audio frame: %v", got)
	}

	input <- pipeline.Message{Type: pipeline.MessageTypeData, Payload: finalChunk}
	stop := readJSONWithTimeout(t, client, time.Second)
	if stop["state"] != string(wsproto.TTSStateStop) {
		t.Fatalf("expected tts stop, got %+v", stop)
	}
}

func TestWSOutputStage_InterruptSendsStopIfStarted(t *testing.T) {
	server, client := newTestWSConnPair(t)
	stage := stages.NewWSOutputStage(stages.NewSafeConn(server), "sess-1", newPCMCodecForTest(t), testWSSampleRate, 60, 3)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan pipeline.Message, 4)
	_ = stage.Process(ctx, input)

	input <- pipeline.Message{Type: pipeline.MessageTypeData, Payload: audio.TTSChunk{Text: "在播报", Audio: audio.Int16ToBytesLE(testFrameSamples(1, 1))}}
	_ = readJSONWithTimeout(t, client, time.Second)   // start
	_ = readJSONWithTimeout(t, client, time.Second)   // sentence_start
	_ = readBinaryWithTimeout(t, client, time.Second) // audio frame

	input <- pipeline.Message{Type: pipeline.MessageTypeInterrupt}
	stop := readJSONWithTimeout(t, client, time.Second)
	if stop["state"] != string(wsproto.TTSStateStop) {
		t.Fatalf("expected tts stop after interrupt, got %+v", stop)
	}
}

// TestWSOutputStage_InterruptBeforeTTSStartSendsNothing 是独立的测试用例
// （而不是复用同一连接接着测），因为触发读超时的连接按 gorilla/websocket
// 的文档不保证还能继续可靠读取，不适合在断言超时之后再复用同一个 conn。
func TestWSOutputStage_InterruptBeforeTTSStartSendsNothing(t *testing.T) {
	server, client := newTestWSConnPair(t)
	stage := stages.NewWSOutputStage(stages.NewSafeConn(server), "sess-1", newPCMCodecForTest(t), testWSSampleRate, 60, 3)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan pipeline.Message, 1)
	_ = stage.Process(ctx, input)

	input <- pipeline.Message{Type: pipeline.MessageTypeInterrupt}

	_ = client.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	var raw map[string]any
	if err := client.ReadJSON(&raw); err == nil {
		t.Fatalf("expected no message when interrupting before any tts start, got %+v", raw)
	}
}

func TestWSOutputStage_IgnoresFinishedAndErrorMessages(t *testing.T) {
	server, client := newTestWSConnPair(t)
	stage := stages.NewWSOutputStage(stages.NewSafeConn(server), "sess-1", newPCMCodecForTest(t), testWSSampleRate, 60, 3)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan pipeline.Message, 2)
	_ = stage.Process(ctx, input)

	input <- pipeline.Message{Type: pipeline.MessageTypeFinished}
	input <- pipeline.Message{Type: pipeline.MessageTypeError, Metadata: pipeline.Metadata{Error: errors.New("boom")}}
	// 紧接着发一个真正的 stt 消息，确认前两条没有产生任何输出干扰这条消息
	// 的到达顺序（如果 Finished/Error 意外写了什么，这里会先读到它）。
	input <- pipeline.NewMessage(pipeline.MessageTypeData, "之后的消息")

	raw := readJSONWithTimeout(t, client, time.Second)
	if raw["type"] != string(wsproto.TypeSTT) || raw["text"] != "之后的消息" {
		t.Fatalf("expected only the stt message to be forwarded, got %+v", raw)
	}
}

// TestWSOutputStage_PacesFramesBeyondPreBuffer is the regression test for
// the "burst then stall" playback stutter: without pacing, WSOutputStage
// forwarded audio as fast as TTSStage produced it, so a multi-second burst
// from the upstream TTS provider arrived at the client all at once. This
// feeds far more than the pre-buffer window's worth of frames in a single
// TTSChunk and asserts that frames beyond the pre-buffer window arrive
// spaced out, not back-to-back.
func TestWSOutputStage_PacesFramesBeyondPreBuffer(t *testing.T) {
	server, client := newTestWSConnPair(t)
	stage := stages.NewWSOutputStage(stages.NewSafeConn(server), "sess-1", newPCMCodecForTest(t), testWSSampleRate, 60, 3)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan pipeline.Message, 16)
	_ = stage.Process(ctx, input)

	const totalFrames = 6 // well beyond the 3-frame pre-buffer window
	input <- pipeline.Message{
		Type:    pipeline.MessageTypeData,
		Payload: audio.TTSChunk{Text: "长文本", Audio: audio.Int16ToBytesLE(testFrameSamples(1, totalFrames))},
	}

	_ = readJSONWithTimeout(t, client, time.Second) // start
	_ = readJSONWithTimeout(t, client, time.Second) // sentence_start

	frameTimes := make([]time.Time, 0, totalFrames)
	for i := 0; i < totalFrames; i++ {
		_ = readBinaryWithTimeout(t, client, 2*time.Second)
		frameTimes = append(frameTimes, time.Now())
	}

	// Frames beyond the pre-buffer window (index >= 3, i.e. the 4th frame
	// onward) should be paced ~60ms apart. Allow a generous lower bound
	// (20ms) to keep this robust against scheduler jitter while still
	// clearly distinguishing "paced" from "all sent back-to-back in
	// microseconds", which is what the bug looked like.
	const preBufferFrames = 3 // must match the preBufferFrames passed to NewWSOutputStage above
	for i := preBufferFrames; i < len(frameTimes); i++ {
		gap := frameTimes[i].Sub(frameTimes[i-1])
		if gap < 20*time.Millisecond {
			t.Errorf("frame %d: expected a paced gap (~60ms), got %v (looks unpaced)", i, gap)
		}
	}
}

// TestWSOutputStage_InterruptDropsQueuedFrames verifies barge-in discards
// whatever audio is still queued in the pacer instead of playing it out —
// otherwise the client would keep hearing stale speech for
// (queued frames * frameDurationMs) after the user interrupted.
func TestWSOutputStage_InterruptDropsQueuedFrames(t *testing.T) {
	server, client := newTestWSConnPair(t)
	stage := stages.NewWSOutputStage(stages.NewSafeConn(server), "sess-1", newPCMCodecForTest(t), testWSSampleRate, 60, 3)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	input := make(chan pipeline.Message, 16)
	_ = stage.Process(ctx, input)

	const totalFrames = 10
	input <- pipeline.Message{
		Type:    pipeline.MessageTypeData,
		Payload: audio.TTSChunk{Audio: audio.Int16ToBytesLE(testFrameSamples(1, totalFrames))},
	}

	_ = readJSONWithTimeout(t, client, time.Second) // start

	// Drain only the pre-buffer window's worth of frames (sent immediately);
	// the remaining 7 are still sitting in the pacer's queue, paced at
	// ~60ms each — which would take ~420ms to fully drain if not dropped.
	const preBufferFrames = 3 // must match the preBufferFrames passed to NewWSOutputStage above
	for i := 0; i < preBufferFrames; i++ {
		_ = readBinaryWithTimeout(t, client, time.Second)
	}

	input <- pipeline.Message{Type: pipeline.MessageTypeInterrupt}

	start := time.Now()
	stop := readJSONWithTimeout(t, client, 200*time.Millisecond)
	if stop["state"] != string(wsproto.TTSStateStop) {
		t.Fatalf("expected tts stop after interrupt, got %+v", stop)
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("stop arrived after %v; queued frames were likely not dropped (would take ~%dms to drain)",
			elapsed, (totalFrames-preBufferFrames)*60)
	}
}

func TestSafeConn_ConcurrentWritesDoNotRace(t *testing.T) {
	server, client := newTestWSConnPair(t)
	safe := stages.NewSafeConn(server)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			_ = safe.WriteJSON(map[string]any{"i": i})
		}
	}()
	for i := 0; i < 20; i++ {
		_ = safe.WriteBinary([]byte{byte(i)})
	}
	<-done

	// 排空客户端收到的消息，确认没有 panic / 连接损坏。
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	count := 0
	for count < 40 {
		if _, _, err := client.ReadMessage(); err != nil {
			break
		}
		count++
	}
	if count == 0 {
		t.Fatal("expected to read at least some messages from concurrent writes")
	}
}
