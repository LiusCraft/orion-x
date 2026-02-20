package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/toolmarket"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type toolMarketRepository struct {
	db *gorm.DB
}

func NewToolMarketRepository(db *gorm.DB) toolmarket.Repository {
	return &toolMarketRepository{db: db}
}

func (r *toolMarketRepository) Create(ctx context.Context, item toolmarket.Item) (toolmarket.Item, error) {
	if r.db == nil {
		return toolmarket.Item{}, errors.New("tool market repository db is not initialized")
	}
	if item.ID == uuid.Nil {
		return toolmarket.Item{}, fmt.Errorf("%w: item id is required", toolmarket.ErrInvalidArgument)
	}

	now := time.Now().UTC()
	model := ToolMarketItemModel{
		ID:        item.ID,
		ToolKey:   item.ToolKey,
		Name:      item.Name,
		Provider:  item.Provider,
		Protocol:  string(item.Protocol),
		Config:    copyRawMessage(item.Config),
		Status:    string(item.Status),
		CreatedBy: item.CreatedBy,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		if isUniqueConstraintError(err) {
			return toolmarket.Item{}, toolmarket.ErrConflict
		}
		return toolmarket.Item{}, fmt.Errorf("insert tool market item: %w", err)
	}

	return mapToolMarketItemModel(model), nil
}

func (r *toolMarketRepository) GetByID(ctx context.Context, id uuid.UUID) (toolmarket.Item, error) {
	if r.db == nil {
		return toolmarket.Item{}, errors.New("tool market repository db is not initialized")
	}
	if id == uuid.Nil {
		return toolmarket.Item{}, fmt.Errorf("%w: item id is required", toolmarket.ErrInvalidArgument)
	}

	var model ToolMarketItemModel
	err := r.db.WithContext(ctx).Take(&model, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return toolmarket.Item{}, toolmarket.ErrNotFound
		}
		return toolmarket.Item{}, fmt.Errorf("query tool market item by id %s: %w", id.String(), err)
	}

	return mapToolMarketItemModel(model), nil
}

func (r *toolMarketRepository) List(ctx context.Context, filter toolmarket.ListFilter) ([]toolmarket.Item, error) {
	if r.db == nil {
		return nil, errors.New("tool market repository db is not initialized")
	}

	query := r.db.WithContext(ctx).Model(&ToolMarketItemModel{})
	if filter.Provider != "" {
		query = query.Where("provider = ?", filter.Provider)
	}
	if filter.Status != nil {
		query = query.Where("status = ?", string(*filter.Status))
	}

	var models []ToolMarketItemModel
	if err := query.Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("query tool market items: %w", err)
	}

	items := make([]toolmarket.Item, 0, len(models))
	for _, model := range models {
		items = append(items, mapToolMarketItemModel(model))
	}

	return items, nil
}

func (r *toolMarketRepository) Update(ctx context.Context, id uuid.UUID, patch toolmarket.UpdatePatch) (toolmarket.Item, error) {
	if r.db == nil {
		return toolmarket.Item{}, errors.New("tool market repository db is not initialized")
	}
	if id == uuid.Nil {
		return toolmarket.Item{}, fmt.Errorf("%w: item id is required", toolmarket.ErrInvalidArgument)
	}
	if !patch.HasChanges() {
		return toolmarket.Item{}, fmt.Errorf("%w: at least one field is required", toolmarket.ErrInvalidArgument)
	}

	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return toolmarket.Item{}, fmt.Errorf("begin update tool market item transaction: %w", tx.Error)
	}
	defer func() {
		if rec := recover(); rec != nil {
			tx.Rollback()
			panic(rec)
		}
	}()

	var model ToolMarketItemModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&model, "id = ?", id).Error
	if err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return toolmarket.Item{}, toolmarket.ErrNotFound
		}
		return toolmarket.Item{}, fmt.Errorf("query tool market item by id %s: %w", id.String(), err)
	}

	if patch.ToolKey != nil {
		model.ToolKey = *patch.ToolKey
	}
	if patch.Name != nil {
		model.Name = *patch.Name
	}
	if patch.Provider != nil {
		model.Provider = *patch.Provider
	}
	if patch.Protocol != nil {
		model.Protocol = string(*patch.Protocol)
	}
	if patch.Config != nil {
		model.Config = copyRawMessage(*patch.Config)
	}
	if patch.Status != nil {
		model.Status = string(*patch.Status)
	}
	model.UpdatedAt = time.Now().UTC()

	if err := tx.Save(&model).Error; err != nil {
		tx.Rollback()
		if isUniqueConstraintError(err) {
			return toolmarket.Item{}, toolmarket.ErrConflict
		}
		return toolmarket.Item{}, fmt.Errorf("update tool market item %s: %w", id.String(), err)
	}

	if err := tx.Commit().Error; err != nil {
		return toolmarket.Item{}, fmt.Errorf("commit update tool market item transaction: %w", err)
	}

	return mapToolMarketItemModel(model), nil
}

func (r *toolMarketRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if r.db == nil {
		return errors.New("tool market repository db is not initialized")
	}
	if id == uuid.Nil {
		return fmt.Errorf("%w: item id is required", toolmarket.ErrInvalidArgument)
	}

	result := r.db.WithContext(ctx).Delete(&ToolMarketItemModel{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("delete tool market item %s: %w", id.String(), result.Error)
	}
	if result.RowsAffected == 0 {
		return toolmarket.ErrNotFound
	}

	return nil
}

func mapToolMarketItemModel(model ToolMarketItemModel) toolmarket.Item {
	return toolmarket.Item{
		ID:        model.ID,
		ToolKey:   model.ToolKey,
		Name:      model.Name,
		Provider:  model.Provider,
		Protocol:  contracts.ToolProtocol(model.Protocol),
		Config:    copyRawMessage(model.Config),
		Status:    contracts.ToolStatus(model.Status),
		CreatedBy: model.CreatedBy,
		CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt,
	}
}
