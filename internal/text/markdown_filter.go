package text

import "regexp"

// FilterMarkdown removes Markdown formatting from text, returning plain text.
// Where necessary, elements are replaced with their best textual forms:
//   - Links become only the link text
//   - Images become only the alt text
//   - Headers are stripped of # symbols
//   - Bold/italic markers are removed
func FilterMarkdown(text string) string {
	return filterMarkdownWithOptions(text, MarkdownOptions{})
}

// MarkdownOptions configures Markdown filtering behavior.
type MarkdownOptions struct {
	// SkipImages removes images entirely instead of keeping alt text.
	SkipImages bool
	// StripListLeaders removes list markers (*, -, +, 1.).
	StripListLeaders bool
	// KeepLinks keeps the full link format [text](url) instead of just text.
	KeepLinks bool
}

// FilterMarkdownWithOptions removes Markdown formatting with custom options.
func FilterMarkdownWithOptions(text string, opts MarkdownOptions) string {
	return filterMarkdownWithOptions(text, opts)
}

var markdownPatterns struct {
	codeBlock        *regexp.Regexp // ```code```
	inlineCode       *regexp.Regexp // `code`
	boldAsterisk     *regexp.Regexp // **text**
	boldUnderscore   *regexp.Regexp // __text__
	italicAsterisk   *regexp.Regexp // *text*
	italicUnderscore *regexp.Regexp // _text_
	strikeThrough    *regexp.Regexp // ~~text~~
	headerAtx        *regexp.Regexp // # Heading
	headerSetext     *regexp.Regexp // Heading\n===
	link             *regexp.Regexp // [text](url)
	image            *regexp.Regexp // ![alt](url)
	html             *regexp.Regexp // <tag>
	blockquote       *regexp.Regexp // > quote
	listLeader       *regexp.Regexp // * item
	hr               *regexp.Regexp // --- or ***
	footnote         *regexp.Regexp // [^1]
	refLink          *regexp.Regexp // [1]: url
	multipleNewlines *regexp.Regexp // 3+ newlines
}

func init() {
	markdownPatterns.codeBlock = regexp.MustCompile("```[\\s\\S]*?```")
	markdownPatterns.inlineCode = regexp.MustCompile("`([^`\n]+)`")
	markdownPatterns.boldAsterisk = regexp.MustCompile(`\*\*([^\n*]+)\*\*`)
	markdownPatterns.boldUnderscore = regexp.MustCompile(`__([^\n_]+)__`)
	markdownPatterns.italicAsterisk = regexp.MustCompile(`\*([^\n*]+)\*`)
	markdownPatterns.italicUnderscore = regexp.MustCompile(`_([^\n_]+)_`)
	markdownPatterns.strikeThrough = regexp.MustCompile(`~~([^\n~]+)~~`)
	markdownPatterns.headerAtx = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	markdownPatterns.headerSetext = regexp.MustCompile(`\n={3,}\s*$`)
	markdownPatterns.link = regexp.MustCompile(`\[([^\]]+)\]\([^\)]+\)`)
	markdownPatterns.image = regexp.MustCompile(`!\[([^\]]*)\]\([^\)]+\)`)
	markdownPatterns.html = regexp.MustCompile(`<[^>]+>`)
	markdownPatterns.blockquote = regexp.MustCompile(`(?m)^\s*>\s*`)
	markdownPatterns.listLeader = regexp.MustCompile(`(?m)^\s*([*\-+]|\d+\.)\s+`)
	markdownPatterns.hr = regexp.MustCompile(`(?m)^\s*([-*_]{3,})\s*$`)
	markdownPatterns.footnote = regexp.MustCompile(`\[\^.+?\](?::\s*.+?$)?`)
	markdownPatterns.refLink = regexp.MustCompile(`(?m)^\s{0,2}\[.+?\]:\s*\S+.*?$`)
	markdownPatterns.multipleNewlines = regexp.MustCompile(`\n{3,}`)
}

func filterMarkdownWithOptions(text string, opts MarkdownOptions) string {
	result := text

	result = markdownPatterns.codeBlock.ReplaceAllString(result, "")
	result = markdownPatterns.headerAtx.ReplaceAllString(result, "$1")
	result = markdownPatterns.headerSetext.ReplaceAllString(result, "")
	result = markdownPatterns.boldAsterisk.ReplaceAllString(result, "$1")
	result = markdownPatterns.boldUnderscore.ReplaceAllString(result, "$1")
	result = markdownPatterns.strikeThrough.ReplaceAllString(result, "$1")
	result = markdownPatterns.italicAsterisk.ReplaceAllString(result, "$1")
	result = markdownPatterns.italicUnderscore.ReplaceAllString(result, "$1")
	result = markdownPatterns.inlineCode.ReplaceAllString(result, "$1")

	if opts.SkipImages {
		result = markdownPatterns.image.ReplaceAllString(result, "")
	} else {
		result = markdownPatterns.image.ReplaceAllString(result, "$1")
	}

	result = markdownPatterns.link.ReplaceAllString(result, "$1")
	result = markdownPatterns.html.ReplaceAllString(result, "")
	result = markdownPatterns.blockquote.ReplaceAllString(result, "")
	result = markdownPatterns.listLeader.ReplaceAllString(result, "")
	result = markdownPatterns.hr.ReplaceAllString(result, "")
	result = markdownPatterns.footnote.ReplaceAllString(result, "")
	result = markdownPatterns.refLink.ReplaceAllString(result, "")
	result = markdownPatterns.multipleNewlines.ReplaceAllString(result, "\n\n")

	return result
}
