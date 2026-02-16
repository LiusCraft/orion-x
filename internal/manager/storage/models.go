package storage

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func MigrationModels() []any {
	return []any{
		&UserModel{},
		&PlatformResourceModel{},
		&PlatformResourceVersionModel{},
		&VoicebotModel{},
		&DeviceModel{},
		&DeviceBindingModel{},
	}
}

type UserModel struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey"`
	Email        string         `gorm:"type:text;not null;uniqueIndex"`
	PasswordHash string         `gorm:"type:text;not null"`
	Role         string         `gorm:"type:text;not null"`
	Status       string         `gorm:"type:text;not null"`
	CreatedAt    time.Time      `gorm:"type:timestamptz;not null"`
	UpdatedAt    time.Time      `gorm:"type:timestamptz;not null"`
	DeletedAt    gorm.DeletedAt `gorm:"type:timestamptz;index"`
}

func (UserModel) TableName() string { return "users" }

type PlatformResourceModel struct {
	ID            uuid.UUID       `gorm:"type:uuid;primaryKey"`
	Category      string          `gorm:"type:text;not null;index:idx_platform_resources_category_provider_status"`
	Provider      string          `gorm:"type:text;not null;index:idx_platform_resources_category_provider_status"`
	ResourceKey   string          `gorm:"type:text;not null;uniqueIndex:uniq_platform_resources_resource_key"`
	Name          string          `gorm:"type:text;not null"`
	SchemaVersion int             `gorm:"not null"`
	Capabilities  json.RawMessage `gorm:"type:jsonb;not null"`
	Config        json.RawMessage `gorm:"type:jsonb;not null"`
	CredentialRef string          `gorm:"type:text;not null"`
	Status        string          `gorm:"type:text;not null;index:idx_platform_resources_category_provider_status"`
	CreatedBy     uuid.UUID       `gorm:"type:uuid;not null"`
	CreatedAt     time.Time       `gorm:"type:timestamptz;not null"`
	UpdatedAt     time.Time       `gorm:"type:timestamptz;not null"`
}

func (PlatformResourceModel) TableName() string { return "platform_resources" }

type PlatformResourceVersionModel struct {
	ID                    uuid.UUID       `gorm:"type:uuid;primaryKey"`
	EntryID               uuid.UUID       `gorm:"type:uuid;not null;uniqueIndex:uniq_platform_resource_versions_entry_version,priority:1"`
	Version               int             `gorm:"not null;uniqueIndex:uniq_platform_resource_versions_entry_version,priority:2"`
	ConfigSnapshot        json.RawMessage `gorm:"type:jsonb;not null"`
	CredentialRefSnapshot string          `gorm:"type:text;not null"`
	PublishedAt           time.Time       `gorm:"type:timestamptz;not null"`
}

func (PlatformResourceVersionModel) TableName() string { return "platform_resource_versions" }

type VoicebotModel struct {
	ID            uuid.UUID       `gorm:"type:uuid;primaryKey"`
	OwnerUserID   uuid.UUID       `gorm:"type:uuid;not null;index"`
	VoicebotKey   string          `gorm:"type:text;not null;uniqueIndex"`
	Name          string          `gorm:"type:text;not null"`
	LLMResourceID uuid.UUID       `gorm:"type:uuid;not null"`
	ASRResourceID uuid.UUID       `gorm:"type:uuid;not null"`
	TTSResourceID uuid.UUID       `gorm:"type:uuid;not null"`
	Settings      json.RawMessage `gorm:"type:jsonb;not null"`
	IsActive      bool            `gorm:"not null;default:true"`
	CreatedAt     time.Time       `gorm:"type:timestamptz;not null"`
	UpdatedAt     time.Time       `gorm:"type:timestamptz;not null"`
}

func (VoicebotModel) TableName() string { return "voicebots" }

type DeviceModel struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	DeviceID    string    `gorm:"type:text;not null;uniqueIndex"`
	OwnerUserID uuid.UUID `gorm:"type:uuid;not null;index"`
	Name        string    `gorm:"type:text;not null"`
	Status      string    `gorm:"type:text;not null"`
	CreatedAt   time.Time `gorm:"type:timestamptz;not null"`
	UpdatedAt   time.Time `gorm:"type:timestamptz;not null"`
}

func (DeviceModel) TableName() string { return "devices" }

type DeviceBindingModel struct {
	DeviceID      uuid.UUID `gorm:"type:uuid;primaryKey"`
	VoicebotID    uuid.UUID `gorm:"type:uuid;not null;index"`
	BoundByUserID uuid.UUID `gorm:"type:uuid;not null"`
	Version       int       `gorm:"not null;default:1"`
	UpdatedAt     time.Time `gorm:"type:timestamptz;not null"`
}

func (DeviceBindingModel) TableName() string { return "device_bindings" }
