package codec

import (
	"os"
	"testing"
)

func TestIsolateBug(t *testing.T) {
	raw, err := os.ReadFile("/tmp/ws_debug_sess_831a4c31811a.pcm")
	if err != nil { t.Skip(err) }
	samples := make([]int16, len(raw)/2)
	for i := range samples { samples[i] = int16(raw[i*2]) | int16(raw[i*2+1])<<8 }

	frameSize := 960

	// Test A: 用全新 codec，只编码→解码 frame 4（第一个语音帧）
	frame4Start := 4 * frameSize  // 3840
	frame4 := samples[frame4Start : frame4Start+frameSize]

	encA, _ := New(FormatOpus, 16000, 1, 60)
	framesA, _ := encA.Encode(frame4)
	decA, _ := New(FormatOpus, 16000, 1, 60)
	pcmA, _ := decA.Decode(framesA[0])

	maxDiffA := 0
	for i := range pcmA {
		diff := int(frame4[i]) - int(pcmA[i])
		if diff < 0 { diff = -diff }
		if diff > maxDiffA { maxDiffA = diff }
	}
	t.Logf("Test A (frame4 only, fresh codec): max_diff=%d", maxDiffA)

	// Test B: 逐步编码帧0-3，再编码帧4，然后用全新解码器解码帧4
	encB, _ := New(FormatOpus, 16000, 1, 60)
	for f := 0; f < 4; f++ {
		_, _ = encB.Encode(samples[f*frameSize : (f+1)*frameSize])
	}
	framesB, _ := encB.Encode(frame4)
	decB, _ := New(FormatOpus, 16000, 1, 60)
	pcmB, _ := decB.Decode(framesB[0])
	maxDiffB := 0
	for i := range pcmB {
		diff := int(frame4[i]) - int(pcmB[i])
		if diff < 0 { diff = -diff }
		if diff > maxDiffB { maxDiffB = diff }
	}
	t.Logf("Test B (frame0-4 encoded, fresh decoder): max_diff=%d", maxDiffB)

	// Test C: 和 Test B 一样编码，但用相同解码器逐步解帧0-3再解帧4
	encC, _ := New(FormatOpus, 16000, 1, 60)
	var allFrames [][]byte
	for f := 0; f < 5; f++ {
		frames, _ := encC.Encode(samples[f*frameSize : (f+1)*frameSize])
		allFrames = append(allFrames, frames...)
	}
	decC, _ := New(FormatOpus, 16000, 1, 60)
	for f := 0; f < 4; f++ {
		_, _ = decC.Decode(allFrames[f]) // 解包0-3，丢弃结果
	}
	pcmCFrame4, _ := decC.Decode(allFrames[4])
	maxDiffC := 0
	for i := range pcmCFrame4 {
		diff := int(frame4[i]) - int(pcmCFrame4[i])
		if diff < 0 { diff = -diff }
		if diff > maxDiffC { maxDiffC = diff }
	}
	t.Logf("Test C (continuous encoder+decoder): max_diff=%d", maxDiffC)

	// Test D: 全新 codec，enc→dec 静音帧0，再 enc→dec 静音帧3，再 enc→dec 语音帧4
	encD, _ := New(FormatOpus, 16000, 1, 60)
	decD, _ := New(FormatOpus, 16000, 1, 60)
	for f := 0; f < 5; f++ {
		frames, _ := encD.Encode(samples[f*frameSize : (f+1)*frameSize])
		if f < 4 {
			_, _ = decD.Decode(frames[0]) // 丢弃
		} else {
			pcmD, _ := decD.Decode(frames[0])
			maxDiffD := 0
			for i := range pcmD {
				diff := int(frame4[i]) - int(pcmD[i])
				if diff < 0 { diff = -diff }
				if diff > maxDiffD { maxDiffD = diff }
			}
			t.Logf("Test D (shared enc+dec, continuous): max_diff=%d", maxDiffD)
		}
	}

	// Summary
	t.Logf("---")
	if maxDiffA < 100 {
		t.Logf("CONCLUSION: Frame4 in isolation is FINE (encoder bug IS state-related)")
	} else {
		t.Logf("CONCLUSION: Frame4 is BROKEN even in isolation (basic encode issue)")
	}
}
