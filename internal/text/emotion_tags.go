package text

import (
	"regexp"
	"strings"
)

var emotionTagPattern = regexp.MustCompile(`\[EMO:(happy|sad|angry|calm|excited)\]`)

// RemoveEmotionTags removes inline emotion markers emitted by the LLM.
func RemoveEmotionTags(text string) string {
	return strings.TrimSpace(emotionTagPattern.ReplaceAllString(text, ""))
}
