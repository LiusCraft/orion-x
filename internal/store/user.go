package store

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserStore struct{ db *gorm.DB }

func NewUserStore(db *gorm.DB) *UserStore { return &UserStore{db: db} }

// Create creates a user with email as the primary identity.
// username is optional; pass "" to auto-generate from email.
func (s *UserStore) Create(email, username, passwordHash, creator string) (*User, error) {
	if username == "" {
		username = emailToUsername(email)
	}
	u := &User{
		ID:           uuid.NewString(),
		Email:        email,
		Username:     username,
		PasswordHash: passwordHash,
		BaseModel:    BaseModel{Creator: creator},
	}
	if err := s.db.Create(u).Error; err != nil {
		return nil, fmt.Errorf("user store: create: %w", err)
	}
	return u, nil
}

// CreateWithGithub creates a user linked to a GitHub account.
// email is the primary identity; username is auto-derived if empty.
func (s *UserStore) CreateWithGithub(email, githubID, creator string) (*User, error) {
	username := emailToUsername(email)
	u := &User{
		ID:        uuid.NewString(),
		Email:     email,
		Username:  username,
		GithubID:  githubID,
		BaseModel: BaseModel{Creator: creator},
	}
	if err := s.db.Create(u).Error; err != nil {
		return nil, fmt.Errorf("user store: create with github: %w", err)
	}
	return u, nil
}

// GetByEmail looks up a user by email (the primary identity).
func (s *UserStore) GetByEmail(email string) (*User, error) {
	var u User
	if err := s.db.Where("email = ?", email).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

// GetByUsername looks up a user by display name. Used for admin setup
// and legacy lookups.
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

func (s *UserStore) GetByGithubID(githubID string) (*User, error) {
	var u User
	if err := s.db.Where("github_id = ?", githubID).First(&u).Error; err != nil {
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

func (s *UserStore) UpdateGithubID(id, githubID string) error {
	return s.db.Model(&User{}).Where("id = ?", id).Update("github_id", githubID).Error
}

// emailToUsername derives a display name from an email address.
func emailToUsername(email string) string {
	// Take the part before @
	local, _, _ := strings.Cut(email, "@")

	// Sanitize to alphanumeric + underscore/dot/hyphen
	var clean []byte
	for _, c := range []byte(local) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '.' || c == '-' {
			clean = append(clean, c)
		}
	}
	name := string(clean)
	if name == "" {
		return "user"
	}
	// Truncate to 64 chars (GORM varchar limit)
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}
