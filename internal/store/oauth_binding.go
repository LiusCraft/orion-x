package store

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OAuthBindingStore 管理用户与第三方 OAuth 平台的绑定关系。
type OAuthBindingStore struct{ db *gorm.DB }

func NewOAuthBindingStore(db *gorm.DB) *OAuthBindingStore { return &OAuthBindingStore{db: db} }

// Bind 绑定用户与平台账号；同一 (user_id, provider) 已存在时更新 provider_uid。
func (s *OAuthBindingStore) Bind(userID, provider, providerUID, creator string) error {
	b := &OAuthBinding{
		UserID:      userID,
		Provider:    provider,
		ProviderUID: providerUID,
		Creator:     creator,
	}
	err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "provider"}},
		DoUpdates: clause.AssignmentColumns([]string{"provider_uid"}),
	}).Create(b).Error
	if err != nil {
		return fmt.Errorf("oauth binding store: bind: %w", err)
	}
	return nil
}

// Unbind 解除用户与平台的绑定。
func (s *OAuthBindingStore) Unbind(userID, provider string) error {
	if err := s.db.Where("user_id = ? AND provider = ?", userID, provider).Delete(&OAuthBinding{}).Error; err != nil {
		return fmt.Errorf("oauth binding store: unbind: %w", err)
	}
	return nil
}

// GetByUserAndProvider 查询用户的某个平台绑定。
func (s *OAuthBindingStore) GetByUserAndProvider(userID, provider string) (*OAuthBinding, error) {
	var b OAuthBinding
	if err := s.db.Where("user_id = ? AND provider = ?", userID, provider).First(&b).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("oauth binding store: get by user: %w", err)
	}
	return &b, nil
}

// GetByProviderAndUID 按平台 UID 查询绑定（用于 OAuth 回调登录）。
func (s *OAuthBindingStore) GetByProviderAndUID(provider, providerUID string) (*OAuthBinding, error) {
	var b OAuthBinding
	if err := s.db.Where("provider = ? AND provider_uid = ?", provider, providerUID).First(&b).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("oauth binding store: get by uid: %w", err)
	}
	return &b, nil
}

// ListByUser 返回用户全部平台绑定。
func (s *OAuthBindingStore) ListByUser(userID string) ([]OAuthBinding, error) {
	var binds []OAuthBinding
	if err := s.db.Where("user_id = ?", userID).Order("provider").Find(&binds).Error; err != nil {
		return nil, fmt.Errorf("oauth binding store: list: %w", err)
	}
	return binds, nil
}

// CountByUser 返回用户的绑定数量。
func (s *OAuthBindingStore) CountByUser(userID string) (int64, error) {
	var n int64
	if err := s.db.Model(&OAuthBinding{}).Where("user_id = ?", userID).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("oauth binding store: count: %w", err)
	}
	return n, nil
}
