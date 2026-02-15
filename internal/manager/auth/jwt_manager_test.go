package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liuscraft/orion-x/internal/manager/contracts"
)

func TestJWTManager_IssueAndParseAccessToken(t *testing.T) {
	manager, err := NewJWTManager(JWTManagerConfig{
		Secret:     "jwt-test-secret",
		Issuer:     "jwt-test",
		AccessTTL:  2 * time.Minute,
		RefreshTTL: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	userID := uuid.New()
	tokens, err := manager.IssueTokenPair(User{
		ID:    userID,
		Role:  contracts.RoleAdmin,
		Email: "admin@example.com",
	})
	if err != nil {
		t.Fatalf("IssueTokenPair() error = %v", err)
	}

	claims, err := manager.Parse(tokens.AccessToken, TokenTypeAccess)
	if err != nil {
		t.Fatalf("Parse(access token) error = %v", err)
	}
	if claims.UserID != userID {
		t.Fatalf("expected user id %s, got %s", userID, claims.UserID)
	}
	if claims.Role != contracts.RoleAdmin {
		t.Fatalf("expected role admin, got %s", claims.Role)
	}
}

func TestJWTManager_ParseRejectsWrongTokenType(t *testing.T) {
	manager, err := NewJWTManager(JWTManagerConfig{
		Secret:     "jwt-test-secret",
		Issuer:     "jwt-test",
		AccessTTL:  2 * time.Minute,
		RefreshTTL: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	tokens, err := manager.IssueTokenPair(User{
		ID:   uuid.New(),
		Role: contracts.RoleNormalUser,
	})
	if err != nil {
		t.Fatalf("IssueTokenPair() error = %v", err)
	}

	_, err = manager.Parse(tokens.AccessToken, TokenTypeRefresh)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestJWTManager_ParseRejectsInvalidSignature(t *testing.T) {
	issuer := "jwt-test"

	issuerManager, err := NewJWTManager(JWTManagerConfig{
		Secret:     "issuer-secret",
		Issuer:     issuer,
		AccessTTL:  2 * time.Minute,
		RefreshTTL: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	validatorManager, err := NewJWTManager(JWTManagerConfig{
		Secret:     "validator-secret",
		Issuer:     issuer,
		AccessTTL:  2 * time.Minute,
		RefreshTTL: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	tokens, err := issuerManager.IssueTokenPair(User{
		ID:   uuid.New(),
		Role: contracts.RoleNormalUser,
	})
	if err != nil {
		t.Fatalf("IssueTokenPair() error = %v", err)
	}

	_, err = validatorManager.Parse(tokens.AccessToken, TokenTypeAccess)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}
