package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type openaiEmbedder struct {
	apiKey     string
	baseURL    string
	model      string
	dimensions int
	client     *http.Client
}

func init() {
	// Register as default ("") and "openai"
	Register("openai", NewOpenAI)
	Register("", NewOpenAI)
}

// NewOpenAI creates an OpenAI-compatible embedder.
// BaseURL defaults to https://api.openai.com/v1, model to text-embedding-3-small.
func NewOpenAI(cfg Config) (Embedder, error) {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := cfg.Model
	if model == "" {
		model = "text-embedding-3-small"
	}
	dims := 1536
	switch {
	case strings.Contains(model, "large"):
		dims = 3072
	case model == "text-embedding-ada-002":
		dims = 1536
	}
	return &openaiEmbedder{
		apiKey:     cfg.APIKey,
		baseURL:    baseURL,
		model:      model,
		dimensions: dims,
		client:     &http.Client{Timeout: 60 * time.Second},
	}, nil
}

func (e *openaiEmbedder) Dimensions() int { return e.dimensions }

func (e *openaiEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	type reqBody struct {
		Input []string `json:"input"`
		Model string   `json:"model"`
	}
	type embedData struct {
		Embedding []float32 `json:"embedding"`
	}
	type respData struct {
		Data []embedData `json:"data"`
	}

	body, _ := json.Marshal(reqBody{Input: texts, Model: e.model})
	url := fmt.Sprintf("%s/embeddings", e.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embed: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("embed: status %d: %s", resp.StatusCode, string(msg))
	}

	var data respData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("embed: decode: %w", err)
	}

	result := make([][]float32, len(data.Data))
	for i, d := range data.Data {
		result[i] = d.Embedding
	}
	return result, nil
}
