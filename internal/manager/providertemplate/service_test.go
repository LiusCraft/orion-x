package providertemplate

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
)

type fakeRepository struct {
	items map[uuid.UUID]Template
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{items: make(map[uuid.UUID]Template)}
}

func (r *fakeRepository) Create(_ context.Context, template Template) (Template, error) {
	now := time.Now().UTC()
	template.CreatedAt = now
	template.UpdatedAt = now
	r.items[template.ID] = template
	return template, nil
}

func (r *fakeRepository) GetByID(_ context.Context, id uuid.UUID) (Template, error) {
	t, ok := r.items[id]
	if !ok {
		return Template{}, ErrNotFound
	}
	return t, nil
}

func (r *fakeRepository) List(_ context.Context, filter ListFilter) ([]Template, error) {
	list := make([]Template, 0)
	for _, item := range r.items {
		if filter.Category != nil && item.Category != *filter.Category {
			continue
		}
		if filter.Provider != "" && item.Provider != filter.Provider {
			continue
		}
		if filter.Status != nil && item.Status != *filter.Status {
			continue
		}
		list = append(list, item)
	}
	return list, nil
}

func (r *fakeRepository) Update(_ context.Context, id uuid.UUID, patch UpdatePatch) (Template, error) {
	t, ok := r.items[id]
	if !ok {
		return Template{}, ErrNotFound
	}
	if patch.Category != nil {
		t.Category = *patch.Category
	}
	if patch.Provider != nil {
		t.Provider = *patch.Provider
	}
	if patch.Status != nil {
		t.Status = *patch.Status
	}
	if patch.Version != nil {
		t.Version = *patch.Version
	}
	if patch.Fields != nil {
		t.Fields = append(json.RawMessage(nil), *patch.Fields...)
	}
	t.UpdatedAt = time.Now().UTC()
	r.items[id] = t
	return t, nil
}

func (r *fakeRepository) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func TestService_CreateAndList(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo)
	createdBy := uuid.New()

	created, err := service.Create(context.Background(), createdBy, CreateInput{
		Category: "llm",
		Provider: "zhipu",
		Status:   "active",
		Version:  1,
		Fields: []Field{
			{Key: "model", Label: "Model", Type: "select", Required: true, Options: []FieldOption{{Label: "glm-4", Value: "glm-4"}}},
			{Key: "temperature", Label: "Temperature", Type: "number", Min: floatPtr(0), Max: floatPtr(2), Step: floatPtr(0.1)},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if created.Category != contracts.ResourceLLM {
		t.Fatalf("expected category llm, got %q", created.Category)
	}
	if created.Provider != "zhipu" {
		t.Fatalf("expected provider zhipu, got %q", created.Provider)
	}
	if created.Version != 1 {
		t.Fatalf("expected version 1, got %d", created.Version)
	}
	if len(created.Fields) == 0 {
		t.Fatalf("expected fields json payload")
	}

	list, err := service.List(context.Background(), ListInput{Category: "llm", Provider: "zhipu", Status: "active"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one template in list, got %d", len(list))
	}
}

func TestService_CreateRejectsReservedOrConflictingPath(t *testing.T) {
	service := NewService(newFakeRepository())
	createdBy := uuid.New()

	_, err := service.Create(context.Background(), createdBy, CreateInput{
		Category: "llm",
		Provider: "zhipu",
		Version:  1,
		Fields: []Field{
			{Key: "base_url", Label: "Base URL", Type: "text"},
		},
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for reserved path, got %v", err)
	}

	_, err = service.Create(context.Background(), createdBy, CreateInput{
		Category: "llm",
		Provider: "zhipu",
		Version:  1,
		Fields: []Field{
			{Key: "audio", Label: "Audio", Type: "text"},
			{Key: "audio.codec", Label: "Codec", Type: "text"},
		},
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for path conflict, got %v", err)
	}
}

func TestService_UpdateAndDelete(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo)

	created, err := service.Create(context.Background(), uuid.New(), CreateInput{
		Category: "tts",
		Provider: "dashscope",
		Version:  1,
		Fields: []Field{
			{Key: "voice", Label: "Voice", Type: "text", Required: true},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	status := "inactive"
	version := 2
	fields := []Field{{Key: "voice", Label: "Voice", Type: "text", Required: true}, {Key: "rate", Label: "Rate", Type: "number", Min: floatPtr(0.5), Max: floatPtr(2)}}
	updated, err := service.Update(context.Background(), created.ID, UpdateInput{
		Status:  &status,
		Version: &version,
		Fields:  &fields,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Status != contracts.ResourceStatusInactive {
		t.Fatalf("expected status inactive, got %q", updated.Status)
	}
	if updated.Version != 2 {
		t.Fatalf("expected version 2, got %d", updated.Version)
	}

	if err := service.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := service.Delete(context.Background(), created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound on second delete, got %v", err)
	}
}

func floatPtr(v float64) *float64 {
	return &v
}
