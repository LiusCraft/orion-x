package platformresource

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
)

const (
	ProviderDashScope = "dashscope"
	ProviderOpenAI    = "openai"
	ProviderZhipu     = "zhipu"
)

type Resource struct {
	ID            uuid.UUID
	Category      contracts.ResourceCategory
	Provider      string
	ResourceKey   string
	Name          string
	SchemaVersion int
	BaseURL       string
	AccessKey     string
	HasAccessKey  bool
	Capabilities  json.RawMessage
	Config        json.RawMessage
	Status        contracts.ResourceStatus
	CreatedBy     uuid.UUID
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Version struct {
	ID                uuid.UUID
	EntryID           uuid.UUID
	Version           int
	BaseURLSnapshot   string
	AccessKeySnapshot string
	ConfigSnapshot    json.RawMessage
	PublishedAt       time.Time
}

type CreateInput struct {
	Category      string
	Provider      string
	ResourceKey   string
	Name          string
	SchemaVersion int
	BaseURL       string
	AccessKey     string
	Capabilities  json.RawMessage
	Config        json.RawMessage
	Status        string
}

type ListInput struct {
	Category string
	Provider string
	Status   string
}

type UpdateInput struct {
	Category      *string
	Provider      *string
	ResourceKey   *string
	Name          *string
	SchemaVersion *int
	BaseURL       *string
	AccessKey     *string
	Capabilities  *json.RawMessage
	Config        *json.RawMessage
	Status        *string
}

func (in UpdateInput) HasChanges() bool {
	return in.Category != nil ||
		in.Provider != nil ||
		in.ResourceKey != nil ||
		in.Name != nil ||
		in.SchemaVersion != nil ||
		in.BaseURL != nil ||
		in.AccessKey != nil ||
		in.Capabilities != nil ||
		in.Config != nil ||
		in.Status != nil
}

type ListFilter struct {
	Category *contracts.ResourceCategory
	Provider string
	Status   *contracts.ResourceStatus
}

type UpdatePatch struct {
	Category      *contracts.ResourceCategory
	Provider      *string
	ResourceKey   *string
	Name          *string
	SchemaVersion *int
	BaseURL       *string
	AccessKey     *string
	Capabilities  *json.RawMessage
	Config        *json.RawMessage
	Status        *contracts.ResourceStatus
}

func (patch UpdatePatch) HasChanges() bool {
	return patch.Category != nil ||
		patch.Provider != nil ||
		patch.ResourceKey != nil ||
		patch.Name != nil ||
		patch.SchemaVersion != nil ||
		patch.BaseURL != nil ||
		patch.AccessKey != nil ||
		patch.Capabilities != nil ||
		patch.Config != nil ||
		patch.Status != nil
}

type Repository interface {
	Create(ctx context.Context, resource Resource) (Resource, error)
	GetByID(ctx context.Context, id uuid.UUID) (Resource, error)
	List(ctx context.Context, filter ListFilter) ([]Resource, error)
	Update(ctx context.Context, id uuid.UUID, patch UpdatePatch) (Resource, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
