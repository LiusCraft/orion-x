// Package parser defines the document parser interface and registry.
// Parsers convert raw document bytes (PDF, Markdown, TXT, etc.) into plain text.
package parser

import (
	"context"
	"io"
	"sort"
	"strings"
)

// Parser converts a document reader into clean plain text.
type Parser interface {
	Parse(ctx context.Context, reader io.Reader, filename string) (string, error)
	SupportedExtensions() []string
}

// Registry holds parsers indexed by file extension.
type Registry struct {
	byExt map[string]Parser
}

// NewRegistry creates an empty parser registry.
func NewRegistry() *Registry {
	return &Registry{byExt: make(map[string]Parser)}
}

// Register registers a parser for all its supported extensions.
func (r *Registry) Register(p Parser) {
	for _, ext := range p.SupportedExtensions() {
		r.byExt[strings.ToLower(ext)] = p
	}
}

// Find returns the parser for the given filename, or false if none matches.
func (r *Registry) Find(filename string) (Parser, bool) {
	idx := strings.LastIndex(filename, ".")
	if idx < 0 {
		return nil, false
	}
	ext := strings.ToLower(filename[idx:])
	p, ok := r.byExt[ext]
	return p, ok
}

// SupportedExtensions returns every registered extension, sorted.
// Callers use it to align upload validation with what can actually be parsed.
func (r *Registry) SupportedExtensions() []string {
	exts := make([]string, 0, len(r.byExt))
	for ext := range r.byExt {
		exts = append(exts, ext)
	}
	sort.Strings(exts)
	return exts
}

// DefaultRegistry returns a Registry pre-loaded with built-in parsers.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(NewTextParser())
	r.Register(NewURLParser())
	return r
}
