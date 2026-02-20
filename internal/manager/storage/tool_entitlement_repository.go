package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/toolentitlement"
	"gorm.io/gorm"
)

type toolEntitlementRepository struct {
	db *gorm.DB
}

func NewToolEntitlementRepository(db *gorm.DB) toolentitlement.Repository {
	return &toolEntitlementRepository{db: db}
}

func (r *toolEntitlementRepository) Create(ctx context.Context, entitlement toolentitlement.Entitlement) (toolentitlement.Entitlement, error) {
	if r.db == nil {
		return toolentitlement.Entitlement{}, errors.New("tool entitlement repository db is not initialized")
	}
	if entitlement.ID == uuid.Nil || entitlement.UserID == uuid.Nil || entitlement.ToolItemID == uuid.Nil {
		return toolentitlement.Entitlement{}, fmt.Errorf("%w: id, user_id and tool_item_id are required", toolentitlement.ErrInvalidArgument)
	}

	now := time.Now().UTC()
	model := UserToolEntitlementModel{
		ID:         entitlement.ID,
		UserID:     entitlement.UserID,
		ToolItemID: entitlement.ToolItemID,
		OfferID:    entitlement.OfferID,
		SourceType: string(entitlement.SourceType),
		SourceRef:  strings.TrimSpace(entitlement.SourceRef),
		Status:     string(entitlement.Status),
		StartsAt:   entitlement.StartsAt.UTC(),
		ExpiresAt:  normalizeTimePtr(entitlement.ExpiresAt),
		QuotaTotal: cloneInt64Value(entitlement.QuotaTotal),
		QuotaUsed:  entitlement.QuotaUsed,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		if isUniqueConstraintError(err) {
			return toolentitlement.Entitlement{}, toolentitlement.ErrConflict
		}
		return toolentitlement.Entitlement{}, fmt.Errorf("insert user tool entitlement: %w", err)
	}

	return mapUserToolEntitlementModel(model), nil
}

func (r *toolEntitlementRepository) ListByUser(ctx context.Context, userID uuid.UUID, status *contracts.EntitlementStatus) ([]toolentitlement.Entitlement, error) {
	if r.db == nil {
		return nil, errors.New("tool entitlement repository db is not initialized")
	}
	if userID == uuid.Nil {
		return nil, fmt.Errorf("%w: user_id is required", toolentitlement.ErrInvalidArgument)
	}

	query := r.db.WithContext(ctx).Model(&UserToolEntitlementModel{}).Where("user_id = ?", userID)
	if status != nil {
		query = query.Where("status = ?", string(*status))
	}

	var models []UserToolEntitlementModel
	if err := query.Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("query user tool entitlements by user_id %s: %w", userID.String(), err)
	}

	items := make([]toolentitlement.Entitlement, 0, len(models))
	for _, model := range models {
		items = append(items, mapUserToolEntitlementModel(model))
	}

	return items, nil
}

func (r *toolEntitlementRepository) GetByIDForUser(ctx context.Context, id, userID uuid.UUID) (toolentitlement.Entitlement, error) {
	if r.db == nil {
		return toolentitlement.Entitlement{}, errors.New("tool entitlement repository db is not initialized")
	}
	if id == uuid.Nil || userID == uuid.Nil {
		return toolentitlement.Entitlement{}, fmt.Errorf("%w: id and user_id are required", toolentitlement.ErrInvalidArgument)
	}

	var model UserToolEntitlementModel
	err := r.db.WithContext(ctx).Take(&model, "id = ? AND user_id = ?", id, userID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return toolentitlement.Entitlement{}, toolentitlement.ErrNotFound
		}
		return toolentitlement.Entitlement{}, fmt.Errorf("query entitlement by id %s and user_id %s: %w", id.String(), userID.String(), err)
	}

	return mapUserToolEntitlementModel(model), nil
}

func (r *toolEntitlementRepository) ListUsageByEntitlement(ctx context.Context, entitlementID uuid.UUID) ([]toolentitlement.UsageEntry, error) {
	if r.db == nil {
		return nil, errors.New("tool entitlement repository db is not initialized")
	}
	if entitlementID == uuid.Nil {
		return nil, fmt.Errorf("%w: entitlement_id is required", toolentitlement.ErrInvalidArgument)
	}

	var models []ToolUsageLedgerModel
	err := r.db.WithContext(ctx).
		Model(&ToolUsageLedgerModel{}).
		Where("entitlement_id = ?", entitlementID).
		Order("created_at DESC").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("query usage ledger by entitlement_id %s: %w", entitlementID.String(), err)
	}

	entries := make([]toolentitlement.UsageEntry, 0, len(models))
	for _, model := range models {
		entries = append(entries, mapToolUsageLedgerModel(model))
	}
	return entries, nil
}

func (r *toolEntitlementRepository) ExistsByOfferAndSourceRef(ctx context.Context, offerID uuid.UUID, sourceRef string) (bool, error) {
	if r.db == nil {
		return false, errors.New("tool entitlement repository db is not initialized")
	}
	if offerID == uuid.Nil {
		return false, fmt.Errorf("%w: offer_id is required", toolentitlement.ErrInvalidArgument)
	}
	normalizedRef := strings.TrimSpace(sourceRef)
	if normalizedRef == "" {
		return false, fmt.Errorf("%w: source_ref is required", toolentitlement.ErrInvalidArgument)
	}

	var count int64
	err := r.db.WithContext(ctx).
		Model(&UserToolEntitlementModel{}).
		Where("offer_id = ? AND source_ref = ?", offerID, normalizedRef).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("query entitlement by offer_id %s source_ref: %w", offerID.String(), err)
	}
	return count > 0, nil
}

func mapUserToolEntitlementModel(model UserToolEntitlementModel) toolentitlement.Entitlement {
	return toolentitlement.Entitlement{
		ID:         model.ID,
		UserID:     model.UserID,
		ToolItemID: model.ToolItemID,
		OfferID:    model.OfferID,
		SourceType: contracts.EntitlementSourceType(model.SourceType),
		SourceRef:  model.SourceRef,
		Status:     contracts.EntitlementStatus(model.Status),
		StartsAt:   model.StartsAt,
		ExpiresAt:  normalizeTimePtr(model.ExpiresAt),
		QuotaTotal: cloneInt64Value(model.QuotaTotal),
		QuotaUsed:  model.QuotaUsed,
		CreatedAt:  model.CreatedAt,
		UpdatedAt:  model.UpdatedAt,
	}
}

func mapToolUsageLedgerModel(model ToolUsageLedgerModel) toolentitlement.UsageEntry {
	return toolentitlement.UsageEntry{
		ID:            model.ID,
		EntitlementID: model.EntitlementID,
		VoicebotID:    cloneUUID(model.VoicebotID),
		DeviceID:      cloneUUID(model.DeviceID),
		ConsumedUnits: model.ConsumedUnits,
		CreatedAt:     model.CreatedAt,
	}
}

func normalizeTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := value.UTC()
	return &normalized
}

func cloneUUID(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
