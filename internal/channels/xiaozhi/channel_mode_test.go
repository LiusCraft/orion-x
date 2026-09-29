package xiaozhi

import (
	"context"
	"testing"

	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/channels/xiaozhi/wsproto"
)

// fakeASRProcessor 记录 SetVADEnabled/BeginTurn/EndTurn 调用，用来单测拾音
// 模式的落地逻辑（applyAnnouncedMode/handleListen），不碰真实 recognizer。
type fakeASRProcessor struct {
	vadSwitches []bool
	beginCalls  int
	endCalls    int
}

func (f *fakeASRProcessor) Write([]byte) error             { return nil }
func (f *fakeASRProcessor) OnResult(func(audio.ASRResult)) {}
func (f *fakeASRProcessor) OnSpeechStart(func())           {}
func (f *fakeASRProcessor) Start(context.Context) error    { return nil }
func (f *fakeASRProcessor) Stop() error                    { return nil }
func (f *fakeASRProcessor) BeginTurn(context.Context) error {
	f.beginCalls++
	return nil
}
func (f *fakeASRProcessor) EndTurn(context.Context) error {
	f.endCalls++
	return nil
}
func (f *fakeASRProcessor) SetVADEnabled(enabled bool) error {
	f.vadSwitches = append(f.vadSwitches, enabled)
	return nil
}

func newModeTestConnection(mode wsproto.Mode) (*wsConnection, *fakeASRProcessor) {
	proc := &fakeASRProcessor{}
	return &wsConnection{
		sessionID: "sess-1",
		mode:      mode,
		asrProc:   proc,
		ctx:       context.Background(),
	}, proc
}

// TestApplyAnnouncedMode：客户端在 listen start 上宣告的 mode 决定 ASR 处理
// 方式；realtime/未知值按 auto 处理，空值沿用当前模式。
func TestApplyAnnouncedMode(t *testing.T) {
	tests := []struct {
		name         string
		initial      wsproto.Mode
		announced    wsproto.Mode
		wantMode     wsproto.Mode
		wantSwitches []bool
	}{
		{
			name: "manual announced switches VAD off", initial: wsproto.ModeAuto,
			announced: wsproto.ModeManual, wantMode: wsproto.ModeManual, wantSwitches: []bool{false},
		},
		{
			name: "auto announced switches VAD on", initial: wsproto.ModeManual,
			announced: wsproto.ModeAuto, wantMode: wsproto.ModeAuto, wantSwitches: []bool{true},
		},
		{
			name: "same mode is a no-op", initial: wsproto.ModeAuto,
			announced: wsproto.ModeAuto, wantMode: wsproto.ModeAuto,
		},
		{
			name: "empty announced keeps the current mode", initial: wsproto.ModeManual,
			announced: "", wantMode: wsproto.ModeManual,
		},
		{
			name: "realtime degrades to auto", initial: wsproto.ModeManual,
			announced: wsproto.ModeRealtime, wantMode: wsproto.ModeAuto, wantSwitches: []bool{true},
		},
		{
			name: "unknown value degrades to auto", initial: wsproto.ModeManual,
			announced: "banana", wantMode: wsproto.ModeAuto, wantSwitches: []bool{true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, proc := newModeTestConnection(tc.initial)
			c.applyAnnouncedMode(tc.announced)

			if c.mode != tc.wantMode {
				t.Errorf("mode = %q, want %q", c.mode, tc.wantMode)
			}
			if len(proc.vadSwitches) != len(tc.wantSwitches) {
				t.Fatalf("SetVADEnabled calls = %v, want %v", proc.vadSwitches, tc.wantSwitches)
			}
			for i, want := range tc.wantSwitches {
				if proc.vadSwitches[i] != want {
					t.Errorf("SetVADEnabled call %d = %v, want %v", i, proc.vadSwitches[i], want)
				}
			}
		})
	}
}

// TestHandleListenDrivesManualTurns：manual 模式下 listen start/stop 驱动
// BeginTurn/EndTurn；auto 模式（未宣告 mode 时沿用）下两者都不被调用。
func TestHandleListenDrivesManualTurns(t *testing.T) {
	manualConn, manualProc := newModeTestConnection(wsproto.ModeManual)
	manualConn.handleListen(&wsproto.ListenMessage{State: wsproto.ListenStart})
	manualConn.handleListen(&wsproto.ListenMessage{State: wsproto.ListenStop})
	if manualProc.beginCalls != 1 || manualProc.endCalls != 1 {
		t.Fatalf("BeginTurn/EndTurn calls = %d/%d, want 1/1",
			manualProc.beginCalls, manualProc.endCalls)
	}

	autoConn, autoProc := newModeTestConnection(wsproto.ModeAuto)
	autoConn.handleListen(&wsproto.ListenMessage{State: wsproto.ListenStart})
	autoConn.handleListen(&wsproto.ListenMessage{State: wsproto.ListenStop})
	if autoProc.beginCalls != 0 || autoProc.endCalls != 0 {
		t.Fatalf("auto mode must not drive recognizer turns, got BeginTurn/EndTurn = %d/%d",
			autoProc.beginCalls, autoProc.endCalls)
	}
}

// TestHandleListenAppliesAnnouncedMode：listen start 上的 mode 即使与建连时
// 不同也要立即生效（真实固件的 mode 只在这里出现），并且同一轮就按新模式
// 驱动 recognizer。
func TestHandleListenAppliesAnnouncedMode(t *testing.T) {
	c, proc := newModeTestConnection(wsproto.ModeAuto)
	c.handleListen(&wsproto.ListenMessage{State: wsproto.ListenStart, Mode: wsproto.ModeManual})

	if c.mode != wsproto.ModeManual {
		t.Fatalf("mode = %q, want manual", c.mode)
	}
	if len(proc.vadSwitches) != 1 || proc.vadSwitches[0] {
		t.Fatalf("SetVADEnabled calls = %v, want [false]", proc.vadSwitches)
	}
	if proc.beginCalls != 1 {
		t.Fatalf("BeginTurn calls = %d, want 1", proc.beginCalls)
	}
}
