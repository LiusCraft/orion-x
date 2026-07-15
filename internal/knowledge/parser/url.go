package parser

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// URLParser registers as the ".url" virtual extension for URL-based content.
// Actual URL fetching is done by the service layer; this parser exists for
// the registry to treat URL sources as a document source type.
type URLParser struct {
	client *http.Client
}

// NewURLParser creates a URLParser.
func NewURLParser() *URLParser {
	return &URLParser{client: &http.Client{Timeout: 30 * time.Second}}
}

// SupportedExtensions returns [".url"] — a virtual extension matched by source="url" documents.
func (p *URLParser) SupportedExtensions() []string {
	return []string{".url"}
}

// Parse is a no-op for URLParser; the service layer fetches URLs directly.
func (p *URLParser) Parse(_ context.Context, _ io.Reader, _ string) (string, error) {
	return "", fmt.Errorf("URLParser: use Service.IngestURL instead")
}
