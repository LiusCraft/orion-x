package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
)

type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	Role         contracts.UserRole
	Status       contracts.UserStatus
}

type Principal struct {
	UserID uuid.UUID
	Email  string
	Role   contracts.UserRole
}

type TokenPair struct {
	AccessToken      string
	RefreshToken     string
	TokenType        string
	AccessExpiresIn  int64
	RefreshExpiresIn int64
}

type TokenClaims struct {
	UserID    uuid.UUID
	Role      contracts.UserRole
	TokenType TokenType
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type LoginResult struct {
	User   Principal
	Tokens TokenPair
}

type UserRepository interface {
	Create(ctx context.Context, user User) error
	GetByID(ctx context.Context, id uuid.UUID) (User, error)
	GetByEmail(ctx context.Context, email string) (User, error)
}

type TokenManager interface {
	IssueTokenPair(user User) (TokenPair, error)
	Parse(token string, expectedType TokenType) (TokenClaims, error)
}

func isSupportedRole(role contracts.UserRole) bool {
	switch role {
	case contracts.RoleAdmin, contracts.RoleNormalUser:
		return true
	default:
		return false
	}
}

func isSupportedStatus(status contracts.UserStatus) bool {
	switch status {
	case contracts.UserStatusActive, contracts.UserStatusDisabled:
		return true
	default:
		return false
	}
}
