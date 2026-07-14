package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/liuscraft/orion-x/internal/logging"
)

// SearchResultItem is a single result from a knowledge search, returned by the Manager API.
type SearchResultItem struct {
	ChunkID      string  `json:"chunk_id"`
	Content      string  `json:"content"`
	Score        float64 `json:"score"`
	DocumentName string  `json:"document_name"`
}

// SearchClient is an HTTP client that calls the Manager's /internal/knowledge/search endpoint.
// It lives on the wsserver side and mirrors the pattern of memory.CuratedStore's HTTP usage.
type SearchClient struct {
	managerURL string
	deviceID   string
	client     *http.Client
}

// NewSearchClient creates a SearchClient for a given device.
func NewSearchClient(managerURL, deviceID string) *SearchClient {
	return &SearchClient{
		managerURL: strings.TrimRight(managerURL, "/"),
		deviceID:   deviceID,
		client:     &http.Client{Timeout: 15 * time.Second},
	}
}

// Search queries the Manager's knowledge search API and returns matching document chunks.
func (c *SearchClient) Search(ctx context.Context, query string, topK int) ([]SearchResultItem, error) {
	if topK <= 0 || topK > 10 {
		topK = 5
	}
	addr := fmt.Sprintf("%s/internal/knowledge/search?q=%s&device_id=%s&top_k=%d",
		c.managerURL, url.QueryEscape(query), c.deviceID, topK)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, nil)
	if err != nil {
		return nil, fmt.Errorf("search_client: create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		logging.Warnf("SearchClient: HTTP error: %v", err)
		return nil, fmt.Errorf("search_client: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search_client: status %d", resp.StatusCode)
	}

	var items []SearchResultItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("search_client: decode: %w", err)
	}
	return items, nil
}
