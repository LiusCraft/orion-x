package parser

import (
	"context"
	"io"
	"strings"
)

// TextParser parses plain-text based files (txt, md, code files, etc.).
type TextParser struct{}

// NewTextParser creates a TextParser that handles common text-based formats.
func NewTextParser() *TextParser { return &TextParser{} }

// SupportedExtensions returns the file extensions this parser handles.
func (p *TextParser) SupportedExtensions() []string {
	return []string{
		".txt", ".md", ".markdown",
		".go", ".py", ".ts", ".js", ".tsx", ".jsx",
		".yaml", ".yml", ".json", ".xml", ".html", ".htm",
		".css", ".scss", ".less",
		".sh", ".bash", ".zsh",
		".sql", ".graphql",
		".env", ".ini", ".cfg", ".conf", ".toml",
		".log", ".csv",
		".c", ".cpp", ".h", ".hpp", ".rs", ".java", ".kt", ".swift",
		".rb", ".php", ".lua", ".r", ".m", ".mm",
	}
}

// Parse reads the entire reader and returns trimmed text.
func (p *TextParser) Parse(_ context.Context, reader io.Reader, _ string) (string, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
