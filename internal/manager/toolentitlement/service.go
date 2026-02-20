package toolentitlement

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/auth"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/toolmarket"
)

type Service struct {
	repo       Repository
	itemReader ToolItemReader
	userReader UserReader
	now        func() time.Time
}

func NewService(repo Repository, itemReader ToolItemReader, userReader UserReader) *Service {
	return &Service{
		repo:       repo,
		itemReader: itemReader,
		userReader: userReader,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *Service) Activate(ctx context.Context, userID uuid.UUID, input ActivateInput) (Entitlement, error) {
	if err := s.validateReady(); err != nil {
		return Entitlement{}, err
	}
	if userID == uuid.Nil || input.ItemID == uuid.Nil {
		return Entitlement{}, fmt.Errorf("%w: user_id and item_id are required", ErrInvalidArgument)
	}

	item, err := s.itemReader.GetByID(ctx, input.ItemID)
	if err != nil {
		if errors.Is(err, toolmarket.ErrNotFound) {
			return Entitlement{}, fmt.Errorf("%w: tool market item not found", ErrNotFound)
		}
		return Entitlement{}, fmt.Errorf("load tool market item: %w", err)
	}
	if item.Status != contracts.ToolStatusActive {
		return Entitlement{}, fmt.Errorf("%w: tool market item is not active", ErrBusinessRule)
	}

	now := s.now()
	if err := s.ensureNotAlreadyActivated(ctx, userID, input.ItemID, now); err != nil {
		return Entitlement{}, err
	}

	entitlement := Entitlement{
		ID:         uuid.New(),
		UserID:     userID,
		ToolItemID: input.ItemID,
		SourceType: contracts.EntitlementSourcePurchase,
		SourceRef:  fmt.Sprintf("self:%s", userID.String()),
		Status:     contracts.EntitlementStatusActive,
		StartsAt:   now,
		QuotaUsed:  0,
	}

	created, err := s.repo.Create(ctx, entitlement)
	if err != nil {
		switch {
		case errors.Is(err, ErrConflict):
			return Entitlement{}, ErrConflict
		case errors.Is(err, ErrInvalidArgument):
			return Entitlement{}, err
		default:
			return Entitlement{}, fmt.Errorf("create entitlement: %w", err)
		}
	}

	return normalizeEntitlement(created, now), nil
}

func (s *Service) Grant(ctx context.Context, actorUserID uuid.UUID, input GrantInput) (Entitlement, error) {
	if err := s.validateReady(); err != nil {
		return Entitlement{}, err
	}
	if actorUserID == uuid.Nil || input.UserID == uuid.Nil || input.ItemID == uuid.Nil {
		return Entitlement{}, fmt.Errorf("%w: actor_user_id, user_id and item_id are required", ErrInvalidArgument)
	}
	if s.userReader == nil {
		return Entitlement{}, errors.New("tool entitlement service user reader is not initialized")
	}

	targetUser, err := s.userReader.GetByID(ctx, input.UserID)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			return Entitlement{}, fmt.Errorf("%w: user not found", ErrNotFound)
		}
		return Entitlement{}, fmt.Errorf("load target user: %w", err)
	}
	if targetUser.Status != contracts.UserStatusActive {
		return Entitlement{}, fmt.Errorf("%w: target user is not active", ErrBusinessRule)
	}

	item, err := s.itemReader.GetByID(ctx, input.ItemID)
	if err != nil {
		if errors.Is(err, toolmarket.ErrNotFound) {
			return Entitlement{}, fmt.Errorf("%w: tool market item not found", ErrNotFound)
		}
		return Entitlement{}, fmt.Errorf("load tool market item: %w", err)
	}
	if item.Status != contracts.ToolStatusActive {
		return Entitlement{}, fmt.Errorf("%w: tool market item is not active", ErrBusinessRule)
	}

	now := s.now()
	if err := s.ensureNotAlreadyActivated(ctx, input.UserID, input.ItemID, now); err != nil {
		return Entitlement{}, err
	}

	startsAt := now
	if input.StartsAt != nil {
		startsAt = input.StartsAt.UTC()
	}
	status := contracts.EntitlementStatusActive
	if startsAt.After(now) {
		status = contracts.EntitlementStatusPending
	}

	sourceRef := strings.TrimSpace(input.SourceRef)
	if sourceRef == "" {
		sourceRef = fmt.Sprintf("admin:%s", actorUserID.String())
	}

	entitlement := Entitlement{
		ID:         uuid.New(),
		UserID:     input.UserID,
		ToolItemID: input.ItemID,
		SourceType: contracts.EntitlementSourceAdminGrant,
		SourceRef:  sourceRef,
		Status:     status,
		StartsAt:   startsAt,
		QuotaUsed:  0,
	}

	created, err := s.repo.Create(ctx, entitlement)
	if err != nil {
		switch {
		case errors.Is(err, ErrConflict):
			return Entitlement{}, ErrConflict
		case errors.Is(err, ErrInvalidArgument):
			return Entitlement{}, err
		default:
			return Entitlement{}, fmt.Errorf("grant entitlement: %w", err)
		}
	}

	return normalizeEntitlement(created, now), nil
}

func (s *Service) ListRepo(ctx context.Context, userID uuid.UUID, input RepoListInput) ([]RepoEntry, error) {
	if err := s.validateReady(); err != nil {
		return nil, err
	}
	if userID == uuid.Nil {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidArgument)
	}

	var statusFilter *contracts.EntitlementStatus
	if strings.TrimSpace(input.Status) != "" {
		parsedStatus, err := parseEntitlementStatus(input.Status)
		if err != nil {
			return nil, err
		}
		statusFilter = &parsedStatus
	}

	entitlements, err := s.repo.ListByUser(ctx, userID, statusFilter)
	if err != nil {
		return nil, fmt.Errorf("list entitlements by user: %w", err)
	}

	now := s.now()
	items := make([]RepoEntry, 0, len(entitlements))
	for _, entitlement := range entitlements {
		normalized := normalizeEntitlement(entitlement, now)

		item, itemErr := s.itemReader.GetByID(ctx, normalized.ToolItemID)
		if itemErr != nil {
			if errors.Is(itemErr, toolmarket.ErrNotFound) {
				continue
			}
			return nil, fmt.Errorf("load tool market item %s: %w", normalized.ToolItemID.String(), itemErr)
		}

		items = append(items, RepoEntry{
			Entitlement: normalized,
			Item:        item,
			IsUsable:    isEntitlementUsable(normalized, now),
		})
	}

	return items, nil
}

func (s *Service) GetRepoEntry(ctx context.Context, userID, entitlementID uuid.UUID) (RepoEntry, error) {
	if err := s.validateReady(); err != nil {
		return RepoEntry{}, err
	}
	if userID == uuid.Nil || entitlementID == uuid.Nil {
		return RepoEntry{}, fmt.Errorf("%w: user_id and entitlement_id are required", ErrInvalidArgument)
	}

	entitlement, err := s.repo.GetByIDForUser(ctx, entitlementID, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return RepoEntry{}, ErrNotFound
		}
		return RepoEntry{}, fmt.Errorf("load entitlement: %w", err)
	}

	now := s.now()
	normalized := normalizeEntitlement(entitlement, now)

	item, err := s.itemReader.GetByID(ctx, normalized.ToolItemID)
	if err != nil {
		if errors.Is(err, toolmarket.ErrNotFound) {
			return RepoEntry{}, ErrNotFound
		}
		return RepoEntry{}, fmt.Errorf("load tool market item %s: %w", normalized.ToolItemID.String(), err)
	}

	return RepoEntry{
		Entitlement: normalized,
		Item:        item,
		IsUsable:    isEntitlementUsable(normalized, now),
	}, nil
}

func (s *Service) GetUsage(ctx context.Context, userID, entitlementID uuid.UUID) (UsageSummary, error) {
	if err := s.validateReady(); err != nil {
		return UsageSummary{}, err
	}
	if userID == uuid.Nil || entitlementID == uuid.Nil {
		return UsageSummary{}, fmt.Errorf("%w: user_id and entitlement_id are required", ErrInvalidArgument)
	}

	entitlement, err := s.repo.GetByIDForUser(ctx, entitlementID, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return UsageSummary{}, ErrNotFound
		}
		return UsageSummary{}, fmt.Errorf("load entitlement: %w", err)
	}

	entries, err := s.repo.ListUsageByEntitlement(ctx, entitlementID)
	if err != nil {
		return UsageSummary{}, fmt.Errorf("list usage entries: %w", err)
	}

	now := s.now()
	normalized := normalizeEntitlement(entitlement, now)

	return UsageSummary{
		Entitlement:    normalized,
		RemainingQuota: remainingQuota(normalized),
		Entries:        entries,
	}, nil
}

func (s *Service) ensureNotAlreadyActivated(ctx context.Context, userID, itemID uuid.UUID, now time.Time) error {
	entitlements, err := s.repo.ListByUser(ctx, userID, nil)
	if err != nil {
		return fmt.Errorf("query existing entitlements: %w", err)
	}

	for _, existing := range entitlements {
		if existing.ToolItemID != itemID {
			continue
		}
		normalized := normalizeEntitlement(existing, now)
		if normalized.Status == contracts.EntitlementStatusActive || normalized.Status == contracts.EntitlementStatusPending {
			return fmt.Errorf("%w: tool already activated", ErrBusinessRule)
		}
	}

	return nil
}

func (s *Service) validateReady() error {
	if s.repo == nil || s.itemReader == nil {
		return errors.New("tool entitlement service dependencies are not initialized")
	}
	if s.now == nil {
		return errors.New("tool entitlement service clock is not initialized")
	}
	return nil
}

func parseEntitlementStatus(value string) (contracts.EntitlementStatus, error) {
	status := contracts.EntitlementStatus(strings.ToLower(strings.TrimSpace(value)))
	switch status {
	case contracts.EntitlementStatusPending,
		contracts.EntitlementStatusActive,
		contracts.EntitlementStatusExpired,
		contracts.EntitlementStatusRevoked:
		return status, nil
	default:
		return "", fmt.Errorf("%w: unsupported status %q", ErrInvalidArgument, value)
	}
}

func normalizeEntitlement(entitlement Entitlement, now time.Time) Entitlement {
	normalized := entitlement
	if normalized.Status == contracts.EntitlementStatusPending && !normalized.StartsAt.After(now) {
		normalized.Status = contracts.EntitlementStatusActive
	}
	if normalized.Status == contracts.EntitlementStatusActive && normalized.ExpiresAt != nil && !normalized.ExpiresAt.After(now) {
		normalized.Status = contracts.EntitlementStatusExpired
	}
	return normalized
}

func isEntitlementUsable(entitlement Entitlement, now time.Time) bool {
	if entitlement.Status != contracts.EntitlementStatusActive {
		return false
	}
	if entitlement.StartsAt.After(now) {
		return false
	}
	if entitlement.ExpiresAt != nil && !entitlement.ExpiresAt.After(now) {
		return false
	}
	if entitlement.QuotaTotal != nil && entitlement.QuotaUsed >= *entitlement.QuotaTotal {
		return false
	}
	return true
}

func remainingQuota(entitlement Entitlement) *int64 {
	if entitlement.QuotaTotal == nil {
		return nil
	}
	remaining := *entitlement.QuotaTotal - entitlement.QuotaUsed
	if remaining < 0 {
		remaining = 0
	}
	return &remaining
}
