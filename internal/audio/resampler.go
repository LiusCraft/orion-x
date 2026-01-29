package audio

import "io"

// Resampler 音频重采样器接口
// 用于在不同采样率之间转换音频数据
type Resampler interface {
	// Resample 重采样音频数据
	// input: 输入 PCM 数据 (int16 格式)
	// inputRate: 输入采样率 (Hz)
	// outputRate: 输出采样率 (Hz)
	// channels: 声道数 (1=mono, 2=stereo)
	// 返回重采样后的 PCM 数据
	Resample(input []int16, inputRate, outputRate, channels int) ([]int16, error)
}

// ResamplingReader 包装 io.Reader，自动进行重采样
// 从 source 读取原始采样率的 PCM 数据，输出目标采样率的数据
type ResamplingReader struct {
	source     io.Reader
	resampler  Resampler
	inputRate  int
	outputRate int
	channels   int

	// 内部缓冲区
	inputBuffer  []byte  // 从 source 读取的原始数据
	sampleBuffer []int16 // 待重采样的样本缓冲
	outputBuffer []int16 // 重采样后的样本缓冲
	outputPos    int     // 输出缓冲区当前位置
}

// NewResamplingReader 创建重采样 Reader
// 如果 inputRate == outputRate，则不进行重采样，直接透传
func NewResamplingReader(source io.Reader, inputRate, outputRate, channels int, resampler Resampler) *ResamplingReader {
	if resampler == nil {
		resampler = NewLinearResampler()
	}

	return &ResamplingReader{
		source:       source,
		resampler:    resampler,
		inputRate:    inputRate,
		outputRate:   outputRate,
		channels:     channels,
		inputBuffer:  make([]byte, 4096),
		sampleBuffer: make([]int16, 0, 2048),
		outputBuffer: make([]int16, 0, 4096),
		outputPos:    0,
	}
}

// Read 实现 io.Reader 接口
// 从 source 读取数据，重采样后写入 p
func (r *ResamplingReader) Read(p []byte) (n int, err error) {
	// 如果采样率相同，直接透传
	if r.inputRate == r.outputRate {
		return r.source.Read(p)
	}

	// 如果输出缓冲区还有数据，先返回
	if r.outputPos < len(r.outputBuffer) {
		n = r.copyOutputToBytes(p)
		if n > 0 {
			return n, nil
		}
	}

	// 读取新数据并重采样
	for {
		// 从 source 读取原始数据
		nr, err := r.source.Read(r.inputBuffer)
		if nr > 0 {
			// 转换 byte 到 int16
			samples := bytesToInt16(r.inputBuffer[:nr])
			r.sampleBuffer = append(r.sampleBuffer, samples...)

			// 执行重采样
			resampled, resampleErr := r.resampler.Resample(
				r.sampleBuffer,
				r.inputRate,
				r.outputRate,
				r.channels,
			)
			if resampleErr != nil {
				return 0, resampleErr
			}

			// 更新输出缓冲区
			r.outputBuffer = resampled
			r.outputPos = 0
			r.sampleBuffer = r.sampleBuffer[:0] // 清空输入缓冲

			// 复制到输出
			n = r.copyOutputToBytes(p)
			if n > 0 {
				return n, nil
			}
		}

		if err != nil {
			// 如果还有剩余数据，返回数据而不是错误
			if len(r.outputBuffer) > r.outputPos {
				n = r.copyOutputToBytes(p)
				if n > 0 {
					return n, nil
				}
			}
			return 0, err
		}
	}
}

// copyOutputToBytes 将输出缓冲区的 int16 数据复制到 byte 数组
func (r *ResamplingReader) copyOutputToBytes(p []byte) int {
	available := len(r.outputBuffer) - r.outputPos
	if available <= 0 {
		return 0
	}

	// 计算可以复制多少样本（每个样本 2 字节）
	maxSamples := len(p) / 2
	if maxSamples > available {
		maxSamples = available
	}

	// 转换 int16 到 byte
	n := int16ToBytes(r.outputBuffer[r.outputPos:r.outputPos+maxSamples], p)
	r.outputPos += maxSamples

	return n
}

// Close 关闭底层 Reader（如果支持）
func (r *ResamplingReader) Close() error {
	if closer, ok := r.source.(io.ReadCloser); ok {
		return closer.Close()
	}
	return nil
}

// bytesToInt16 将 byte 数组转换为 int16 数组 (Little Endian)
func bytesToInt16(data []byte) []int16 {
	samples := make([]int16, len(data)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(data[i*2]) | int16(data[i*2+1])<<8
	}
	return samples
}

// int16ToBytes 将 int16 数组转换为 byte 数组 (Little Endian)
func int16ToBytes(samples []int16, data []byte) int {
	n := 0
	for i := 0; i < len(samples) && n+1 < len(data); i++ {
		data[n] = byte(samples[i])
		data[n+1] = byte(samples[i] >> 8)
		n += 2
	}
	return n
}
