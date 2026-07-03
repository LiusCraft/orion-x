package text

// ExtractLeadingEmoji 返回文本开头的 emoji（含 skin-tone 修饰符）和剩余文本。
// 若首字符不是 emoji 则返回 ("", original)。
func ExtractLeadingEmoji(s string) (emoji, remaining string) {
	if s == "" {
		return "", ""
	}
	runes := []rune(s)
	if !isEmoji(runes[0]) {
		return "", s
	}
	end := 1
	// 接受 skin-tone modifier (U+1F3FB..U+1F3FF)
	if len(runes) > 1 && runes[1] >= 0x1F3FB && runes[1] <= 0x1F3FF {
		end = 2
	}
	return string(runes[:end]), string(runes[end:])
}

func isEmoji(r rune) bool {
	switch {
	case r >= 0x1F600 && r <= 0x1F64F, // Emoticons
		r >= 0x1F300 && r <= 0x1F5FF, // Misc Symbols and Pictographs
		r >= 0x1F680 && r <= 0x1F6FF, // Transport and Map
		r >= 0x1F900 && r <= 0x1F9FF, // Supplemental Symbols
		r >= 0x2600 && r <= 0x26FF,   // Misc symbols
		r >= 0x2700 && r <= 0x27BF,   // Dingbats
		r >= 0x1F1E0 && r <= 0x1F1FF: // Flags (regional indicators)
		return true
	}
	return false
}
