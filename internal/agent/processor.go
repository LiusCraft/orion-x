package agent

import (
	"context"
	"strings"
)

// LLMProcessor LLM流处理器
type LLMProcessor interface {
	ProcessStream(ctx context.Context, text string) (<-chan TextChunkEvent, <-chan error)
}

// EmotionExtractor 情绪提取器
type EmotionExtractor interface {
	Extract(text string) string
}

// EmotionExtractor 情绪提取器实现
type emotionExtractor struct {
	emotionPatterns map[string]string
}

func NewEmotionExtractor() EmotionExtractor {
	return &emotionExtractor{
		emotionPatterns: map[string]string{
			`[EMO:happy]`:   "happy",
			`[EMO:sad]`:     "sad",
			`[EMO:angry]`:   "angry",
			`[EMO:calm]`:    "calm",
			`[EMO:excited]`: "excited",
		},
	}
}

// Extract 从文本中提取情绪标签
func (e *emotionExtractor) Extract(text string) string {
	for pattern, emotion := range e.emotionPatterns {
		if strings.Contains(text, pattern) {
			return emotion
		}
	}
	return "default"
}
