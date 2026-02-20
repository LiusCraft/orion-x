package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

const CurrentSchemaVersion = 4

type MigrationResult struct {
	TargetVersion   int
	PreviousVersion int
	Applied         bool
	Tables          []string
}

type Migrator interface {
	Migrate(ctx context.Context) (MigrationResult, error)
}

type GormMigrator struct {
	db *gorm.DB
}

func NewMigrator(db *gorm.DB) *GormMigrator {
	return &GormMigrator{db: db}
}

func (m *GormMigrator) Migrate(ctx context.Context) (MigrationResult, error) {
	tx := m.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return MigrationResult{}, fmt.Errorf("begin migration transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	if err := tx.AutoMigrate(&schemaMigrationModel{}); err != nil {
		tx.Rollback()
		return MigrationResult{}, fmt.Errorf("auto migrate schema_migrations: %w", err)
	}

	previousVersion, err := getCurrentVersion(tx)
	if err != nil {
		tx.Rollback()
		return MigrationResult{}, fmt.Errorf("query current schema version: %w", err)
	}
	if previousVersion > CurrentSchemaVersion {
		tx.Rollback()
		return MigrationResult{}, fmt.Errorf("database schema version %d is newer than supported version %d", previousVersion, CurrentSchemaVersion)
	}

	result := MigrationResult{
		TargetVersion:   CurrentSchemaVersion,
		PreviousVersion: previousVersion,
		Tables:          migrationTableNames(),
	}
	if previousVersion == CurrentSchemaVersion {
		if err := tx.Commit().Error; err != nil {
			return MigrationResult{}, fmt.Errorf("commit no-op migration transaction: %w", err)
		}
		return result, nil
	}

	if err := tx.AutoMigrate(MigrationModels()...); err != nil {
		tx.Rollback()
		return MigrationResult{}, fmt.Errorf("auto migrate manager tables: %w", err)
	}

	if err := migratePlatformResourceSchema(tx); err != nil {
		tx.Rollback()
		return MigrationResult{}, err
	}

	record := schemaMigrationModel{
		Version:   CurrentSchemaVersion,
		AppliedAt: time.Now().UTC(),
	}
	if err := tx.Create(&record).Error; err != nil {
		tx.Rollback()
		return MigrationResult{}, fmt.Errorf("insert schema migration record: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return MigrationResult{}, fmt.Errorf("commit migration transaction: %w", err)
	}

	result.Applied = true
	return result, nil
}

type schemaMigrationModel struct {
	ID        uint      `gorm:"primaryKey"`
	Version   int       `gorm:"not null;uniqueIndex"`
	AppliedAt time.Time `gorm:"type:timestamptz;not null"`
}

func (schemaMigrationModel) TableName() string {
	return "schema_migrations"
}

func getCurrentVersion(db *gorm.DB) (int, error) {
	var row schemaMigrationModel
	err := db.Order("version DESC").Take(&row).Error
	if err == nil {
		return row.Version, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return 0, err
}

func migrationTableNames() []string {
	return []string{
		"users",
		"provider_templates",
		"platform_resources",
		"platform_resource_versions",
		"tool_market_items",
		"tool_offers",
		"user_tool_entitlements",
		"tool_usage_ledger",
		"voicebots",
		"devices",
		"device_bindings",
	}
}

func migratePlatformResourceSchema(tx *gorm.DB) error {
	if tx == nil {
		return errors.New("migration db is nil")
	}

	migrator := tx.Migrator()
	if migrator.HasColumn(&PlatformResourceModel{}, "credential_ref") {
		if err := migrator.DropColumn(&PlatformResourceModel{}, "credential_ref"); err != nil {
			return fmt.Errorf("drop platform_resources.credential_ref: %w", err)
		}
	}
	if migrator.HasColumn(&PlatformResourceVersionModel{}, "credential_ref_snapshot") {
		if err := migrator.DropColumn(&PlatformResourceVersionModel{}, "credential_ref_snapshot"); err != nil {
			return fmt.Errorf("drop platform_resource_versions.credential_ref_snapshot: %w", err)
		}
	}

	return nil
}
