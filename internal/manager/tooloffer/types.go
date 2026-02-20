package tooloffer

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/toolmarket"
)

type Offer struct {
	ID              uuid.UUID
	ToolItemID      uuid.UUID
	OfferType       contracts.ToolOfferType
	Price           *float64
	Currency        *string
	QuotaTotal      *int64
	DurationSeconds *int64
	Status          contracts.ToolStatus
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type CreateInput struct {
	OfferType       string
	Price           *float64
	Currency        *string
	QuotaTotal      *int64
	DurationSeconds *int64
	Status          string
}

type ListInput struct {
	Status     string
	OnlyActive bool
}

type ListFilter struct {
	Status *contracts.ToolStatus
}

type Repository interface {
	Create(ctx context.Context, offer Offer) (Offer, error)
	GetByID(ctx context.Context, id uuid.UUID) (Offer, error)
	ListByItem(ctx context.Context, toolItemID uuid.UUID, filter ListFilter) ([]Offer, error)
}

type ToolItemReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (toolmarket.Item, error)
}
