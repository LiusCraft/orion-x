package toolentitlement

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/auth"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/toolmarket"
)

type fakeEntitlementRepository struct {
	entitlements map[uuid.UUID]Entitlement
	usage        map[uuid.UUID][]UsageEntry
}

func newFakeEntitlementRepository() *fakeEntitlementRepository {
	return &fakeEntitlementRepository{
		entitlements: make(map[uuid.UUID]Entitlement),
		usage:        make(map[uuid.UUID][]UsageEntry),
	}
}

func (r *fakeEntitlementRepository) Create(_ context.Context, entitlement Entitlement) (Entitlement, error) {
	now := time.Now().UTC()
	entitlement.CreatedAt = now
	entitlement.UpdatedAt = now
	r.entitlements[entitlement.ID] = entitlement
	return entitlement, nil
}

func (r *fakeEntitlementRepository) ListByUser(_ context.Context, userID uuid.UUID, status *contracts.EntitlementStatus) ([]Entitlement, error) {
	items := make([]Entitlement, 0)
	for _, item := range r.entitlements {
		if item.UserID != userID {
			continue
		}
		if status != nil && item.Status != *status {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *fakeEntitlementRepository) GetByIDForUser(_ context.Context, id, userID uuid.UUID) (Entitlement, error) {
	item, exists := r.entitlements[id]
	if !exists || item.UserID != userID {
		return Entitlement{}, ErrNotFound
	}
	return item, nil
}

func (r *fakeEntitlementRepository) ListUsageByEntitlement(_ context.Context, entitlementID uuid.UUID) ([]UsageEntry, error) {
	entries := r.usage[entitlementID]
	cloned := make([]UsageEntry, len(entries))
	copy(cloned, entries)
	return cloned, nil
}

type fakeEntitlementItemReader struct {
	items map[uuid.UUID]toolmarket.Item
}

func (r *fakeEntitlementItemReader) GetByID(_ context.Context, id uuid.UUID) (toolmarket.Item, error) {
	item, exists := r.items[id]
	if !exists {
		return toolmarket.Item{}, toolmarket.ErrNotFound
	}
	return item, nil
}

type fakeEntitlementUserReader struct {
	users map[uuid.UUID]auth.User
}

func (r *fakeEntitlementUserReader) GetByID(_ context.Context, id uuid.UUID) (auth.User, error) {
	user, exists := r.users[id]
	if !exists {
		return auth.User{}, auth.ErrUserNotFound
	}
	return user, nil
}

func TestService_ActivateAndUsageFlow(t *testing.T) {
	repo := newFakeEntitlementRepository()
	userID := uuid.New()
	itemID := uuid.New()

	service := NewService(
		repo,
		&fakeEntitlementItemReader{items: map[uuid.UUID]toolmarket.Item{
			itemID: {ID: itemID, Status: contracts.ToolStatusActive},
		}},
		&fakeEntitlementUserReader{users: map[uuid.UUID]auth.User{
			userID: {ID: userID, Status: contracts.UserStatusActive},
		}},
	)

	entitlement, err := service.Activate(context.Background(), userID, ActivateInput{ItemID: itemID})
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if entitlement.Status != contracts.EntitlementStatusActive {
		t.Fatalf("expected active status, got %q", entitlement.Status)
	}

	_, err = service.Activate(context.Background(), userID, ActivateInput{ItemID: itemID})
	if !errors.Is(err, ErrBusinessRule) {
		t.Fatalf("expected ErrBusinessRule for duplicate activation, got %v", err)
	}

	summary, err := service.GetUsage(context.Background(), userID, entitlement.ID)
	if err != nil {
		t.Fatalf("GetUsage() error = %v", err)
	}
	if summary.RemainingQuota != nil {
		t.Fatalf("expected unlimited quota (nil), got %#v", summary.RemainingQuota)
	}
}

func TestService_GrantFutureEntitlementStartsAsPending(t *testing.T) {
	repo := newFakeEntitlementRepository()
	actorID := uuid.New()
	userID := uuid.New()
	itemID := uuid.New()

	now := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	service := NewService(
		repo,
		&fakeEntitlementItemReader{items: map[uuid.UUID]toolmarket.Item{itemID: {ID: itemID, Status: contracts.ToolStatusActive}}},
		&fakeEntitlementUserReader{users: map[uuid.UUID]auth.User{userID: {ID: userID, Status: contracts.UserStatusActive}}},
	)
	service.now = func() time.Time { return now }

	startsAt := now.Add(30 * time.Minute)
	granted, err := service.Grant(context.Background(), actorID, GrantInput{
		UserID:   userID,
		ItemID:   itemID,
		StartsAt: &startsAt,
	})
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	if granted.Status != contracts.EntitlementStatusPending {
		t.Fatalf("expected pending status, got %q", granted.Status)
	}
}

func TestService_ListRepoMarksExpiredAsNotUsable(t *testing.T) {
	repo := newFakeEntitlementRepository()
	userID := uuid.New()
	itemID := uuid.New()

	now := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(-time.Minute)
	entitlementID := uuid.New()
	repo.entitlements[entitlementID] = Entitlement{
		ID:         entitlementID,
		UserID:     userID,
		ToolItemID: itemID,
		Status:     contracts.EntitlementStatusActive,
		StartsAt:   now.Add(-time.Hour),
		ExpiresAt:  &expiresAt,
	}

	service := NewService(
		repo,
		&fakeEntitlementItemReader{items: map[uuid.UUID]toolmarket.Item{itemID: {ID: itemID, Status: contracts.ToolStatusActive}}},
		&fakeEntitlementUserReader{users: map[uuid.UUID]auth.User{userID: {ID: userID, Status: contracts.UserStatusActive}}},
	)
	service.now = func() time.Time { return now }

	items, err := service.ListRepo(context.Background(), userID, RepoListInput{})
	if err != nil {
		t.Fatalf("ListRepo() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected list size 1, got %d", len(items))
	}
	if items[0].Entitlement.Status != contracts.EntitlementStatusExpired {
		t.Fatalf("expected expired status, got %q", items[0].Entitlement.Status)
	}
	if items[0].IsUsable {
		t.Fatalf("expected entitlement unusable after expiry")
	}
}

func TestService_GetRepoEntryReturnsUsableEntry(t *testing.T) {
	repo := newFakeEntitlementRepository()
	userID := uuid.New()
	itemID := uuid.New()
	entitlementID := uuid.New()

	now := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	repo.entitlements[entitlementID] = Entitlement{
		ID:         entitlementID,
		UserID:     userID,
		ToolItemID: itemID,
		Status:     contracts.EntitlementStatusActive,
		StartsAt:   now.Add(-time.Minute),
	}

	service := NewService(
		repo,
		&fakeEntitlementItemReader{items: map[uuid.UUID]toolmarket.Item{itemID: {ID: itemID, Status: contracts.ToolStatusActive}}},
		&fakeEntitlementUserReader{users: map[uuid.UUID]auth.User{userID: {ID: userID, Status: contracts.UserStatusActive}}},
	)
	service.now = func() time.Time { return now }

	entry, err := service.GetRepoEntry(context.Background(), userID, entitlementID)
	if err != nil {
		t.Fatalf("GetRepoEntry() error = %v", err)
	}
	if !entry.IsUsable {
		t.Fatalf("expected entry usable")
	}
	if entry.Entitlement.ID != entitlementID {
		t.Fatalf("expected entitlement id %s, got %s", entitlementID.String(), entry.Entitlement.ID.String())
	}
}

func TestService_GetRepoEntryNotFound(t *testing.T) {
	service := NewService(
		newFakeEntitlementRepository(),
		&fakeEntitlementItemReader{items: map[uuid.UUID]toolmarket.Item{}},
		&fakeEntitlementUserReader{users: map[uuid.UUID]auth.User{}},
	)

	_, err := service.GetRepoEntry(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
