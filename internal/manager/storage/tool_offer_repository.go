package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/tooloffer"
	"gorm.io/gorm"
)

type toolOfferRepository struct {
	db *gorm.DB
}

func NewToolOfferRepository(db *gorm.DB) tooloffer.Repository {
	return &toolOfferRepository{db: db}
}

func (r *toolOfferRepository) Create(ctx context.Context, offer tooloffer.Offer) (tooloffer.Offer, error) {
	if r.db == nil {
		return tooloffer.Offer{}, errors.New("tool offer repository db is not initialized")
	}
	if offer.ID == uuid.Nil || offer.ToolItemID == uuid.Nil {
		return tooloffer.Offer{}, fmt.Errorf("%w: id and tool_item_id are required", tooloffer.ErrInvalidArgument)
	}

	now := time.Now().UTC()
	model := ToolOfferModel{
		ID:              offer.ID,
		ToolItemID:      offer.ToolItemID,
		OfferType:       string(offer.OfferType),
		Price:           cloneFloat64(offer.Price),
		Currency:        cloneString(offer.Currency),
		QuotaTotal:      cloneInt64Value(offer.QuotaTotal),
		DurationSeconds: cloneInt64Value(offer.DurationSeconds),
		Status:          string(offer.Status),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		if isUniqueConstraintError(err) {
			return tooloffer.Offer{}, tooloffer.ErrConflict
		}
		return tooloffer.Offer{}, fmt.Errorf("insert tool offer: %w", err)
	}

	return mapToolOfferModel(model), nil
}

func (r *toolOfferRepository) GetByID(ctx context.Context, id uuid.UUID) (tooloffer.Offer, error) {
	if r.db == nil {
		return tooloffer.Offer{}, errors.New("tool offer repository db is not initialized")
	}
	if id == uuid.Nil {
		return tooloffer.Offer{}, fmt.Errorf("%w: offer id is required", tooloffer.ErrInvalidArgument)
	}

	var model ToolOfferModel
	err := r.db.WithContext(ctx).Take(&model, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tooloffer.Offer{}, tooloffer.ErrNotFound
		}
		return tooloffer.Offer{}, fmt.Errorf("query tool offer by id %s: %w", id.String(), err)
	}

	return mapToolOfferModel(model), nil
}

func (r *toolOfferRepository) ListByItem(ctx context.Context, toolItemID uuid.UUID, filter tooloffer.ListFilter) ([]tooloffer.Offer, error) {
	if r.db == nil {
		return nil, errors.New("tool offer repository db is not initialized")
	}
	if toolItemID == uuid.Nil {
		return nil, fmt.Errorf("%w: tool_item_id is required", tooloffer.ErrInvalidArgument)
	}

	query := r.db.WithContext(ctx).Model(&ToolOfferModel{}).Where("tool_item_id = ?", toolItemID)
	if filter.Status != nil {
		query = query.Where("status = ?", string(*filter.Status))
	}

	var models []ToolOfferModel
	if err := query.Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("query tool offers by tool_item_id %s: %w", toolItemID.String(), err)
	}

	offers := make([]tooloffer.Offer, 0, len(models))
	for _, model := range models {
		offers = append(offers, mapToolOfferModel(model))
	}
	return offers, nil
}

func mapToolOfferModel(model ToolOfferModel) tooloffer.Offer {
	return tooloffer.Offer{
		ID:              model.ID,
		ToolItemID:      model.ToolItemID,
		OfferType:       contracts.ToolOfferType(model.OfferType),
		Price:           cloneFloat64(model.Price),
		Currency:        cloneString(model.Currency),
		QuotaTotal:      cloneInt64Value(model.QuotaTotal),
		DurationSeconds: cloneInt64Value(model.DurationSeconds),
		Status:          contracts.ToolStatus(model.Status),
		CreatedAt:       model.CreatedAt,
		UpdatedAt:       model.UpdatedAt,
	}
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneInt64Value(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
