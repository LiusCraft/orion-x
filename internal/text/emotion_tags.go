package text

import (
	"regexp"
	"strings"
)

var emotionTagPattern = regexp.MustCompile(`\[EMO:(happy|sad|angry|calm|excited)\]`)

// ExtractEmotionTag returns the first recognized inline emotion marker.
func ExtractEmotionTag(text string) string {
	matches := emotionTagPattern.FindStringSubmatch(text)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

// RemoveEmotionTags removes inline emotion markers emitted by the LLM.
func RemoveEmotionTags(text string) string {
	return strings.TrimSpace(emotionTagPattern.ReplaceAllString(text, ""))
}
