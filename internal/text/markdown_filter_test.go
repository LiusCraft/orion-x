package text

import "testing"

func TestFilterMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text",
			input:    "Hello world",
			expected: "Hello world",
		},
		{
			name:     "bold asterisk",
			input:    "**bold** text",
			expected: "bold text",
		},
		{
			name:     "bold underscore",
			input:    "__bold__ text",
			expected: "bold text",
		},
		{
			name:     "italic asterisk",
			input:    "*italic* text",
			expected: "italic text",
		},
		{
			name:     "italic underscore",
			input:    "_italic_ text",
			expected: "italic text",
		},
		{
			name:     "strikethrough",
			input:    "~~deleted~~ text",
			expected: "deleted text",
		},
		{
			name:     "inline code",
			input:    "`code` here",
			expected: "code here",
		},
		{
			name:     "code block",
			input:    "before ```code block``` after",
			expected: "before  after",
		},
		{
			name:     "atx header",
			input:    "# Heading 1\n## Heading 2",
			expected: "Heading 1\nHeading 2",
		},
		{
			name:     "setext header",
			input:    "Heading\n===",
			expected: "Heading",
		},
		{
			name:     "link",
			input:    "[link text](https://example.com)",
			expected: "link text",
		},
		{
			name:     "image with alt",
			input:    "![alt text](image.png)",
			expected: "alt text",
		},
		{
			name:     "image without alt",
			input:    "![](image.png)",
			expected: "",
		},
		{
			name:     "blockquote",
			input:    "> quoted text",
			expected: "quoted text",
		},
		{
			name:     "list leader asterisk",
			input:    "* item one\n* item two",
			expected: "item one\nitem two",
		},
		{
			name:     "list leader dash",
			input:    "- item one\n- item two",
			expected: "item one\nitem two",
		},
		{
			name:     "list leader plus",
			input:    "+ item one\n+ item two",
			expected: "item one\nitem two",
		},
		{
			name:     "numbered list",
			input:    "1. first\n2. second",
			expected: "first\nsecond",
		},
		{
			name:     "horizontal rule",
			input:    "before\n---\nafter",
			expected: "before\n\nafter",
		},
		{
			name:     "html tag",
			input:    "text <span>with</span> tags",
			expected: "text with tags",
		},
		{
			name:     "complex example",
			input:    "# Hello **world**\n\nThis is [a link](https://example.com) and `code`.",
			expected: "Hello world\n\nThis is a link and code.",
		},
		{
			name:     "multiple newlines cleaned",
			input:    "text\n\n\n\nmore",
			expected: "text\n\nmore",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterMarkdown(tt.input)
			if got != tt.expected {
				t.Errorf("FilterMarkdown() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFilterMarkdownWithOptions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		opts     MarkdownOptions
		expected string
	}{
		{
			name:     "skip images",
			input:    "text ![alt](img.png) more",
			opts:     MarkdownOptions{SkipImages: true},
			expected: "text  more",
		},
		{
			name:     "keep list leaders",
			input:    "* item",
			opts:     MarkdownOptions{StripListLeaders: false},
			expected: "item",
		},
		{
			name:     "nested emphasis",
			input:    "***bold and italic*** text",
			expected: "bold and italic text",
		},
		{
			name:     "mixed formatting",
			input:    "**bold** and *italic* and `code`",
			expected: "bold and italic and code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterMarkdownWithOptions(tt.input, tt.opts)
			if got != tt.expected {
				t.Errorf("FilterMarkdownWithOptions() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func BenchmarkFilterMarkdown(b *testing.B) {
	input := "# Heading\n\nThis is **bold** and *italic* text with [a link](https://example.com) and `code`."
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FilterMarkdown(input)
	}
}

func ExampleFilterMarkdown() {
	input := "# Welcome\n\nThis is **bold** and [a link](https://example.com)."
	output := FilterMarkdown(input)
	_ = output
}
