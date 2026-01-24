package text

import "strings"

type Segmenter struct {
	MaxRunes int
	buffer   []rune
}

func NewSegmenter(maxRunes int) *Segmenter {
	return &Segmenter{MaxRunes: maxRunes}
}

func (s *Segmenter) Feed(text string) []string {
	if text == "" {
		return nil
	}

	outputs := make([]string, 0)
	for _, r := range text {
		s.buffer = append(s.buffer, r)
		if isSentenceBoundary(r) || (s.MaxRunes > 0 && len(s.buffer) >= s.MaxRunes) {
			if sentence := s.flushBuffer(); sentence != "" {
				outputs = append(outputs, sentence)
			}
		}
	}
	return outputs
}

func (s *Segmenter) Flush() string {
	return s.flushBuffer()
}

func (s *Segmenter) flushBuffer() string {
	if len(s.buffer) == 0 {
		return ""
	}
	sentence := strings.TrimSpace(string(s.buffer))
	s.buffer = s.buffer[:0]
	return sentence
}

func isSentenceBoundary(r rune) bool {
	switch r {
	case '\n', '.', '!', '?', ';', '。', '！', '？', '；', '…':
		return true
	default:
		return false
	}
}
