package embedder

import (
	"context"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type openaiEmbedder struct {
	client     openai.Client
	model      string
	dimensions int
}

func init() {
	Register("openai", NewOpenAI)
	Register("", NewOpenAI)
}

// NewOpenAI creates an OpenAI-compatible embedder using the official Go SDK.
func NewOpenAI(cfg Config) (Embedder, error) {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("embedder: api key is required for %s", baseURL)
	}
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if baseURL != "https://api.openai.com/v1" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	model := cfg.Model
	if model == "" {
		model = "text-embedding-3-small"
	}
	dims := 1536
	if strings.Contains(model, "large") {
		dims = 3072
	}

	return &openaiEmbedder{
		client:     openai.NewClient(opts...),
		model:      model,
		dimensions: dims,
	}, nil
}

func (e *openaiEmbedder) Dimensions() int { return e.dimensions }

func (e *openaiEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	params := openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: texts},
		Model: e.model,
	}
	resp, err := e.client.Embeddings.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}

	result := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		vec := make([]float32, len(d.Embedding))
		for j, v := range d.Embedding {
			vec[j] = float32(v)
		}
		result[i] = vec
	}
	return result, nil
}
