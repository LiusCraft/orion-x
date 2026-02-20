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
		&ProviderTemplateModel{},
		&PlatformResourceModel{},
		&PlatformResourceVersionModel{},
		&ToolMarketItemModel{},
		&ToolOfferModel{},
		&UserToolEntitlementModel{},
		&ToolUsageLedgerModel{},
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
	BaseURL       string          `gorm:"type:text;not null"`
	AccessKey     string          `gorm:"type:text;not null"`
	Capabilities  json.RawMessage `gorm:"type:jsonb;not null"`
	Config        json.RawMessage `gorm:"type:jsonb;not null"`
	Status        string          `gorm:"type:text;not null;index:idx_platform_resources_category_provider_status"`
	CreatedBy     uuid.UUID       `gorm:"type:uuid;not null"`
	CreatedAt     time.Time       `gorm:"type:timestamptz;not null"`
	UpdatedAt     time.Time       `gorm:"type:timestamptz;not null"`
}

func (PlatformResourceModel) TableName() string { return "platform_resources" }

type PlatformResourceVersionModel struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey"`
	EntryID           uuid.UUID       `gorm:"type:uuid;not null;uniqueIndex:uniq_platform_resource_versions_entry_version,priority:1"`
	Version           int             `gorm:"not null;uniqueIndex:uniq_platform_resource_versions_entry_version,priority:2"`
	BaseURLSnapshot   string          `gorm:"type:text;not null"`
	AccessKeySnapshot string          `gorm:"type:text;not null"`
	ConfigSnapshot    json.RawMessage `gorm:"type:jsonb;not null"`
	PublishedAt       time.Time       `gorm:"type:timestamptz;not null"`
}

func (PlatformResourceVersionModel) TableName() string { return "platform_resource_versions" }

type ProviderTemplateModel struct {
	ID        uuid.UUID       `gorm:"type:uuid;primaryKey"`
	Category  string          `gorm:"type:text;not null;index:idx_provider_templates_category_provider_status"`
	Provider  string          `gorm:"type:text;not null;index:idx_provider_templates_category_provider_status"`
	Status    string          `gorm:"type:text;not null;index:idx_provider_templates_category_provider_status"`
	Version   int             `gorm:"not null"`
	Fields    json.RawMessage `gorm:"type:jsonb;not null"`
	CreatedBy uuid.UUID       `gorm:"type:uuid;not null"`
	CreatedAt time.Time       `gorm:"type:timestamptz;not null"`
	UpdatedAt time.Time       `gorm:"type:timestamptz;not null"`
	DeletedAt gorm.DeletedAt  `gorm:"type:timestamptz;index"`
}

func (ProviderTemplateModel) TableName() string { return "provider_templates" }

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

type ToolMarketItemModel struct {
	ID        uuid.UUID       `gorm:"type:uuid;primaryKey"`
	ToolKey   string          `gorm:"type:text;not null;uniqueIndex:uniq_tool_market_items_tool_key"`
	Name      string          `gorm:"type:text;not null"`
	Provider  string          `gorm:"type:text;not null;index:idx_tool_market_items_provider_status"`
	Protocol  string          `gorm:"type:text;not null"`
	Config    json.RawMessage `gorm:"type:jsonb;not null"`
	Status    string          `gorm:"type:text;not null;index:idx_tool_market_items_provider_status"`
	CreatedBy uuid.UUID       `gorm:"type:uuid;not null"`
	CreatedAt time.Time       `gorm:"type:timestamptz;not null"`
	UpdatedAt time.Time       `gorm:"type:timestamptz;not null"`
}

func (ToolMarketItemModel) TableName() string { return "tool_market_items" }

type ToolOfferModel struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	ToolItemID      uuid.UUID `gorm:"type:uuid;not null;index:idx_tool_offers_tool_item_status"`
	OfferType       string    `gorm:"type:text;not null"`
	Price           *float64  `gorm:"type:numeric(18,2)"`
	Currency        *string   `gorm:"type:text"`
	QuotaTotal      *int64    `gorm:"type:bigint"`
	DurationSeconds *int64    `gorm:"type:bigint"`
	Status          string    `gorm:"type:text;not null;index:idx_tool_offers_tool_item_status"`
	CreatedAt       time.Time `gorm:"type:timestamptz;not null"`
	UpdatedAt       time.Time `gorm:"type:timestamptz;not null"`
}

func (ToolOfferModel) TableName() string { return "tool_offers" }

type UserToolEntitlementModel struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null;index:idx_user_tool_entitlements_user_status"`
	ToolItemID uuid.UUID  `gorm:"type:uuid;not null;index"`
	OfferID    uuid.UUID  `gorm:"type:uuid;not null;index"`
	SourceType string     `gorm:"type:text;not null"`
	SourceRef  string     `gorm:"type:text;not null"`
	Status     string     `gorm:"type:text;not null;index:idx_user_tool_entitlements_user_status"`
	StartsAt   time.Time  `gorm:"type:timestamptz;not null"`
	ExpiresAt  *time.Time `gorm:"type:timestamptz"`
	QuotaTotal *int64     `gorm:"type:bigint"`
	QuotaUsed  int64      `gorm:"type:bigint;not null;default:0"`
	CreatedAt  time.Time  `gorm:"type:timestamptz;not null"`
	UpdatedAt  time.Time  `gorm:"type:timestamptz;not null"`
}

func (UserToolEntitlementModel) TableName() string { return "user_tool_entitlements" }

type ToolUsageLedgerModel struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey"`
	EntitlementID uuid.UUID  `gorm:"type:uuid;not null;index:idx_tool_usage_ledger_entitlement"`
	VoicebotID    *uuid.UUID `gorm:"type:uuid;index"`
	DeviceID      *uuid.UUID `gorm:"type:uuid;index"`
	ConsumedUnits int64      `gorm:"type:bigint;not null"`
	CreatedAt     time.Time  `gorm:"type:timestamptz;not null"`
}

func (ToolUsageLedgerModel) TableName() string { return "tool_usage_ledger" }
