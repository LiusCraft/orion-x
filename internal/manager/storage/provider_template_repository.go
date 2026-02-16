package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/providertemplate"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type providerTemplateRepository struct {
	db *gorm.DB
}

func NewProviderTemplateRepository(db *gorm.DB) providertemplate.Repository {
	return &providerTemplateRepository{db: db}
}

func (r *providerTemplateRepository) Create(ctx context.Context, template providertemplate.Template) (providertemplate.Template, error) {
	if r.db == nil {
		return providertemplate.Template{}, errors.New("provider template repository db is not initialized")
	}
	if template.ID == uuid.Nil {
		return providertemplate.Template{}, fmt.Errorf("%w: template id is required", providertemplate.ErrInvalidArgument)
	}

	now := time.Now().UTC()
	model := ProviderTemplateModel{
		ID:        template.ID,
		Category:  string(template.Category),
		Provider:  template.Provider,
		Status:    string(template.Status),
		Version:   template.Version,
		Fields:    copyRawMessage(template.Fields),
		CreatedBy: template.CreatedBy,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		if isUniqueConstraintError(err) {
			return providertemplate.Template{}, providertemplate.ErrConflict
		}
		return providertemplate.Template{}, fmt.Errorf("insert provider template: %w", err)
	}

	return mapProviderTemplateModel(model), nil
}

func (r *providerTemplateRepository) GetByID(ctx context.Context, id uuid.UUID) (providertemplate.Template, error) {
	if r.db == nil {
		return providertemplate.Template{}, errors.New("provider template repository db is not initialized")
	}
	if id == uuid.Nil {
		return providertemplate.Template{}, fmt.Errorf("%w: template id is required", providertemplate.ErrInvalidArgument)
	}

	var model ProviderTemplateModel
	err := r.db.WithContext(ctx).Take(&model, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return providertemplate.Template{}, providertemplate.ErrNotFound
		}
		return providertemplate.Template{}, fmt.Errorf("query provider template by id %s: %w", id.String(), err)
	}

	return mapProviderTemplateModel(model), nil
}

func (r *providerTemplateRepository) List(ctx context.Context, filter providertemplate.ListFilter) ([]providertemplate.Template, error) {
	if r.db == nil {
		return nil, errors.New("provider template repository db is not initialized")
	}

	query := r.db.WithContext(ctx).Model(&ProviderTemplateModel{})
	if filter.Category != nil {
		query = query.Where("category = ?", string(*filter.Category))
	}
	if filter.Provider != "" {
		query = query.Where("provider = ?", filter.Provider)
	}
	if filter.Status != nil {
		query = query.Where("status = ?", string(*filter.Status))
	}

	var models []ProviderTemplateModel
	if err := query.Order("updated_at DESC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("query provider templates: %w", err)
	}

	items := make([]providertemplate.Template, 0, len(models))
	for _, model := range models {
		items = append(items, mapProviderTemplateModel(model))
	}
	return items, nil
}

func (r *providerTemplateRepository) Update(ctx context.Context, id uuid.UUID, patch providertemplate.UpdatePatch) (providertemplate.Template, error) {
	if r.db == nil {
		return providertemplate.Template{}, errors.New("provider template repository db is not initialized")
	}
	if id == uuid.Nil {
		return providertemplate.Template{}, fmt.Errorf("%w: template id is required", providertemplate.ErrInvalidArgument)
	}
	if !patch.HasChanges() {
		return providertemplate.Template{}, fmt.Errorf("%w: at least one field is required", providertemplate.ErrInvalidArgument)
	}

	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return providertemplate.Template{}, fmt.Errorf("begin update provider template transaction: %w", tx.Error)
	}
	defer func() {
		if rec := recover(); rec != nil {
			tx.Rollback()
			panic(rec)
		}
	}()

	var model ProviderTemplateModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&model, "id = ?", id).Error
	if err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return providertemplate.Template{}, providertemplate.ErrNotFound
		}
		return providertemplate.Template{}, fmt.Errorf("query provider template by id %s: %w", id.String(), err)
	}

	if patch.Category != nil {
		model.Category = string(*patch.Category)
	}
	if patch.Provider != nil {
		model.Provider = *patch.Provider
	}
	if patch.Status != nil {
		model.Status = string(*patch.Status)
	}
	if patch.Version != nil {
		model.Version = *patch.Version
	}
	if patch.Fields != nil {
		model.Fields = copyRawMessage(*patch.Fields)
	}
	model.UpdatedAt = time.Now().UTC()

	if err := tx.Save(&model).Error; err != nil {
		tx.Rollback()
		if isUniqueConstraintError(err) {
			return providertemplate.Template{}, providertemplate.ErrConflict
		}
		return providertemplate.Template{}, fmt.Errorf("update provider template %s: %w", id.String(), err)
	}

	if err := tx.Commit().Error; err != nil {
		return providertemplate.Template{}, fmt.Errorf("commit update provider template transaction: %w", err)
	}

	return mapProviderTemplateModel(model), nil
}

func (r *providerTemplateRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if r.db == nil {
		return errors.New("provider template repository db is not initialized")
	}
	if id == uuid.Nil {
		return fmt.Errorf("%w: template id is required", providertemplate.ErrInvalidArgument)
	}

	result := r.db.WithContext(ctx).Delete(&ProviderTemplateModel{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("delete provider template %s: %w", id.String(), result.Error)
	}
	if result.RowsAffected == 0 {
		return providertemplate.ErrNotFound
	}
	return nil
}

func mapProviderTemplateModel(model ProviderTemplateModel) providertemplate.Template {
	return providertemplate.Template{
		ID:        model.ID,
		Category:  contracts.ResourceCategory(model.Category),
		Provider:  model.Provider,
		Status:    contracts.ResourceStatus(model.Status),
		Version:   model.Version,
		Fields:    copyRawMessage(model.Fields),
		CreatedBy: model.CreatedBy,
		CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt,
	}
}
