package toolentitlement

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/auth"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/toolmarket"
)

type Entitlement struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	ToolItemID uuid.UUID
	OfferID    uuid.UUID
	SourceType contracts.EntitlementSourceType
	SourceRef  string
	Status     contracts.EntitlementStatus
	StartsAt   time.Time
	ExpiresAt  *time.Time
	QuotaTotal *int64
	QuotaUsed  int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type UsageEntry struct {
	ID            uuid.UUID
	EntitlementID uuid.UUID
	VoicebotID    *uuid.UUID
	DeviceID      *uuid.UUID
	ConsumedUnits int64
	CreatedAt     time.Time
}

type ActivateInput struct {
	ItemID uuid.UUID
}

type GrantInput struct {
	UserID    uuid.UUID
	ItemID    uuid.UUID
	SourceRef string
	StartsAt  *time.Time
}

type RepoListInput struct {
	Status string
}

type RepoEntry struct {
	Entitlement Entitlement
	Item        toolmarket.Item
	IsUsable    bool
}

type UsageSummary struct {
	Entitlement    Entitlement
	RemainingQuota *int64
	Entries        []UsageEntry
}

type Repository interface {
	Create(ctx context.Context, entitlement Entitlement) (Entitlement, error)
	ListByUser(ctx context.Context, userID uuid.UUID, status *contracts.EntitlementStatus) ([]Entitlement, error)
	GetByIDForUser(ctx context.Context, id, userID uuid.UUID) (Entitlement, error)
	ListUsageByEntitlement(ctx context.Context, entitlementID uuid.UUID) ([]UsageEntry, error)
}

type ToolItemReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (toolmarket.Item, error)
}

type UserReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (auth.User, error)
}
