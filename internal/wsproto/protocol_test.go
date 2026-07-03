package wsproto

import (
	"encoding/json"
	"testing"
)

func TestParseClientMessage_Hello(t *testing.T) {
	data := []byte(`{
		"type": "hello",
		"device_id": "dev-1",
		"audio_params": {"format": "opus", "sample_rate": 16000, "channels": 1},
		"mode": "manual"
	}`)

	msg, err := ParseClientMessage(data)
	if err != nil {
		t.Fatalf("ParseClientMessage failed: %v", err)
	}
	hello, ok := msg.(*HelloMessage)
	if !ok {
		t.Fatalf("expected *HelloMessage, got %T", msg)
	}
	if hello.DeviceID != "dev-1" {
		t.Errorf("unexpected device id: %q", hello.DeviceID)
	}
	if hello.AudioParams.Format != "opus" || hello.AudioParams.SampleRate != 16000 {
		t.Errorf("unexpected audio params: %+v", hello.AudioParams)
	}
	if hello.Mode != ModeManual {
		t.Errorf("expected mode manual, got %q", hello.Mode)
	}
}

func TestParseClientMessage_HelloExtendedAudioParams(t *testing.T) {
	data := []byte(`{
		"type": "hello",
		"device_id": "dev-1",
		"audio_params": {
			"format": "opus",
			"sample_rate": 16000,
			"channels": 1,
			"frame_duration": 60,
			"bits_per_sample": 16,
			"play_buffer_duration": 2000
		}
	}`)

	msg, err := ParseClientMessage(data)
	if err != nil {
		t.Fatalf("ParseClientMessage failed: %v", err)
	}
	hello := msg.(*HelloMessage)
	want := AudioParams{
		Format:             "opus",
		SampleRate:         16000,
		Channels:           1,
		FrameDuration:      60,
		BitsPerSample:      16,
		PlayBufferDuration: 2000,
	}
	if hello.AudioParams != want {
		t.Errorf("unexpected audio params: got %+v, want %+v", hello.AudioParams, want)
	}
}

func TestParseClientMessage_HelloOmitsExtendedAudioParams(t *testing.T) {
	data := []byte(`{"type": "hello", "audio_params": {"format": "pcm", "sample_rate": 16000, "channels": 1}}`)

	msg, err := ParseClientMessage(data)
	if err != nil {
		t.Fatalf("ParseClientMessage failed: %v", err)
	}
	hello := msg.(*HelloMessage)
	if hello.AudioParams.FrameDuration != 0 || hello.AudioParams.BitsPerSample != 0 || hello.AudioParams.PlayBufferDuration != 0 {
		t.Errorf("expected omitted extended fields to zero-value, got %+v", hello.AudioParams)
	}
}

func TestParseClientMessage_Listen(t *testing.T) {
	data := []byte(`{"type": "listen", "state": "start"}`)
	msg, err := ParseClientMessage(data)
	if err != nil {
		t.Fatalf("ParseClientMessage failed: %v", err)
	}
	listen, ok := msg.(*ListenMessage)
	if !ok {
		t.Fatalf("expected *ListenMessage, got %T", msg)
	}
	if listen.State != ListenStart {
		t.Errorf("expected state start, got %q", listen.State)
	}
}

func TestParseClientMessage_ListenDetectWithText(t *testing.T) {
	data := []byte(`{"type": "listen", "state": "detect", "text": "hello there"}`)
	msg, err := ParseClientMessage(data)
	if err != nil {
		t.Fatalf("ParseClientMessage failed: %v", err)
	}
	listen := msg.(*ListenMessage)
	if listen.State != ListenDetect || listen.Text != "hello there" {
		t.Errorf("unexpected listen message: %+v", listen)
	}
}

func TestParseClientMessage_Abort(t *testing.T) {
	data := []byte(`{"type": "abort", "session_id": "s1"}`)
	msg, err := ParseClientMessage(data)
	if err != nil {
		t.Fatalf("ParseClientMessage failed: %v", err)
	}
	abort, ok := msg.(*AbortMessage)
	if !ok {
		t.Fatalf("expected *AbortMessage, got %T", msg)
	}
	if abort.SessionID != "s1" {
		t.Errorf("unexpected session id: %q", abort.SessionID)
	}
}

func TestParseClientMessage_UnsupportedType(t *testing.T) {
	for _, typ := range []string{"iot", "mcp", "server", "ping", "unknown"} {
		data := []byte(`{"type": "` + typ + `"}`)
		if _, err := ParseClientMessage(data); err == nil {
			t.Errorf("expected error for unsupported type %q", typ)
		}
	}
}

func TestParseClientMessage_InvalidJSON(t *testing.T) {
	if _, err := ParseClientMessage([]byte(`not json`)); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseClientMessage_MalformedHello(t *testing.T) {
	// audio_params 类型不匹配（应为 object，实际是 string）应该报错。
	data := []byte(`{"type": "hello", "audio_params": "oops"}`)
	if _, err := ParseClientMessage(data); err == nil {
		t.Fatal("expected error for malformed hello message")
	}
}

func TestNewHelloResponse_RoundTrip(t *testing.T) {
	resp := NewHelloResponse("sess-1", AudioParams{Format: "pcm", SampleRate: 24000, Channels: 1}, ModeAuto, "welcome")

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded HelloMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.Type != TypeHello || decoded.SessionID != "sess-1" || decoded.Mode != ModeAuto {
		t.Errorf("unexpected round-tripped hello response: %+v", decoded)
	}
	if decoded.AudioParams.SampleRate != 24000 {
		t.Errorf("unexpected sample rate: %d", decoded.AudioParams.SampleRate)
	}
}

func TestNewSTTMessage(t *testing.T) {
	msg := NewSTTMessage("sess-1", "识别文本")
	if msg.Type != TypeSTT || msg.SessionID != "sess-1" || msg.Text != "识别文本" {
		t.Errorf("unexpected stt message: %+v", msg)
	}
}

func TestNewTTSMessage(t *testing.T) {
	msg := NewTTSMessage("sess-1", TTSStateSentenceStart, "你好")
	if msg.Type != TypeTTS || msg.State != TTSStateSentenceStart || msg.Text != "你好" {
		t.Errorf("unexpected tts message: %+v", msg)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if raw["state"] != "sentence_start" {
		t.Errorf("unexpected state field in JSON: %v", raw["state"])
	}
}

func TestTTSMessage_OmitsEmptyTextField(t *testing.T) {
	msg := NewTTSMessage("sess-1", TTSStateStop, "")
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if _, present := raw["text"]; present {
		t.Errorf("expected empty text field to be omitted, got: %v", raw)
	}
}
