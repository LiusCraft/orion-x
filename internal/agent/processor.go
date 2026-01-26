package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/liuscraft/orion-x/pkg/markdown"
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

// MarkdownFilter Markdown过滤器
type MarkdownFilter interface {
	Filter(text string) string
	RemoveEmotionTags(text string) string
}

// markdownFilter Markdown过滤器实现（使用 pkg/markdown）
type markdownFilter struct{}

func NewMarkdownFilter() MarkdownFilter {
	return &markdownFilter{}
}

// Filter 过滤Markdown标记
func (f *markdownFilter) Filter(text string) string {
	// 使用 pkg/markdown 进行过滤
	result := markdown.Filter(text)
	// 移除情绪标签
	return removeEmotionTags(result)
}

// RemoveEmotionTags 移除情绪标签
func (f *markdownFilter) RemoveEmotionTags(text string) string {
	return removeEmotionTags(text)
}

func removeEmotionTags(text string) string {
	// 移除 [EMO:xxx] 标签
	for _, emotion := range []string{"happy", "sad", "angry", "calm", "excited"} {
		text = replaceAll(text, fmt.Sprintf("[EMO:%s]", emotion), "")
	}
	return strings.TrimSpace(text)
}

func replaceAll(s, old, new string) string {
	result := ""
	i := 0
	for {
		idx := index(s[i:], old)
		if idx == -1 {
			result += s[i:]
			break
		}
		result += s[i:i+idx] + new
		i += idx + len(old)
	}
	return result
}

func index(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
