package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/platformresource"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type platformResourceRepository struct {
	db *gorm.DB
}

func NewPlatformResourceRepository(db *gorm.DB) platformresource.Repository {
	return &platformResourceRepository{db: db}
}

func (r *platformResourceRepository) Create(ctx context.Context, resource platformresource.Resource) (platformresource.Resource, error) {
	if r.db == nil {
		return platformresource.Resource{}, errors.New("platform resource repository db is not initialized")
	}
	if resource.ID == uuid.Nil {
		return platformresource.Resource{}, fmt.Errorf("%w: resource id is required", platformresource.ErrInvalidArgument)
	}

	now := time.Now().UTC()
	model := PlatformResourceModel{
		ID:            resource.ID,
		Category:      string(resource.Category),
		Provider:      resource.Provider,
		ResourceKey:   resource.ResourceKey,
		Name:          resource.Name,
		SchemaVersion: resource.SchemaVersion,
		Capabilities:  copyRawMessage(resource.Capabilities),
		Config:        copyRawMessage(resource.Config),
		CredentialRef: resource.CredentialRef,
		Status:        string(resource.Status),
		CreatedBy:     resource.CreatedBy,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return platformresource.Resource{}, fmt.Errorf("begin create platform resource transaction: %w", tx.Error)
	}
	defer func() {
		if rec := recover(); rec != nil {
			tx.Rollback()
			panic(rec)
		}
	}()

	if err := tx.Create(&model).Error; err != nil {
		tx.Rollback()
		if isUniqueConstraintError(err) {
			return platformresource.Resource{}, platformresource.ErrConflict
		}
		return platformresource.Resource{}, fmt.Errorf("insert platform resource: %w", err)
	}

	if err := insertPlatformResourceVersion(tx, model, now); err != nil {
		tx.Rollback()
		return platformresource.Resource{}, fmt.Errorf("insert initial platform resource version: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return platformresource.Resource{}, fmt.Errorf("commit create platform resource transaction: %w", err)
	}

	return mapPlatformResourceModel(model), nil
}

func (r *platformResourceRepository) GetByID(ctx context.Context, id uuid.UUID) (platformresource.Resource, error) {
	if r.db == nil {
		return platformresource.Resource{}, errors.New("platform resource repository db is not initialized")
	}
	if id == uuid.Nil {
		return platformresource.Resource{}, fmt.Errorf("%w: resource id is required", platformresource.ErrInvalidArgument)
	}

	var model PlatformResourceModel
	err := r.db.WithContext(ctx).Take(&model, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return platformresource.Resource{}, platformresource.ErrNotFound
		}
		return platformresource.Resource{}, fmt.Errorf("query platform resource by id %s: %w", id.String(), err)
	}

	return mapPlatformResourceModel(model), nil
}

func (r *platformResourceRepository) List(ctx context.Context, filter platformresource.ListFilter) ([]platformresource.Resource, error) {
	if r.db == nil {
		return nil, errors.New("platform resource repository db is not initialized")
	}

	query := r.db.WithContext(ctx).Model(&PlatformResourceModel{})
	if filter.Category != nil {
		query = query.Where("category = ?", string(*filter.Category))
	}
	if filter.Provider != "" {
		query = query.Where("provider = ?", filter.Provider)
	}
	if filter.Status != nil {
		query = query.Where("status = ?", string(*filter.Status))
	}

	var models []PlatformResourceModel
	if err := query.Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("query platform resources: %w", err)
	}

	items := make([]platformresource.Resource, 0, len(models))
	for _, model := range models {
		items = append(items, mapPlatformResourceModel(model))
	}
	return items, nil
}

func (r *platformResourceRepository) Update(ctx context.Context, id uuid.UUID, patch platformresource.UpdatePatch) (platformresource.Resource, error) {
	if r.db == nil {
		return platformresource.Resource{}, errors.New("platform resource repository db is not initialized")
	}
	if id == uuid.Nil {
		return platformresource.Resource{}, fmt.Errorf("%w: resource id is required", platformresource.ErrInvalidArgument)
	}
	if !patch.HasChanges() {
		return platformresource.Resource{}, fmt.Errorf("%w: at least one field is required", platformresource.ErrInvalidArgument)
	}

	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return platformresource.Resource{}, fmt.Errorf("begin update platform resource transaction: %w", tx.Error)
	}
	defer func() {
		if rec := recover(); rec != nil {
			tx.Rollback()
			panic(rec)
		}
	}()

	var model PlatformResourceModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&model, "id = ?", id).Error
	if err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return platformresource.Resource{}, platformresource.ErrNotFound
		}
		return platformresource.Resource{}, fmt.Errorf("query platform resource by id %s: %w", id.String(), err)
	}

	if patch.Category != nil {
		model.Category = string(*patch.Category)
	}
	if patch.Provider != nil {
		model.Provider = *patch.Provider
	}
	if patch.ResourceKey != nil {
		model.ResourceKey = *patch.ResourceKey
	}
	if patch.Name != nil {
		model.Name = *patch.Name
	}
	if patch.SchemaVersion != nil {
		model.SchemaVersion = *patch.SchemaVersion
	}
	if patch.Capabilities != nil {
		model.Capabilities = copyRawMessage(*patch.Capabilities)
	}
	if patch.Config != nil {
		model.Config = copyRawMessage(*patch.Config)
	}
	if patch.CredentialRef != nil {
		model.CredentialRef = *patch.CredentialRef
	}
	if patch.Status != nil {
		model.Status = string(*patch.Status)
	}
	model.UpdatedAt = time.Now().UTC()

	if err := tx.Save(&model).Error; err != nil {
		tx.Rollback()
		if isUniqueConstraintError(err) {
			return platformresource.Resource{}, platformresource.ErrConflict
		}
		return platformresource.Resource{}, fmt.Errorf("update platform resource %s: %w", id.String(), err)
	}

	if err := insertPlatformResourceVersion(tx, model, model.UpdatedAt); err != nil {
		tx.Rollback()
		return platformresource.Resource{}, fmt.Errorf("insert platform resource version: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return platformresource.Resource{}, fmt.Errorf("commit update platform resource transaction: %w", err)
	}

	return mapPlatformResourceModel(model), nil
}

func (r *platformResourceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if r.db == nil {
		return errors.New("platform resource repository db is not initialized")
	}
	if id == uuid.Nil {
		return fmt.Errorf("%w: resource id is required", platformresource.ErrInvalidArgument)
	}

	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("begin delete platform resource transaction: %w", tx.Error)
	}
	defer func() {
		if rec := recover(); rec != nil {
			tx.Rollback()
			panic(rec)
		}
	}()

	result := tx.Delete(&PlatformResourceModel{}, "id = ?", id)
	if result.Error != nil {
		tx.Rollback()
		return fmt.Errorf("delete platform resource %s: %w", id.String(), result.Error)
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		return platformresource.ErrNotFound
	}

	if err := tx.Delete(&PlatformResourceVersionModel{}, "entry_id = ?", id).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("delete platform resource versions %s: %w", id.String(), err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("commit delete platform resource transaction: %w", err)
	}

	return nil
}

func mapPlatformResourceModel(model PlatformResourceModel) platformresource.Resource {
	return platformresource.Resource{
		ID:            model.ID,
		Category:      contracts.ResourceCategory(model.Category),
		Provider:      model.Provider,
		ResourceKey:   model.ResourceKey,
		Name:          model.Name,
		SchemaVersion: model.SchemaVersion,
		Capabilities:  copyRawMessage(model.Capabilities),
		Config:        copyRawMessage(model.Config),
		CredentialRef: model.CredentialRef,
		Status:        contracts.ResourceStatus(model.Status),
		CreatedBy:     model.CreatedBy,
		CreatedAt:     model.CreatedAt,
		UpdatedAt:     model.UpdatedAt,
	}
}

func insertPlatformResourceVersion(tx *gorm.DB, model PlatformResourceModel, publishedAt time.Time) error {
	nextVersion, err := nextPlatformResourceVersion(tx, model.ID)
	if err != nil {
		return fmt.Errorf("compute next version for resource %s: %w", model.ID.String(), err)
	}

	versionModel := PlatformResourceVersionModel{
		ID:                    uuid.New(),
		EntryID:               model.ID,
		Version:               nextVersion,
		ConfigSnapshot:        copyRawMessage(model.Config),
		CredentialRefSnapshot: model.CredentialRef,
		PublishedAt:           publishedAt,
	}
	if err := tx.Create(&versionModel).Error; err != nil {
		return err
	}

	return nil
}

func nextPlatformResourceVersion(tx *gorm.DB, entryID uuid.UUID) (int, error) {
	var latest PlatformResourceVersionModel
	err := tx.Where("entry_id = ?", entryID).Order("version DESC").Take(&latest).Error
	if err == nil {
		return latest.Version + 1, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 1, nil
	}
	return 0, err
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "duplicate key") || strings.Contains(lower, "unique constraint") || strings.Contains(lower, "unique failed")
}

func copyRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	cloned := make([]byte, len(raw))
	copy(cloned, raw)
	return json.RawMessage(cloned)
}
