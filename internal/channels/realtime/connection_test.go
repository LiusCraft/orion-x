package realtime

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/liuscraft/orion-x/internal/audio"
	wire "github.com/liuscraft/orion-x/internal/realtime"
)

type testASR struct {
	vadEnabled []bool
}

func (a *testASR) Write([]byte) error { return nil }

func (a *testASR) OnResult(func(audio.ASRResult)) {}

func (a *testASR) OnSpeechStart(func()) {}

func (a *testASR) Start(context.Context) error { return nil }

func (a *testASR) Stop() error { return nil }

func (a *testASR) BeginTurn(context.Context) error { return nil }

func (a *testASR) EndTurn(context.Context) error { return nil }

func (a *testASR) SetVADEnabled(enabled bool) error {
	a.vadEnabled = append(a.vadEnabled, enabled)
	return nil
}

func TestPCM16Resample(t *testing.T) {
	input := make([]byte, 4)
	binary.LittleEndian.PutUint16(input[0:2], 1000)
	binary.LittleEndian.PutUint16(input[2:4], 2000)

	output, err := pcm16Resample(input, 16000, 24000)
	if err != nil {
		t.Fatalf("pcm16Resample() error = %v", err)
	}
	if len(output) != 6 {
		t.Fatalf("len(output) = %d, want 6", len(output))
	}
}

func TestPCM16ResampleRejectsOddBytes(t *testing.T) {
	if _, err := pcm16Resample([]byte{1}, 24000, 16000); err == nil {
		t.Fatal("pcm16Resample() error = nil, want odd-byte error")
	}
}

func TestPCM16ResampleRejectsUnsupportedRate(t *testing.T) {
	if _, err := pcm16Resample(make([]byte, 2), 1, 16000); err == nil {
		t.Fatal("pcm16Resample() error = nil, want unsupported-rate error")
	}
}

func TestValidateInputAudioFormat(t *testing.T) {
	for _, format := range []*wire.AudioFormat{
		{Type: "audio/pcm", Rate: 8000},
		{Type: "audio/pcm", SampleRate: 48000},
	} {
		if err := validateInputAudioFormat(format); err != nil {
			t.Errorf("validateInputAudioFormat(%#v) error = %v", format, err)
		}
	}
	if err := validateInputAudioFormat(&wire.AudioFormat{Type: "audio/pcm", Rate: 1}); err == nil {
		t.Fatal("validateInputAudioFormat() error = nil, want unsupported-rate error")
	}
}

func TestDefaultSessionConfig(t *testing.T) {
	cfg := defaultSessionConfig()
	if cfg.Type != "realtime" {
		t.Errorf("cfg.Type = %q, want realtime", cfg.Type)
	}
	if cfg.Audio == nil || cfg.Audio.Input == nil || cfg.Audio.Input.Format.Rate != 24000 {
		t.Fatalf("cfg.Audio.Input.Format = %#v, want 24000 Hz", cfg.Audio)
	}
	if cfg.Audio.Input.TurnDetection == nil || cfg.Audio.Input.TurnDetection.Type != "server_vad" {
		t.Fatalf("turn detection = %#v, want server_vad", cfg.Audio.Input.TurnDetection)
	}
}

func TestSessionConfigForVADDisabled(t *testing.T) {
	cfg := sessionConfigForVAD(false)
	if cfg.Audio == nil || cfg.Audio.Input == nil {
		t.Fatal("session config has no input audio configuration")
	}
	if cfg.Audio.Input.TurnDetection != nil {
		t.Fatalf("turn detection = %#v, want nil", cfg.Audio.Input.TurnDetection)
	}
}

func TestHandleAudioClearRejectsActiveTurn(t *testing.T) {
	conn := &connection{turnOpen: true}
	if err := conn.handleAudioClear(); err == nil {
		t.Fatal("handleAudioClear() error = nil, want active-turn error")
	}
}

func TestApplyTurnDetectionNullDisablesVAD(t *testing.T) {
	parsed, err := wire.ParseClientEvent([]byte(`{"type":"session.update","session":{"audio":{"input":{"turn_detection":null}}}}`))
	if err != nil {
		t.Fatalf("ParseClientEvent() error = %v", err)
	}
	asr := &testASR{}
	conn := &connection{
		asr:               asr,
		turnDetectionType: "server_vad",
		createResponse:    true,
		interruptResponse: true,
	}
	if err := conn.applyTurnDetection(parsed.Session.Audio.Input); err != nil {
		t.Fatalf("applyTurnDetection() error = %v", err)
	}
	if len(asr.vadEnabled) != 1 || asr.vadEnabled[0] {
		t.Fatalf("SetVADEnabled calls = %v, want [false]", asr.vadEnabled)
	}
	if conn.turnDetectionType != "" {
		t.Errorf("turnDetectionType = %q, want empty", conn.turnDetectionType)
	}
	if conn.createResponse {
		t.Fatal("createResponse = true, want false for manual turn detection")
	}
}

func TestApplyTurnDetectionRejectsUnsupportedType(t *testing.T) {
	asr := &testASR{}
	conn := &connection{asr: asr}
	input := &wire.AudioInputConfig{TurnDetection: &wire.TurnDetection{Type: "semantic_vad"}}
	if err := conn.applyTurnDetection(input); err == nil {
		t.Fatal("applyTurnDetection() error = nil, want unsupported type error")
	}
	if len(asr.vadEnabled) != 0 {
		t.Fatalf("SetVADEnabled calls = %v, want none", asr.vadEnabled)
	}
}

func TestServerAudioDeltaUsesGAFields(t *testing.T) {
	data, err := json.Marshal(wire.ServerEvent{
		Type:         wire.ServerResponseOutputAudioDelta,
		OutputIndex:  0,
		ContentIndex: 0,
		Delta:        "AQI=",
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got["delta"] != "AQI=" {
		t.Errorf("delta = %v, want AQI=", got["delta"])
	}
	if got["output_index"] != float64(0) {
		t.Errorf("output_index = %v, want 0", got["output_index"])
	}
	if got["content_index"] != float64(0) {
		t.Errorf("content_index = %v, want 0", got["content_index"])
	}
	if _, ok := got["audio"]; ok {
		t.Fatalf("audio field = %v, want absent", got["audio"])
	}
}
