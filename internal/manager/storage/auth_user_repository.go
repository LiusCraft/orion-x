package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/auth"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type authUserRepository struct {
	db *gorm.DB
}

func NewAuthUserRepository(db *gorm.DB) auth.UserRepository {
	return &authUserRepository{db: db}
}

func (r *authUserRepository) Create(ctx context.Context, user auth.User) error {
	if user.ID == uuid.Nil {
		return fmt.Errorf("%w: user id is required", auth.ErrInvalidArgument)
	}

	normalizedEmail := strings.ToLower(strings.TrimSpace(user.Email))
	if normalizedEmail == "" {
		return fmt.Errorf("%w: email is required", auth.ErrInvalidArgument)
	}
	if strings.TrimSpace(user.PasswordHash) == "" {
		return fmt.Errorf("%w: password hash is required", auth.ErrInvalidArgument)
	}

	now := time.Now().UTC()
	model := UserModel{
		ID:           user.ID,
		Email:        normalizedEmail,
		PasswordHash: user.PasswordHash,
		Role:         string(user.Role),
		Status:       string(user.Status),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "email"}}, DoNothing: true}).
		Create(&model)
	if result.Error != nil {
		return fmt.Errorf("create user %s: %w", normalizedEmail, result.Error)
	}
	if result.RowsAffected == 0 {
		return auth.ErrConflict
	}

	return nil
}

func (r *authUserRepository) GetByID(ctx context.Context, id uuid.UUID) (auth.User, error) {
	if id == uuid.Nil {
		return auth.User{}, fmt.Errorf("%w: user id is required", auth.ErrInvalidArgument)
	}

	var model UserModel
	err := r.db.WithContext(ctx).Take(&model, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return auth.User{}, auth.ErrUserNotFound
		}
		return auth.User{}, fmt.Errorf("query user by id %s: %w", id.String(), err)
	}

	return mapUserModel(model), nil
}

func (r *authUserRepository) GetByEmail(ctx context.Context, email string) (auth.User, error) {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if normalizedEmail == "" {
		return auth.User{}, fmt.Errorf("%w: email is required", auth.ErrInvalidArgument)
	}

	var model UserModel
	err := r.db.WithContext(ctx).Take(&model, "email = ?", normalizedEmail).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return auth.User{}, auth.ErrUserNotFound
		}
		return auth.User{}, fmt.Errorf("query user by email %s: %w", normalizedEmail, err)
	}

	return mapUserModel(model), nil
}

func mapUserModel(model UserModel) auth.User {
	return auth.User{
		ID:           model.ID,
		Email:        model.Email,
		PasswordHash: model.PasswordHash,
		Role:         contracts.UserRole(model.Role),
		Status:       contracts.UserStatus(model.Status),
	}
}
