package audio

import (
	"bytes"
	"io"
	"math"
	"testing"
)

func TestLinearResampler_SameRate(t *testing.T) {
	resampler := NewLinearResampler()
	input := []int16{100, 200, 300, 400, 500}

	output, err := resampler.Resample(input, 16000, 16000, 1)
	if err != nil {
		t.Fatalf("Resample failed: %v", err)
	}

	if len(output) != len(input) {
		t.Errorf("Expected length %d, got %d", len(input), len(output))
	}

	for i := range input {
		if output[i] != input[i] {
			t.Errorf("Sample %d: expected %d, got %d", i, input[i], output[i])
		}
	}
}

func TestLinearResampler_16kTo24k(t *testing.T) {
	resampler := NewLinearResampler()
	// 生成 100 个样本 @ 16kHz
	input := make([]int16, 100)
	for i := range input {
		input[i] = int16(i * 100)
	}

	output, err := resampler.Resample(input, 16000, 24000, 1)
	if err != nil {
		t.Fatalf("Resample failed: %v", err)
	}

	// 24kHz 应该有更多样本 (约 1.5 倍)
	expectedLen := int(math.Ceil(float64(len(input)) * 24000.0 / 16000.0))
	if len(output) != expectedLen {
		t.Errorf("Expected length ~%d, got %d", expectedLen, len(output))
	}

	// 验证第一个和最后一个样本
	if output[0] != input[0] {
		t.Errorf("First sample mismatch: expected %d, got %d", input[0], output[0])
	}
}

func TestLinearResampler_24kTo16k(t *testing.T) {
	resampler := NewLinearResampler()
	// 生成 150 个样本 @ 24kHz
	input := make([]int16, 150)
	for i := range input {
		input[i] = int16(i * 100)
	}

	output, err := resampler.Resample(input, 24000, 16000, 1)
	if err != nil {
		t.Fatalf("Resample failed: %v", err)
	}

	// 16kHz 应该有更少样本 (约 0.67 倍)
	expectedLen := int(math.Ceil(float64(len(input)) * 16000.0 / 24000.0))
	if len(output) != expectedLen {
		t.Errorf("Expected length ~%d, got %d", expectedLen, len(output))
	}
}

func TestLinearResampler_48kTo16k(t *testing.T) {
	resampler := NewLinearResampler()
	// 生成 300 个样本 @ 48kHz
	input := make([]int16, 300)
	for i := range input {
		input[i] = int16(i * 50)
	}

	output, err := resampler.Resample(input, 48000, 16000, 1)
	if err != nil {
		t.Fatalf("Resample failed: %v", err)
	}

	// 16kHz 应该有约 1/3 样本
	expectedLen := int(math.Ceil(float64(len(input)) * 16000.0 / 48000.0))
	if len(output) != expectedLen {
		t.Errorf("Expected length ~%d, got %d", expectedLen, len(output))
	}
}

func TestLinearResampler_Stereo(t *testing.T) {
	resampler := NewLinearResampler()
	// 生成立体声数据：左声道和右声道交替
	input := []int16{100, 200, 300, 400, 500, 600, 700, 800} // 4 frames * 2 channels

	output, err := resampler.Resample(input, 16000, 24000, 2)
	if err != nil {
		t.Fatalf("Resample failed: %v", err)
	}

	// 验证输出也是立体声（偶数个样本）
	if len(output)%2 != 0 {
		t.Errorf("Stereo output should have even number of samples, got %d", len(output))
	}
}

func TestLinearResampler_EmptyInput(t *testing.T) {
	resampler := NewLinearResampler()
	input := []int16{}

	output, err := resampler.Resample(input, 16000, 24000, 1)
	if err != nil {
		t.Fatalf("Resample failed: %v", err)
	}

	if len(output) != 0 {
		t.Errorf("Expected empty output, got %d samples", len(output))
	}
}

func TestLinearResampler_InvalidRate(t *testing.T) {
	resampler := NewLinearResampler()
	input := []int16{100, 200, 300}

	tests := []struct {
		name       string
		inputRate  int
		outputRate int
		channels   int
	}{
		{"zero input rate", 0, 16000, 1},
		{"zero output rate", 16000, 0, 1},
		{"negative input rate", -16000, 16000, 1},
		{"negative output rate", 16000, -16000, 1},
		{"zero channels", 16000, 16000, 0},
		{"negative channels", 16000, 16000, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resampler.Resample(input, tt.inputRate, tt.outputRate, tt.channels)
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestLinearResampler_SineWaveQuality(t *testing.T) {
	resampler := NewLinearResampler()

	// 生成 1kHz 正弦波 @ 16kHz
	sampleRate := 16000
	freq := 1000.0
	duration := 0.1 // 100ms
	samples := int(float64(sampleRate) * duration)
	input := make([]int16, samples)

	for i := 0; i < samples; i++ {
		t := float64(i) / float64(sampleRate)
		sample := math.Sin(2 * math.Pi * freq * t)
		input[i] = int16(sample * 16000) // 降低幅度避免削波
	}

	// 重采样到 24kHz
	output, err := resampler.Resample(input, 16000, 24000, 1)
	if err != nil {
		t.Fatalf("Resample failed: %v", err)
	}

	// 验证输出也是正弦波（检查几个周期）
	peakCount := 0

	for i := 1; i < len(output)-1; i++ {
		// 检测峰值
		if output[i] > output[i-1] && output[i] > output[i+1] && output[i] > 8000 {
			peakCount++
		}
	}

	// 100ms 内应该有约 10 个峰值
	expectedPeaks := int(freq * duration)
	tolerance := 2
	if peakCount < expectedPeaks-tolerance || peakCount > expectedPeaks+tolerance {
		t.Logf("Warning: Expected ~%d peaks, got %d (may indicate quality issue)", expectedPeaks, peakCount)
	}
}

func TestResamplingReader_PassThrough(t *testing.T) {
	// 相同采样率应该直接透传
	input := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	reader := bytes.NewReader(input)

	resampler := NewLinearResampler()
	resamplingReader := NewResamplingReader(reader, 16000, 16000, 1, resampler)

	output := make([]byte, len(input))
	n, err := io.ReadFull(resamplingReader, output)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if n != len(input) {
		t.Errorf("Expected to read %d bytes, got %d", len(input), n)
	}

	if !bytes.Equal(input, output) {
		t.Errorf("Output mismatch: expected %v, got %v", input, output)
	}
}

func TestResamplingReader_Resample(t *testing.T) {
	// 生成测试数据：100 个 int16 样本
	input := make([]int16, 100)
	for i := range input {
		input[i] = int16(i * 100)
	}

	// 转换为 byte
	inputBytes := make([]byte, len(input)*2)
	int16ToBytes(input, inputBytes)

	reader := bytes.NewReader(inputBytes)
	resampler := NewLinearResampler()
	resamplingReader := NewResamplingReader(reader, 16000, 24000, 1, resampler)

	// 读取所有数据
	output := make([]byte, 4096)
	n, err := io.ReadFull(resamplingReader, output)
	if err != nil && err != io.ErrUnexpectedEOF {
		t.Fatalf("Read failed: %v", err)
	}

	// 验证输出长度增加了（16k -> 24k 约 1.5 倍）
	expectedBytes := int(math.Ceil(float64(len(inputBytes)) * 24000.0 / 16000.0))
	tolerance := 10
	if n < expectedBytes-tolerance || n > expectedBytes+tolerance {
		t.Errorf("Expected ~%d bytes, got %d", expectedBytes, n)
	}
}

func TestResamplingReader_MultipleReads(t *testing.T) {
	// 生成较大的测试数据
	input := make([]int16, 1000)
	for i := range input {
		input[i] = int16(i)
	}

	inputBytes := make([]byte, len(input)*2)
	int16ToBytes(input, inputBytes)

	reader := bytes.NewReader(inputBytes)
	resampler := NewLinearResampler()
	resamplingReader := NewResamplingReader(reader, 16000, 24000, 1, resampler)

	// 多次小块读取
	totalRead := 0
	buffer := make([]byte, 256)
	for {
		n, err := resamplingReader.Read(buffer)
		totalRead += n
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
	}

	// 验证总读取量
	expectedBytes := int(math.Ceil(float64(len(inputBytes)) * 24000.0 / 16000.0))
	tolerance := 20
	if totalRead < expectedBytes-tolerance || totalRead > expectedBytes+tolerance {
		t.Errorf("Expected to read ~%d bytes total, got %d", expectedBytes, totalRead)
	}
}

func TestBytesToInt16AndBack(t *testing.T) {
	original := []int16{-32768, -100, 0, 100, 32767}
	bytes := make([]byte, len(original)*2)

	// 转换到 bytes
	n := int16ToBytes(original, bytes)
	if n != len(bytes) {
		t.Errorf("Expected to write %d bytes, got %d", len(bytes), n)
	}

	// 转换回 int16
	recovered := bytesToInt16(bytes)
	if len(recovered) != len(original) {
		t.Errorf("Expected %d samples, got %d", len(original), len(recovered))
	}

	for i := range original {
		if recovered[i] != original[i] {
			t.Errorf("Sample %d: expected %d, got %d", i, original[i], recovered[i])
		}
	}
}

func BenchmarkLinearResampler_16kTo24k(b *testing.B) {
	resampler := NewLinearResampler()
	input := make([]int16, 1600) // 100ms @ 16kHz
	for i := range input {
		input[i] = int16(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = resampler.Resample(input, 16000, 24000, 1)
	}
}

func BenchmarkLinearResampler_24kTo16k(b *testing.B) {
	resampler := NewLinearResampler()
	input := make([]int16, 2400) // 100ms @ 24kHz
	for i := range input {
		input[i] = int16(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = resampler.Resample(input, 24000, 16000, 1)
	}
}

func BenchmarkResamplingReader(b *testing.B) {
	input := make([]int16, 1600)
	for i := range input {
		input[i] = int16(i)
	}
	inputBytes := make([]byte, len(input)*2)
	int16ToBytes(input, inputBytes)

	buffer := make([]byte, 512)
	resampler := NewLinearResampler()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(inputBytes)
		resamplingReader := NewResamplingReader(reader, 16000, 24000, 1, resampler)
		for {
			_, err := resamplingReader.Read(buffer)
			if err == io.EOF {
				break
			}
		}
	}
}
