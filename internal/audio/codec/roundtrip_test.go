package codec

import (
	"os"
	"testing"
)

func TestRoundtripFullPCM(t *testing.T) {
	// 读取原始 PCM
	raw, err := os.ReadFile("/tmp/ws_debug_sess_831a4c31811a.pcm")
	if err != nil {
		t.Skip(err)
	}
	samples := make([]int16, len(raw)/2)
	for i := range samples {
		samples[i] = int16(raw[i*2]) | int16(raw[i*2+1])<<8
	}

	// 用项目 codec 编码
	enc, err := New(FormatOpus, 16000, 1, 60)
	if err != nil {
		t.Fatal(err)
	}

	frameSize := 16000 * 60 / 1000 // 960
	var allFrames [][]byte
	for i := 0; i+frameSize <= len(samples); i += frameSize {
		frames, encErr := enc.Encode(samples[i : i+frameSize])
		if encErr != nil {
			t.Fatalf("encode at %d: %v", i, encErr)
		}
		allFrames = append(allFrames, frames...)
	}
	// flush remaining
	rest := len(samples) % frameSize
	if rest > 0 {
		_, _ = enc.Encode(samples[len(samples)-rest:])
	}
	flushFrames, _ := enc.Flush()
	allFrames = append(allFrames, flushFrames...)

	t.Logf("encoded %d frames", len(allFrames))

	// 用项目 codec 解码
	dec, err := New(FormatOpus, 16000, 1, 60)
	if err != nil {
		t.Fatal(err)
	}

	var decodedAll []int16
	for i, f := range allFrames {
		if len(f) == 0 {
			continue
		}
		pcm, decErr := dec.Decode(f)
		if decErr != nil {
			t.Fatalf("decode frame %d: %v", i, decErr)
		}
		decodedAll = append(decodedAll, pcm...)
	}

	t.Logf("original: %d samples, decoded: %d samples", len(samples), len(decodedAll))

	// 逐帧对比
	minLen := len(samples)
	if len(decodedAll) < minLen {
		minLen = len(decodedAll)
	}
	badFrames := 0
	for i := 0; i+frameSize <= minLen; i += frameSize {
		frameIdx := i / frameSize
		maxDiff := 0
		for j := 0; j < frameSize && i+j < minLen; j++ {
			diff := int(samples[i+j]) - int(decodedAll[i+j])
			if diff < 0 {
				diff = -diff
			}
			if diff > maxDiff {
				maxDiff = diff
			}
		}
		if maxDiff > 100 {
			badFrames++
			if badFrames <= 5 {
				t.Logf("frame %d: max_diff=%d", frameIdx, maxDiff)
			}
		}
	}
	t.Logf("bad frames (>100 diff): %d / %d", badFrames, minLen/frameSize)
	
	if badFrames > 0 {
		// 这是大问题
		for i := 0; i+frameSize <= minLen; i += frameSize {
			frameIdx := i / frameSize
			maxDiff := 0
			for j := 0; j < frameSize && i+j < minLen; j++ {
				diff := int(samples[i+j]) - int(decodedAll[i+j])
				if diff < 0 { diff = -diff }
				if diff > maxDiff { maxDiff = diff }
			}
			if maxDiff < 10 && frameIdx < 10 {
				t.Logf("  GOOD frame %d @%.2fs: max_diff=%d", frameIdx, float64(i)/16000, maxDiff)
			}
		}
	}
}
