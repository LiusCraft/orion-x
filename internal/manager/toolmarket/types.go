package toolmarket

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
)

type Item struct {
	ID        uuid.UUID
	ToolKey   string
	Name      string
	Provider  string
	Protocol  contracts.ToolProtocol
	Config    json.RawMessage
	Status    contracts.ToolStatus
	CreatedBy uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateInput struct {
	ToolKey  string
	Name     string
	Provider string
	Protocol string
	Config   json.RawMessage
	Status   string
}

type ListInput struct {
	Provider   string
	Status     string
	OnlyActive bool
}

type UpdateInput struct {
	ToolKey  *string          `json:"tool_key"`
	Name     *string          `json:"name"`
	Provider *string          `json:"provider"`
	Protocol *string          `json:"protocol"`
	Config   *json.RawMessage `json:"config"`
	Status   *string          `json:"status"`
}

func (in UpdateInput) HasChanges() bool {
	return in.ToolKey != nil ||
		in.Name != nil ||
		in.Provider != nil ||
		in.Protocol != nil ||
		in.Config != nil ||
		in.Status != nil
}

type ListFilter struct {
	Provider string
	Status   *contracts.ToolStatus
}

type UpdatePatch struct {
	ToolKey  *string
	Name     *string
	Provider *string
	Protocol *contracts.ToolProtocol
	Config   *json.RawMessage
	Status   *contracts.ToolStatus
}

func (patch UpdatePatch) HasChanges() bool {
	return patch.ToolKey != nil ||
		patch.Name != nil ||
		patch.Provider != nil ||
		patch.Protocol != nil ||
		patch.Config != nil ||
		patch.Status != nil
}

type Repository interface {
	Create(ctx context.Context, item Item) (Item, error)
	GetByID(ctx context.Context, id uuid.UUID) (Item, error)
	List(ctx context.Context, filter ListFilter) ([]Item, error)
	Update(ctx context.Context, id uuid.UUID, patch UpdatePatch) (Item, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type ConfigValidator interface {
	Validate(ctx context.Context, protocol contracts.ToolProtocol, raw json.RawMessage) (json.RawMessage, error)
}
