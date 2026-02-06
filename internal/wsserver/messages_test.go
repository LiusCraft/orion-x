package wsserver

import (
	"encoding/json"
	"testing"
)

func TestHelloMessageUnmarshal(t *testing.T) {
	payload := []byte(`{
		"type":"hello",
		"device_id":"AA:BB",
		"audio_params":{
			"format":"opus",
			"sample_rate":16000,
			"channels":1,
			"frame_duration":60,
			"bits_per_sample":16,
			"play_buffer_duration":300
		}
	}`)

	var msg HelloMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		t.Fatalf("unmarshal hello failed: %v", err)
	}
	if msg.Type != "hello" || msg.DeviceID != "AA:BB" {
		t.Fatalf("unexpected hello message: %+v", msg)
	}
	if msg.AudioParams == nil || msg.AudioParams.Format != "opus" {
		t.Fatalf("unexpected audio params: %+v", msg.AudioParams)
	}
}

func TestListenMessageUnmarshal(t *testing.T) {
	payload := []byte(`{"type":"listen","mode":"manual","state":"detect","text":"你好","text_response":"ok"}`)
	var msg ListenMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		t.Fatalf("unmarshal listen failed: %v", err)
	}
	if msg.Type != "listen" || msg.State != "detect" || msg.Text == "" {
		t.Fatalf("unexpected listen message: %+v", msg)
	}
}
