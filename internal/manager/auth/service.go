package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/liuscraft/orion-x/internal/manager/contracts"
)

type Service struct {
	users  UserRepository
	tokens TokenManager
}

func NewService(users UserRepository, tokens TokenManager) *Service {
	return &Service{
		users:  users,
		tokens: tokens,
	}
}

func (s *Service) Login(ctx context.Context, email, password string) (LoginResult, error) {
	if err := s.validateReady(); err != nil {
		return LoginResult{}, err
	}

	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if normalizedEmail == "" || strings.TrimSpace(password) == "" {
		return LoginResult{}, fmt.Errorf("%w: email and password are required", ErrInvalidArgument)
	}

	user, err := s.users.GetByEmail(ctx, normalizedEmail)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return LoginResult{}, ErrInvalidCredentials
		}
		return LoginResult{}, fmt.Errorf("load user by email: %w", err)
	}

	if !isSupportedRole(user.Role) || !isSupportedStatus(user.Status) {
		return LoginResult{}, ErrUnauthorized
	}
	if user.Status != contracts.UserStatusActive {
		return LoginResult{}, ErrForbidden
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}

	tokenPair, err := s.tokens.IssueTokenPair(user)
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue token pair: %w", err)
	}

	return LoginResult{
		User: Principal{
			UserID: user.ID,
			Email:  user.Email,
			Role:   user.Role,
		},
		Tokens: tokenPair,
	}, nil
}

func (s *Service) Register(ctx context.Context, email, password string) (LoginResult, error) {
	if err := s.validateReady(); err != nil {
		return LoginResult{}, err
	}

	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if normalizedEmail == "" || strings.TrimSpace(password) == "" {
		return LoginResult{}, fmt.Errorf("%w: email and password are required", ErrInvalidArgument)
	}

	passwordHash, err := HashPassword(password)
	if err != nil {
		return LoginResult{}, err
	}

	userCount, err := s.users.Count(ctx)
	if err != nil {
		return LoginResult{}, fmt.Errorf("count users: %w", err)
	}

	role := contracts.RoleNormalUser
	if userCount == 0 {
		role = contracts.RoleAdmin
	}

	user := User{
		ID:           uuid.New(),
		Email:        normalizedEmail,
		PasswordHash: passwordHash,
		Role:         role,
		Status:       contracts.UserStatusActive,
	}

	if err := s.users.Create(ctx, user); err != nil {
		if errors.Is(err, ErrConflict) {
			return LoginResult{}, ErrConflict
		}
		return LoginResult{}, fmt.Errorf("create user: %w", err)
	}

	tokenPair, err := s.tokens.IssueTokenPair(user)
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue token pair: %w", err)
	}

	return LoginResult{
		User: Principal{
			UserID: user.ID,
			Email:  user.Email,
			Role:   user.Role,
		},
		Tokens: tokenPair,
	}, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (LoginResult, error) {
	if err := s.validateReady(); err != nil {
		return LoginResult{}, err
	}

	claims, err := s.tokens.Parse(refreshToken, TokenTypeRefresh)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return LoginResult{}, ErrUnauthorized
		}
		return LoginResult{}, fmt.Errorf("parse refresh token: %w", err)
	}

	user, err := s.users.GetByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return LoginResult{}, ErrUnauthorized
		}
		return LoginResult{}, fmt.Errorf("load user by id: %w", err)
	}
	if !isSupportedRole(user.Role) || !isSupportedStatus(user.Status) {
		return LoginResult{}, ErrUnauthorized
	}
	if user.Status != contracts.UserStatusActive {
		return LoginResult{}, ErrForbidden
	}

	tokenPair, err := s.tokens.IssueTokenPair(user)
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue token pair: %w", err)
	}

	return LoginResult{
		User: Principal{
			UserID: user.ID,
			Email:  user.Email,
			Role:   user.Role,
		},
		Tokens: tokenPair,
	}, nil
}

func (s *Service) Authenticate(ctx context.Context, accessToken string) (Principal, error) {
	if err := s.validateReady(); err != nil {
		return Principal{}, err
	}

	claims, err := s.tokens.Parse(accessToken, TokenTypeAccess)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return Principal{}, ErrUnauthorized
		}
		return Principal{}, fmt.Errorf("parse access token: %w", err)
	}

	user, err := s.users.GetByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return Principal{}, ErrUnauthorized
		}
		return Principal{}, fmt.Errorf("load user by id: %w", err)
	}
	if !isSupportedRole(user.Role) || !isSupportedStatus(user.Status) {
		return Principal{}, ErrUnauthorized
	}
	if user.Status != contracts.UserStatusActive {
		return Principal{}, ErrForbidden
	}

	return Principal{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
	}, nil
}

func (s *Service) Reauthenticate(ctx context.Context, userID uuid.UUID, password string) error {
	if err := s.validateReady(); err != nil {
		return err
	}
	if userID == uuid.Nil || strings.TrimSpace(password) == "" {
		return fmt.Errorf("%w: user_id and password are required", ErrInvalidArgument)
	}

	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return ErrUnauthorized
		}
		return fmt.Errorf("load user by id: %w", err)
	}

	if !isSupportedRole(user.Role) || !isSupportedStatus(user.Status) {
		return ErrUnauthorized
	}
	if user.Status != contracts.UserStatusActive {
		return ErrForbidden
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}

	return nil
}

func HashPassword(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", fmt.Errorf("%w: password is required", ErrInvalidArgument)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("generate bcrypt hash: %w", err)
	}
	return string(hash), nil
}

func (s *Service) validateReady() error {
	if s.users == nil || s.tokens == nil {
		return errors.New("auth service dependencies are not initialized")
	}
	return nil
}
