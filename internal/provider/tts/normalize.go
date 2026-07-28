package tts

import "math"

// SpeedToRate 将归一化语速倍率（0.5~2.0）映射到 provider 实际 scale。
// 当前阿里云/火山/MiniMax 都使用倍率，直接返回。
func SpeedToRate(speed float64) float64 {
	if speed <= 0 {
		return 0
	}
	return clamp(speed, 0.5, 2.0)
}

// PitchToRatio 将归一化音调倍率（0.5~2.0）映射为 provider 倍率。
// 阿里云/火山直接使用此倍率。
func PitchToRatio(pitch float64) float64 {
	if pitch <= 0 {
		return 0
	}
	return clamp(pitch, 0.5, 2.0)
}

// PitchToSemitone 将归一化音调倍率（0.5~2.0）映射为半音偏移量。
// 用于 Google/MiniMax 等使用半音的 provider。
func PitchToSemitone(pitch float64) float64 {
	if pitch <= 0 {
		return 0
	}
	ratio := clamp(pitch, 0.5, 2.0)
	return 12 * math.Log2(ratio)
}

// VolumeToPercent 将归一化音量（0.0~1.0）映射为百分比（0~100）。
// 用于阿里云等使用百分比音量的 provider。
func VolumeToPercent(volume float64) int {
	if volume <= 0 {
		return 0
	}
	v := clamp(volume, 0.0, 1.0)
	return int(v * 100)
}

// VolumeToMinimax 将归一化音量（0.0~1.0）映射为 MiniMax vol（0.1~10.0）。
func VolumeToMinimax(volume float64) float64 {
	if volume <= 0 {
		return 0
	}
	v := clamp(volume, 0.0, 1.0)
	return 0.1 + v*9.9
}

// VolumeToGainDb 将归一化音量（0.0~1.0）映射为 Google gainDb（-96~+16）。
func VolumeToGainDb(volume float64) float64 {
	if volume <= 0 {
		return 0
	}
	v := clamp(volume, 0.0, 1.0)
	return -96 + v*112
}

// EmotionMap 定义公共情绪 → provider 情绪值的映射表。
// 各 adapter 按需查找，不支持静默忽略。
var EmotionMap = map[string]map[string]string{
	// 通用映射
	"": {
		"happy":   "happy",
		"sad":     "sad",
		"angry":   "angry",
		"calm":    "calm",
		"excited": "excited",
		"fearful": "fearful",
		"neutral": "neutral",
	},
	// MiniMax 特有映射
	"minimax": {
		"happy":   "happy",
		"sad":     "sad",
		"angry":   "angry",
		"calm":    "calm",
		"excited": "surprised",
		"fearful": "fearful",
		"neutral": "fluent",
	},
	// Azure 特有映射（SSML style）
	"azure": {
		"happy":   "cheerful",
		"sad":     "sad",
		"angry":   "angry",
		"calm":    "calm",
		"excited": "excited",
		"fearful": "fearful",
	},
}

// MapEmotion 将公共情绪值映射到指定 provider 的情绪值。
// 返回空字符串表示不支持该情绪或未知 provider。
func MapEmotion(provider, emotion string) string {
	if emotion == "" {
		return ""
	}
	m, ok := EmotionMap[provider]
	if !ok {
		m = EmotionMap[""]
	}
	if v, ok := m[emotion]; ok {
		return v
	}
	return ""
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
