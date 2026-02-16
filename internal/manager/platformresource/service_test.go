package platformresource

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
)

type serviceFakeRepository struct {
	resources   map[uuid.UUID]Resource
	resourceIDs map[string]uuid.UUID
	versions    map[uuid.UUID]int
}

func newServiceFakeRepository() *serviceFakeRepository {
	return &serviceFakeRepository{
		resources:   make(map[uuid.UUID]Resource),
		resourceIDs: make(map[string]uuid.UUID),
		versions:    make(map[uuid.UUID]int),
	}
}

func (r *serviceFakeRepository) Create(_ context.Context, resource Resource) (Resource, error) {
	if _, exists := r.resourceIDs[resource.ResourceKey]; exists {
		return Resource{}, ErrConflict
	}
	now := time.Now().UTC()
	resource.CreatedAt = now
	resource.UpdatedAt = now
	r.resources[resource.ID] = resource
	r.resourceIDs[resource.ResourceKey] = resource.ID
	r.versions[resource.ID] = 1
	return resource, nil
}

func (r *serviceFakeRepository) GetByID(_ context.Context, id uuid.UUID) (Resource, error) {
	resource, exists := r.resources[id]
	if !exists {
		return Resource{}, ErrNotFound
	}
	return resource, nil
}

func (r *serviceFakeRepository) List(_ context.Context, filter ListFilter) ([]Resource, error) {
	items := make([]Resource, 0)
	for _, item := range r.resources {
		if filter.Category != nil && item.Category != *filter.Category {
			continue
		}
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

func (r *serviceFakeRepository) Update(_ context.Context, id uuid.UUID, patch UpdatePatch) (Resource, error) {
	resource, exists := r.resources[id]
	if !exists {
		return Resource{}, ErrNotFound
	}
	if !patch.HasChanges() {
		return Resource{}, ErrInvalidArgument
	}

	if patch.ResourceKey != nil {
		if otherID, exists := r.resourceIDs[*patch.ResourceKey]; exists && otherID != id {
			return Resource{}, ErrConflict
		}
		delete(r.resourceIDs, resource.ResourceKey)
		resource.ResourceKey = *patch.ResourceKey
		r.resourceIDs[resource.ResourceKey] = id
	}
	if patch.Category != nil {
		resource.Category = *patch.Category
	}
	if patch.Provider != nil {
		resource.Provider = *patch.Provider
	}
	if patch.Name != nil {
		resource.Name = *patch.Name
	}
	if patch.SchemaVersion != nil {
		resource.SchemaVersion = *patch.SchemaVersion
	}
	if patch.Capabilities != nil {
		resource.Capabilities = copyRaw(*patch.Capabilities)
	}
	if patch.Config != nil {
		resource.Config = copyRaw(*patch.Config)
	}
	if patch.CredentialRef != nil {
		resource.CredentialRef = *patch.CredentialRef
	}
	if patch.Status != nil {
		resource.Status = *patch.Status
	}

	resource.UpdatedAt = time.Now().UTC()
	r.resources[id] = resource
	r.versions[id]++
	return resource, nil
}

func (r *serviceFakeRepository) Delete(_ context.Context, id uuid.UUID) error {
	resource, exists := r.resources[id]
	if !exists {
		return ErrNotFound
	}
	delete(r.resources, id)
	delete(r.resourceIDs, resource.ResourceKey)
	delete(r.versions, id)
	return nil
}

func TestService_CreateAndListSuccess(t *testing.T) {
	repo := newServiceFakeRepository()
	service := NewService(repo)
	createdBy := uuid.New()

	created, err := service.Create(context.Background(), createdBy, CreateInput{
		Category:      "LLM",
		Provider:      "ZhIPu",
		ResourceKey:   "LLM-ZHIPU-PROD",
		Name:          "Zhipu Production",
		SchemaVersion: 1,
		Capabilities:  json.RawMessage(`{"stream":true}`),
		Config:        json.RawMessage(`{"model":"glm-4-flash"}`),
		CredentialRef: "secret://manager/llm/zhipu/prod",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if created.Category != contracts.ResourceLLM {
		t.Fatalf("expected category llm, got %q", created.Category)
	}
	if created.Provider != ProviderZhipu {
		t.Fatalf("expected provider zhipu, got %q", created.Provider)
	}
	if created.Status != contracts.ResourceStatusActive {
		t.Fatalf("expected default status active, got %q", created.Status)
	}
	if created.ResourceKey != "llm-zhipu-prod" {
		t.Fatalf("expected normalized resource key, got %q", created.ResourceKey)
	}
	if repo.versions[created.ID] != 1 {
		t.Fatalf("expected initial version count 1, got %d", repo.versions[created.ID])
	}

	items, err := service.List(context.Background(), ListInput{
		Category: "llm",
		Provider: "zhipu",
		Status:   "active",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected list size 1, got %d", len(items))
	}
	if items[0].ID != created.ID {
		t.Fatalf("expected listed id %s, got %s", created.ID, items[0].ID)
	}
}

func TestService_CreateRejectsInvalidCategoryOrProvider(t *testing.T) {
	repo := newServiceFakeRepository()
	service := NewService(repo)
	createdBy := uuid.New()

	tests := []struct {
		name  string
		input CreateInput
	}{
		{
			name: "invalid category",
			input: CreateInput{
				Category:      "vision",
				Provider:      "dashscope",
				ResourceKey:   "vision-dashscope-main",
				Name:          "invalid category",
				SchemaVersion: 1,
				Capabilities:  json.RawMessage(`{"stream":true}`),
				Config:        json.RawMessage(`{"model":"x"}`),
				CredentialRef: "secret://x",
			},
		},
		{
			name: "invalid provider",
			input: CreateInput{
				Category:      "asr",
				Provider:      "zhipu",
				ResourceKey:   "asr-zhipu-main",
				Name:          "invalid provider",
				SchemaVersion: 1,
				Capabilities:  json.RawMessage(`{"stream":true}`),
				Config:        json.RawMessage(`{"model":"x"}`),
				CredentialRef: "secret://x",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.Create(context.Background(), createdBy, tt.input)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("expected ErrInvalidArgument, got %v", err)
			}
		})
	}
}

func TestService_UpdateRejectsProviderCategoryMismatch(t *testing.T) {
	repo := newServiceFakeRepository()
	service := NewService(repo)

	created, err := service.Create(context.Background(), uuid.New(), CreateInput{
		Category:      "asr",
		Provider:      "dashscope",
		ResourceKey:   "asr-dashscope-prod",
		Name:          "ASR Prod",
		SchemaVersion: 1,
		Capabilities:  json.RawMessage(`{"stream":true}`),
		Config:        json.RawMessage(`{"model":"fun-asr-realtime"}`),
		CredentialRef: "secret://manager/asr/dashscope/prod",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	provider := "zhipu"
	_, err = service.Update(context.Background(), created.ID, UpdateInput{Provider: &provider})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestService_UpdateSuccessIncrementsVersion(t *testing.T) {
	repo := newServiceFakeRepository()
	service := NewService(repo)

	created, err := service.Create(context.Background(), uuid.New(), CreateInput{
		Category:      "llm",
		Provider:      "zhipu",
		ResourceKey:   "llm-zhipu-prod",
		Name:          "LLM Prod",
		SchemaVersion: 1,
		Capabilities:  json.RawMessage(`{"stream":true}`),
		Config:        json.RawMessage(`{"model":"glm-4-flash"}`),
		CredentialRef: "secret://manager/llm/zhipu/prod",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updatedName := "LLM Prod V2"
	schemaVersion := 2
	updatedConfig := json.RawMessage(`{"model":"glm-4-air"}`)

	updated, err := service.Update(context.Background(), created.ID, UpdateInput{
		Name:          &updatedName,
		SchemaVersion: &schemaVersion,
		Config:        &updatedConfig,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if updated.Name != updatedName {
		t.Fatalf("expected updated name %q, got %q", updatedName, updated.Name)
	}
	if updated.SchemaVersion != schemaVersion {
		t.Fatalf("expected schema version %d, got %d", schemaVersion, updated.SchemaVersion)
	}
	if string(updated.Config) != string(updatedConfig) {
		t.Fatalf("expected updated config %s, got %s", string(updatedConfig), string(updated.Config))
	}
	if repo.versions[created.ID] != 2 {
		t.Fatalf("expected version count 2 after update, got %d", repo.versions[created.ID])
	}
}

func TestService_ListRejectsUnknownProviderFilter(t *testing.T) {
	service := NewService(newServiceFakeRepository())

	_, err := service.List(context.Background(), ListInput{Provider: "unknown-provider"})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

func copyRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	cloned := make([]byte, len(raw))
	copy(cloned, raw)
	return json.RawMessage(cloned)
}
