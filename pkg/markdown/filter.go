// Package markdown provides utilities for stripping Markdown formatting from text.
package markdown

import (
	"regexp"
)

// Filter removes Markdown formatting from text, returning plain text.
// Where necessary, elements are replaced with their best textual forms:
//   - Links become only the link text
//   - Images become only the alt text
//   - Headers are stripped of # symbols
//   - Bold/italic markers are removed
func Filter(text string) string {
	return filterWithOptions(text, Options{})
}

// Options configures the filtering behavior.
type Options struct {
	// SkipImages removes images entirely instead of keeping alt text.
	SkipImages bool
	// StripListLeaders removes list markers (*, -, +, 1.).
	StripListLeaders bool
	// KeepLinks keeps the full link format [text](url) instead of just text.
	KeepLinks bool
}

// FilterWithOptions removes Markdown formatting with custom options.
func FilterWithOptions(text string, opts Options) string {
	return filterWithOptions(text, opts)
}

// Pre-compiled regex patterns for better performance.
// Patterns are compiled once at package initialization.
var (
	patterns struct {
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
)

func init() {
	// Initialize all regex patterns at package load time.
	// Using MustCompile since patterns are constants and valid.
	// (?m) enables multiline mode where ^ and $ match line beginnings/ends.
	// For emphasis patterns, we use [^\n*] etc to prevent matching across newlines.
	patterns.codeBlock = regexp.MustCompile("```[\\s\\S]*?```")
	patterns.inlineCode = regexp.MustCompile("`([^`\n]+)`")
	patterns.boldAsterisk = regexp.MustCompile(`\*\*([^\n*]+)\*\*`)
	patterns.boldUnderscore = regexp.MustCompile(`__([^\n_]+)__`)
	patterns.italicAsterisk = regexp.MustCompile(`\*([^\n*]+)\*`)
	patterns.italicUnderscore = regexp.MustCompile(`_([^\n_]+)_`)
	patterns.strikeThrough = regexp.MustCompile(`~~([^\n~]+)~~`)
	patterns.headerAtx = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	patterns.headerSetext = regexp.MustCompile(`\n={3,}\s*$`)
	patterns.link = regexp.MustCompile(`\[([^\]]+)\]\([^\)]+\)`)
	patterns.image = regexp.MustCompile(`!\[([^\]]*)\]\([^\)]+\)`)
	patterns.html = regexp.MustCompile(`<[^>]+>`)
	patterns.blockquote = regexp.MustCompile(`(?m)^\s*>\s*`)
	patterns.listLeader = regexp.MustCompile(`(?m)^\s*([*\-+]|\d+\.)\s+`)
	patterns.hr = regexp.MustCompile(`(?m)^\s*([-*_]{3,})\s*$`)
	patterns.footnote = regexp.MustCompile(`\[\^.+?\](?::\s*.+?$)?`)
	patterns.refLink = regexp.MustCompile(`(?m)^\s{0,2}\[.+?\]:\s*\S+.*?$`)
	patterns.multipleNewlines = regexp.MustCompile(`\n{3,}`)
}

// filterWithOptions performs the actual filtering with given options.
func filterWithOptions(text string, opts Options) string {
	result := text

	// Remove code blocks first (multiline)
	result = patterns.codeBlock.ReplaceAllString(result, "")

	// Process headers
	result = patterns.headerAtx.ReplaceAllString(result, "$1")
	result = patterns.headerSetext.ReplaceAllString(result, "")

	// Process emphasis (bold/italic)
	// Process bold before italic to avoid conflicts
	result = patterns.boldAsterisk.ReplaceAllString(result, "$1")
	result = patterns.boldUnderscore.ReplaceAllString(result, "$1")
	result = patterns.strikeThrough.ReplaceAllString(result, "$1")
	result = patterns.italicAsterisk.ReplaceAllString(result, "$1")
	result = patterns.italicUnderscore.ReplaceAllString(result, "$1")

	// Process inline code after emphasis (to avoid processing * in code)
	result = patterns.inlineCode.ReplaceAllString(result, "$1")

	// Process images
	if opts.SkipImages {
		result = patterns.image.ReplaceAllString(result, "")
	} else {
		result = patterns.image.ReplaceAllString(result, "$1")
	}

	// Process links
	if opts.KeepLinks {
		// Keep full link format
		result = patterns.link.ReplaceAllString(result, "$1")
	} else {
		// Keep only link text
		result = patterns.link.ReplaceAllString(result, "$1")
	}

	// Remove HTML tags
	result = patterns.html.ReplaceAllString(result, "")

	// Process blockquotes
	result = patterns.blockquote.ReplaceAllString(result, "")

	// Process list leaders (remove the marker, keep content)
	result = patterns.listLeader.ReplaceAllString(result, "")

	// Remove horizontal rules
	result = patterns.hr.ReplaceAllString(result, "")

	// Remove footnotes
	result = patterns.footnote.ReplaceAllString(result, "")

	// Remove reference links
	result = patterns.refLink.ReplaceAllString(result, "")

	// Clean up excessive newlines
	result = patterns.multipleNewlines.ReplaceAllString(result, "\n\n")

	return result
}
