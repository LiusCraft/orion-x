package toolmarket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/toolvalidator"
)

type fakeConfigValidator struct {
	validate func(ctx context.Context, protocol contracts.ToolProtocol, raw json.RawMessage) (json.RawMessage, error)
}

func (v fakeConfigValidator) Validate(ctx context.Context, protocol contracts.ToolProtocol, raw json.RawMessage) (json.RawMessage, error) {
	if v.validate != nil {
		return v.validate(ctx, protocol, raw)
	}
	return cloneRaw(raw), nil
}

type fakeToolMarketRepository struct {
	items map[uuid.UUID]Item
	byKey map[string]uuid.UUID
}

func newFakeToolMarketRepository() *fakeToolMarketRepository {
	return &fakeToolMarketRepository{
		items: make(map[uuid.UUID]Item),
		byKey: make(map[string]uuid.UUID),
	}
}

func (r *fakeToolMarketRepository) Create(_ context.Context, item Item) (Item, error) {
	if _, exists := r.byKey[item.ToolKey]; exists {
		return Item{}, ErrConflict
	}
	now := time.Now().UTC()
	item.CreatedAt = now
	item.UpdatedAt = now
	r.items[item.ID] = item
	r.byKey[item.ToolKey] = item.ID
	return item, nil
}

func (r *fakeToolMarketRepository) GetByID(_ context.Context, id uuid.UUID) (Item, error) {
	item, exists := r.items[id]
	if !exists {
		return Item{}, ErrNotFound
	}
	return item, nil
}

func (r *fakeToolMarketRepository) List(_ context.Context, filter ListFilter) ([]Item, error) {
	items := make([]Item, 0)
	for _, item := range r.items {
		if filter.Provider != "" && item.Provider != filter.Provider {
			continue
		}
		if filter.Status != nil && item.Status != *filter.Status {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *fakeToolMarketRepository) Update(_ context.Context, id uuid.UUID, patch UpdatePatch) (Item, error) {
	item, exists := r.items[id]
	if !exists {
		return Item{}, ErrNotFound
	}
	if patch.ToolKey != nil {
		if otherID, used := r.byKey[*patch.ToolKey]; used && otherID != id {
			return Item{}, ErrConflict
		}
		delete(r.byKey, item.ToolKey)
		item.ToolKey = *patch.ToolKey
		r.byKey[item.ToolKey] = id
	}
	if patch.Name != nil {
		item.Name = *patch.Name
	}
	if patch.Provider != nil {
		item.Provider = *patch.Provider
	}
	if patch.Protocol != nil {
		item.Protocol = *patch.Protocol
	}
	if patch.Config != nil {
		item.Config = cloneRaw(*patch.Config)
	}
	if patch.Status != nil {
		item.Status = *patch.Status
	}
	item.UpdatedAt = time.Now().UTC()
	r.items[id] = item
	return item, nil
}

func (r *fakeToolMarketRepository) Delete(_ context.Context, id uuid.UUID) error {
	item, exists := r.items[id]
	if !exists {
		return ErrNotFound
	}
	delete(r.byKey, item.ToolKey)
	delete(r.items, id)
	return nil
}

func TestService_CreateAndListSuccess(t *testing.T) {
	repo := newFakeToolMarketRepository()
	service := NewService(repo, fakeConfigValidator{})

	created, err := service.Create(context.Background(), uuid.New(), CreateInput{
		ToolKey:  "mcp-device-helper",
		Name:     "Device Helper",
		Provider: "acme",
		Protocol: "mcp",
		Config: json.RawMessage(`{
			"transport":"stream_http",
			"stream_http":{
				"endpoint":"https://example.com/mcp"
			}
		}`),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if created.Status != contracts.ToolStatusActive {
		t.Fatalf("expected status active, got %q", created.Status)
	}
	if created.Protocol != contracts.ToolProtocolMCP {
		t.Fatalf("expected protocol mcp, got %q", created.Protocol)
	}

	items, err := service.List(context.Background(), ListInput{
		Provider:   "acme",
		OnlyActive: true,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected list size 1, got %d", len(items))
	}
	if items[0].ID != created.ID {
		t.Fatalf("expected item id %s, got %s", created.ID, items[0].ID)
	}
}

func TestService_CreateRejectsInvalidConfig(t *testing.T) {
	service := NewService(newFakeToolMarketRepository(), fakeConfigValidator{
		validate: func(ctx context.Context, protocol contracts.ToolProtocol, raw json.RawMessage) (json.RawMessage, error) {
			return nil, fmt.Errorf("%w: bad config", toolvalidator.ErrInvalidArgument)
		},
	})

	_, err := service.Create(context.Background(), uuid.New(), CreateInput{
		ToolKey:  "mcp-invalid",
		Name:     "Invalid Tool",
		Provider: "acme",
		Protocol: "mcp",
		Config: json.RawMessage(`{
			"transport":"sse",
			"sse":{}
		}`),
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestService_UpdateRejectsConfigWithUnsupportedTransport(t *testing.T) {
	repo := newFakeToolMarketRepository()
	service := NewService(repo, fakeConfigValidator{
		validate: func(ctx context.Context, protocol contracts.ToolProtocol, raw json.RawMessage) (json.RawMessage, error) {
			if bytes.Contains(raw, []byte(`"transport":"grpc"`)) {
				return nil, fmt.Errorf("%w: unsupported transport", toolvalidator.ErrInvalidArgument)
			}
			return cloneRaw(raw), nil
		},
	})

	created, err := service.Create(context.Background(), uuid.New(), CreateInput{
		ToolKey:  "mcp-stdout-tool",
		Name:     "StdIO Tool",
		Provider: "acme",
		Protocol: "mcp",
		Config: json.RawMessage(`{
			"transport":"stdio",
			"stdio":{"command":"python"}
		}`),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	config := json.RawMessage(`{
		"transport":"grpc",
		"stdio":{"command":"python"}
	}`)
	_, err = service.Update(context.Background(), created.ID, UpdateInput{Config: &config})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	cloned := make([]byte, len(raw))
	copy(cloned, raw)
	return json.RawMessage(cloned)
}
