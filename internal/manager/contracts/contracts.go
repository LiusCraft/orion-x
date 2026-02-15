package contracts

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type UserRole string

const (
	RoleAdmin      UserRole = "admin"
	RoleNormalUser UserRole = "normal_user"
)

type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
)

type ResourceCategory string

const (
	ResourceLLM ResourceCategory = "llm"
	ResourceASR ResourceCategory = "asr"
	ResourceTTS ResourceCategory = "tts"
)

type ResourceStatus string

const (
	ResourceStatusActive   ResourceStatus = "active"
	ResourceStatusInactive ResourceStatus = "inactive"
)

type DeviceStatus string

const (
	DeviceStatusActive   DeviceStatus = "active"
	DeviceStatusDisabled DeviceStatus = "disabled"
)

type User struct {
	ID        uuid.UUID
	Email     string
	Role      UserRole
	Status    UserStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

type PlatformResource struct {
	ID            uuid.UUID
	Category      ResourceCategory
	Provider      string
	ResourceKey   string
	Status        ResourceStatus
	SchemaVersion int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Voicebot struct {
	ID          uuid.UUID
	OwnerUserID uuid.UUID
	VoicebotKey string
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Device struct {
	ID          uuid.UUID
	DeviceID    string
	OwnerUserID uuid.UUID
	Status      DeviceStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type DeviceBinding struct {
	DeviceID      uuid.UUID
	VoicebotID    uuid.UUID
	BoundByUserID uuid.UUID
	Version       int
	UpdatedAt     time.Time
}

type UserRepository interface {
	Create(ctx context.Context, user User) error
	GetByID(ctx context.Context, id uuid.UUID) (User, error)
	GetByEmail(ctx context.Context, email string) (User, error)
}

type PlatformResourceRepository interface {
	Create(ctx context.Context, entry PlatformResource) error
	List(ctx context.Context, category ResourceCategory, provider string, status ResourceStatus) ([]PlatformResource, error)
}

type VoicebotRepository interface {
	Create(ctx context.Context, voicebot Voicebot) error
	ListByOwner(ctx context.Context, ownerUserID uuid.UUID) ([]Voicebot, error)
}

type DeviceRepository interface {
	Create(ctx context.Context, device Device) error
	ListByOwner(ctx context.Context, ownerUserID uuid.UUID) ([]Device, error)
	UpsertBinding(ctx context.Context, binding DeviceBinding) error
}
