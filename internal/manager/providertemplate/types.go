package providertemplate

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
)

type FieldOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type Field struct {
	Key          string        `json:"key"`
	Label        string        `json:"label"`
	Type         string        `json:"type"`
	Required     bool          `json:"required"`
	DefaultValue any           `json:"default_value,omitempty"`
	HelperText   string        `json:"helper_text,omitempty"`
	Placeholder  string        `json:"placeholder,omitempty"`
	Min          *float64      `json:"min,omitempty"`
	Max          *float64      `json:"max,omitempty"`
	Step         *float64      `json:"step,omitempty"`
	Options      []FieldOption `json:"options,omitempty"`
}

type Template struct {
	ID        uuid.UUID
	Category  contracts.ResourceCategory
	Provider  string
	Status    contracts.ResourceStatus
	Version   int
	Fields    json.RawMessage
	CreatedBy uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateInput struct {
	Category string
	Provider string
	Status   string
	Version  int
	Fields   []Field
}

type ListInput struct {
	Category string
	Provider string
	Status   string
}

type UpdateInput struct {
	Category *string
	Provider *string
	Status   *string
	Version  *int
	Fields   *[]Field
}

func (in UpdateInput) HasChanges() bool {
	return in.Category != nil || in.Provider != nil || in.Status != nil || in.Version != nil || in.Fields != nil
}

type ListFilter struct {
	Category *contracts.ResourceCategory
	Provider string
	Status   *contracts.ResourceStatus
}

type UpdatePatch struct {
	Category *contracts.ResourceCategory
	Provider *string
	Status   *contracts.ResourceStatus
	Version  *int
	Fields   *json.RawMessage
}

func (patch UpdatePatch) HasChanges() bool {
	return patch.Category != nil || patch.Provider != nil || patch.Status != nil || patch.Version != nil || patch.Fields != nil
}

type Repository interface {
	Create(ctx context.Context, template Template) (Template, error)
	GetByID(ctx context.Context, id uuid.UUID) (Template, error)
	List(ctx context.Context, filter ListFilter) ([]Template, error)
	Update(ctx context.Context, id uuid.UUID, patch UpdatePatch) (Template, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
