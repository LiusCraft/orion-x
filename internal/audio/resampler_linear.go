package audio

import (
	"fmt"
	"math"
)

// LinearResampler 线性插值重采样器
// 使用线性插值算法进行采样率转换
// 优点：简单、快速、无依赖
// 缺点：音质一般，高频可能失真
// 适用场景：实时语音处理，对音质要求不高的场景
type LinearResampler struct{}

// NewLinearResampler 创建线性插值重采样器
func NewLinearResampler() *LinearResampler {
	return &LinearResampler{}
}

// Resample 使用线性插值进行重采样
// 算法：
//
//	ratio = inputRate / outputRate
//	position = outputIndex * ratio
//	i = floor(position)
//	frac = position - i
//	output[outputIndex] = input[i] * (1 - frac) + input[i+1] * frac
func (r *LinearResampler) Resample(input []int16, inputRate, outputRate, channels int) ([]int16, error) {
	if inputRate <= 0 || outputRate <= 0 {
		return nil, fmt.Errorf("invalid sample rate: input=%d, output=%d", inputRate, outputRate)
	}
	if channels <= 0 {
		return nil, fmt.Errorf("invalid channels: %d", channels)
	}
	if len(input) == 0 {
		return []int16{}, nil
	}

	// 如果采样率相同，直接返回副本
	if inputRate == outputRate {
		result := make([]int16, len(input))
		copy(result, input)
		return result, nil
	}

	// 计算输入和输出的帧数（一帧包含所有声道的样本）
	inputFrames := len(input) / channels
	if inputFrames == 0 {
		return []int16{}, nil
	}

	// 计算输出帧数
	ratio := float64(inputRate) / float64(outputRate)
	outputFrames := int(math.Ceil(float64(inputFrames) / ratio))
	output := make([]int16, outputFrames*channels)

	// 对每个输出帧进行插值
	for outFrame := 0; outFrame < outputFrames; outFrame++ {
		// 计算对应的输入位置
		position := float64(outFrame) * ratio
		inFrame := int(position)
		frac := position - float64(inFrame)

		// 边界检查：确保不超出输入范围
		if inFrame >= inputFrames-1 {
			inFrame = inputFrames - 2
			if inFrame < 0 {
				inFrame = 0
			}
			frac = 1.0
		}

		// 对每个声道进行插值
		for ch := 0; ch < channels; ch++ {
			inIdx1 := inFrame*channels + ch
			inIdx2 := (inFrame+1)*channels + ch
			outIdx := outFrame*channels + ch

			// 边界检查
			if inIdx2 >= len(input) {
				inIdx2 = inIdx1
			}

			// 线性插值
			sample1 := float64(input[inIdx1])
			sample2 := float64(input[inIdx2])
			interpolated := sample1*(1.0-frac) + sample2*frac

			// 裁剪到 int16 范围
			if interpolated > 32767 {
				interpolated = 32767
			} else if interpolated < -32768 {
				interpolated = -32768
			}

			output[outIdx] = int16(interpolated)
		}
	}

	return output, nil
}
