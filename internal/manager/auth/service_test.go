package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liuscraft/orion-x/internal/manager/contracts"
)

type fakeUserRepository struct {
	byID    map[uuid.UUID]User
	byEmail map[string]User
}

func (f *fakeUserRepository) Create(_ context.Context, user User) error {
	normalizedEmail := strings.ToLower(strings.TrimSpace(user.Email))
	if normalizedEmail == "" {
		return ErrInvalidArgument
	}
	if _, ok := f.byEmail[normalizedEmail]; ok {
		return ErrConflict
	}
	if f.byID == nil {
		f.byID = make(map[uuid.UUID]User)
	}
	if f.byEmail == nil {
		f.byEmail = make(map[string]User)
	}

	user.Email = normalizedEmail
	f.byID[user.ID] = user
	f.byEmail[normalizedEmail] = user
	return nil
}

func (f *fakeUserRepository) GetByID(_ context.Context, id uuid.UUID) (User, error) {
	user, ok := f.byID[id]
	if !ok {
		return User{}, ErrUserNotFound
	}
	return user, nil
}

func (f *fakeUserRepository) GetByEmail(_ context.Context, email string) (User, error) {
	user, ok := f.byEmail[strings.ToLower(strings.TrimSpace(email))]
	if !ok {
		return User{}, ErrUserNotFound
	}
	return user, nil
}

func TestService_LoginSuccess(t *testing.T) {
	passwordHash, err := HashPassword("P@ssw0rd")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	userID := uuid.New()
	user := User{
		ID:           userID,
		Email:        "admin@example.com",
		PasswordHash: passwordHash,
		Role:         contracts.RoleAdmin,
		Status:       contracts.UserStatusActive,
	}

	repo := &fakeUserRepository{
		byID:    map[uuid.UUID]User{userID: user},
		byEmail: map[string]User{"admin@example.com": user},
	}
	tokens, err := NewJWTManager(JWTManagerConfig{
		Secret:     "unit-test-secret",
		Issuer:     "unit-test",
		AccessTTL:  5 * time.Minute,
		RefreshTTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	service := NewService(repo, tokens)
	result, err := service.Login(context.Background(), "admin@example.com", "P@ssw0rd")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	if result.User.UserID != userID {
		t.Fatalf("expected user id %s, got %s", userID, result.User.UserID)
	}
	if result.User.Role != contracts.RoleAdmin {
		t.Fatalf("expected role admin, got %s", result.User.Role)
	}
	if result.Tokens.AccessToken == "" || result.Tokens.RefreshToken == "" {
		t.Fatalf("expected non-empty tokens")
	}
}

func TestService_LoginInvalidCredentials(t *testing.T) {
	passwordHash, err := HashPassword("P@ssw0rd")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	userID := uuid.New()
	user := User{
		ID:           userID,
		Email:        "user@example.com",
		PasswordHash: passwordHash,
		Role:         contracts.RoleNormalUser,
		Status:       contracts.UserStatusActive,
	}

	repo := &fakeUserRepository{
		byID:    map[uuid.UUID]User{userID: user},
		byEmail: map[string]User{"user@example.com": user},
	}
	tokens, err := NewJWTManager(JWTManagerConfig{
		Secret:     "unit-test-secret",
		Issuer:     "unit-test",
		AccessTTL:  5 * time.Minute,
		RefreshTTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	service := NewService(repo, tokens)
	_, err = service.Login(context.Background(), "user@example.com", "wrong-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestService_LoginDisabledUser(t *testing.T) {
	passwordHash, err := HashPassword("P@ssw0rd")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	userID := uuid.New()
	user := User{
		ID:           userID,
		Email:        "disabled@example.com",
		PasswordHash: passwordHash,
		Role:         contracts.RoleNormalUser,
		Status:       contracts.UserStatusDisabled,
	}

	repo := &fakeUserRepository{
		byID:    map[uuid.UUID]User{userID: user},
		byEmail: map[string]User{"disabled@example.com": user},
	}
	tokens, err := NewJWTManager(JWTManagerConfig{
		Secret:     "unit-test-secret",
		Issuer:     "unit-test",
		AccessTTL:  5 * time.Minute,
		RefreshTTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	service := NewService(repo, tokens)
	_, err = service.Login(context.Background(), "disabled@example.com", "P@ssw0rd")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_RefreshSuccess(t *testing.T) {
	passwordHash, err := HashPassword("P@ssw0rd")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	userID := uuid.New()
	user := User{
		ID:           userID,
		Email:        "refresh@example.com",
		PasswordHash: passwordHash,
		Role:         contracts.RoleNormalUser,
		Status:       contracts.UserStatusActive,
	}

	repo := &fakeUserRepository{
		byID:    map[uuid.UUID]User{userID: user},
		byEmail: map[string]User{"refresh@example.com": user},
	}
	tokens, err := NewJWTManager(JWTManagerConfig{
		Secret:     "unit-test-secret",
		Issuer:     "unit-test",
		AccessTTL:  5 * time.Minute,
		RefreshTTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	service := NewService(repo, tokens)
	loginResult, err := service.Login(context.Background(), "refresh@example.com", "P@ssw0rd")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	refreshResult, err := service.Refresh(context.Background(), loginResult.Tokens.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if refreshResult.Tokens.AccessToken == "" || refreshResult.Tokens.RefreshToken == "" {
		t.Fatalf("expected non-empty tokens after refresh")
	}
}

func TestService_AuthenticateDisabledUser(t *testing.T) {
	passwordHash, err := HashPassword("P@ssw0rd")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	userID := uuid.New()
	user := User{
		ID:           userID,
		Email:        "normal@example.com",
		PasswordHash: passwordHash,
		Role:         contracts.RoleNormalUser,
		Status:       contracts.UserStatusActive,
	}

	repo := &fakeUserRepository{
		byID:    map[uuid.UUID]User{userID: user},
		byEmail: map[string]User{"normal@example.com": user},
	}
	tokens, err := NewJWTManager(JWTManagerConfig{
		Secret:     "unit-test-secret",
		Issuer:     "unit-test",
		AccessTTL:  5 * time.Minute,
		RefreshTTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	service := NewService(repo, tokens)
	loginResult, err := service.Login(context.Background(), "normal@example.com", "P@ssw0rd")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	disabledUser := user
	disabledUser.Status = contracts.UserStatusDisabled
	repo.byID[userID] = disabledUser
	repo.byEmail["normal@example.com"] = disabledUser

	_, err = service.Authenticate(context.Background(), loginResult.Tokens.AccessToken)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_RegisterSuccess(t *testing.T) {
	repo := &fakeUserRepository{
		byID:    make(map[uuid.UUID]User),
		byEmail: make(map[string]User),
	}
	tokens, err := NewJWTManager(JWTManagerConfig{
		Secret:     "unit-test-secret",
		Issuer:     "unit-test",
		AccessTTL:  5 * time.Minute,
		RefreshTTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	service := NewService(repo, tokens)
	result, err := service.Register(context.Background(), "NewUser@Example.COM ", "P@ssw0rd")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if result.User.UserID == uuid.Nil {
		t.Fatalf("expected non-nil user id")
	}
	if result.User.Email != "newuser@example.com" {
		t.Fatalf("expected normalized email, got %q", result.User.Email)
	}
	if result.User.Role != contracts.RoleNormalUser {
		t.Fatalf("expected role normal_user, got %s", result.User.Role)
	}
	if result.Tokens.AccessToken == "" || result.Tokens.RefreshToken == "" {
		t.Fatalf("expected non-empty tokens")
	}

	stored, ok := repo.byEmail["newuser@example.com"]
	if !ok {
		t.Fatalf("expected registered user to be persisted")
	}
	if stored.PasswordHash == "" || stored.PasswordHash == "P@ssw0rd" {
		t.Fatalf("expected hashed password in repository")
	}
}

func TestService_RegisterConflict(t *testing.T) {
	passwordHash, err := HashPassword("P@ssw0rd")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	existingID := uuid.New()
	repo := &fakeUserRepository{
		byID: map[uuid.UUID]User{
			existingID: {
				ID:           existingID,
				Email:        "exists@example.com",
				PasswordHash: passwordHash,
				Role:         contracts.RoleNormalUser,
				Status:       contracts.UserStatusActive,
			},
		},
		byEmail: map[string]User{},
	}
	repo.byEmail["exists@example.com"] = repo.byID[existingID]

	tokens, err := NewJWTManager(JWTManagerConfig{
		Secret:     "unit-test-secret",
		Issuer:     "unit-test",
		AccessTTL:  5 * time.Minute,
		RefreshTTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	service := NewService(repo, tokens)
	_, err = service.Register(context.Background(), " EXISTS@example.com", "another-pass")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestService_Reauthenticate(t *testing.T) {
	passwordHash, err := HashPassword("P@ssw0rd")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	userID := uuid.New()
	user := User{
		ID:           userID,
		Email:        "reauth@example.com",
		PasswordHash: passwordHash,
		Role:         contracts.RoleAdmin,
		Status:       contracts.UserStatusActive,
	}

	repo := &fakeUserRepository{
		byID:    map[uuid.UUID]User{userID: user},
		byEmail: map[string]User{"reauth@example.com": user},
	}
	tokens, err := NewJWTManager(JWTManagerConfig{
		Secret:     "unit-test-secret",
		Issuer:     "unit-test",
		AccessTTL:  5 * time.Minute,
		RefreshTTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	service := NewService(repo, tokens)

	if err := service.Reauthenticate(context.Background(), userID, "P@ssw0rd"); err != nil {
		t.Fatalf("Reauthenticate() error = %v", err)
	}

	if err := service.Reauthenticate(context.Background(), userID, "bad-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}
