package agent

func deltaFromBufferedContent(content string, lastLength int) (string, int) {
	if lastLength < 0 {
		lastLength = 0
	}
	if lastLength > len(content) {
		lastLength = len(content)
	}
	return content[lastLength:], len(content)
}
