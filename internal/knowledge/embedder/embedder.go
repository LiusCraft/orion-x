// Package embedder defines the text embedding interface and factory.
// Embedders convert text into dense vector representations.
package embedder

import (
	"context"
	"fmt"
	"strings"
)

// Embedder converts text batches into float32 vectors.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}

// Config configures an embedder instance.
type Config struct {
	Type    string // "openai"
	APIKey  string
	BaseURL string
	Model   string
}

// Factory creates an Embedder from a Config.
type Factory func(Config) (Embedder, error)

var factories = map[string]Factory{}

// Register registers an embedder factory under the given type name.
// Passing an empty type registers it as the default.
func Register(etype string, f Factory) {
	factories[strings.ToLower(strings.TrimSpace(etype))] = f
}

// New creates an Embedder using the factory registered for cfg.Type.
func New(cfg Config) (Embedder, error) {
	t := strings.ToLower(strings.TrimSpace(cfg.Type))
	f, ok := factories[t]
	if !ok {
		return nil, fmt.Errorf("unknown embedder type: %q", t)
	}
	return f(cfg)
}
