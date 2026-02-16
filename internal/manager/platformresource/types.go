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
	Capabilities  json.RawMessage
	Config        json.RawMessage
	CredentialRef string
	Status        contracts.ResourceStatus
	CreatedBy     uuid.UUID
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Version struct {
	ID                    uuid.UUID
	EntryID               uuid.UUID
	Version               int
	ConfigSnapshot        json.RawMessage
	CredentialRefSnapshot string
	PublishedAt           time.Time
}

type CreateInput struct {
	Category      string
	Provider      string
	ResourceKey   string
	Name          string
	SchemaVersion int
	Capabilities  json.RawMessage
	Config        json.RawMessage
	CredentialRef string
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
	Capabilities  *json.RawMessage
	Config        *json.RawMessage
	CredentialRef *string
	Status        *string
}

func (in UpdateInput) HasChanges() bool {
	return in.Category != nil ||
		in.Provider != nil ||
		in.ResourceKey != nil ||
		in.Name != nil ||
		in.SchemaVersion != nil ||
		in.Capabilities != nil ||
		in.Config != nil ||
		in.CredentialRef != nil ||
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
	Capabilities  *json.RawMessage
	Config        *json.RawMessage
	CredentialRef *string
	Status        *contracts.ResourceStatus
}

func (patch UpdatePatch) HasChanges() bool {
	return patch.Category != nil ||
		patch.Provider != nil ||
		patch.ResourceKey != nil ||
		patch.Name != nil ||
		patch.SchemaVersion != nil ||
		patch.Capabilities != nil ||
		patch.Config != nil ||
		patch.CredentialRef != nil ||
		patch.Status != nil
}

type Repository interface {
	Create(ctx context.Context, resource Resource) (Resource, error)
	GetByID(ctx context.Context, id uuid.UUID) (Resource, error)
	List(ctx context.Context, filter ListFilter) ([]Resource, error)
	Update(ctx context.Context, id uuid.UUID, patch UpdatePatch) (Resource, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
