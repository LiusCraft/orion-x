package store

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserStore struct{ db *gorm.DB }

func NewUserStore(db *gorm.DB) *UserStore { return &UserStore{db: db} }

func (s *UserStore) Create(username, passwordHash, creator string) (*User, error) {
	u := &User{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: passwordHash,
		BaseModel:    BaseModel{Creator: creator},
	}
	if err := s.db.Create(u).Error; err != nil {
		return nil, fmt.Errorf("user store: create: %w", err)
	}
	return u, nil
}

func (s *UserStore) GetByUsername(username string) (*User, error) {
	var u User
	if err := s.db.Where("username = ?", username).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (s *UserStore) GetByID(id string) (*User, error) {
	var u User
	if err := s.db.First(&u, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (s *UserStore) UpdatePassword(id, newHash string) error {
	return s.db.Model(&User{}).Where("id = ?", id).Update("password_hash", newHash).Error
}

func (s *UserStore) UpdateEmail(id, email string) error {
	return s.db.Model(&User{}).Where("id = ?", id).Update("email", email).Error
}

func (s *UserStore) SetAdmin(id string, isAdmin bool) error {
	return s.db.Model(&User{}).Where("id = ?", id).Update("is_admin", isAdmin).Error
}
