package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// APIKey 用户自助创建的 API Key（docs/api-key-design.md §5.4）。
//
// 这里只存摘要（hash）与公开段（lookup）：明文只在创建响应里出现一次，库与日志里
// 再没有第二份。字段的语义与约束见设计文档的同名表。
type APIKey struct {
	ID     string `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserID string `gorm:"not null;index;type:varchar(36)" json:"user_id"`
	Name   string `gorm:"not null;type:varchar(64)" json:"name"`
	// Lookup 是 key 串里的公开段，不是秘密：它只用于 O(1) 查库，撤销后不复用。
	Lookup string `gorm:"not null;uniqueIndex;type:varchar(16)" json:"-"`
	// Hash 是 sha256(完整串) 的 hex，唯一索引是碰撞的兜底。
	Hash   string         `gorm:"not null;uniqueIndex;type:varchar(64)" json:"-"`
	Scopes pq.StringArray `gorm:"type:text[];not null" json:"scopes"`
	// ExpiresAt 为空 = 不过期；RevokedAt 非空 = 终态，不可恢复。
	ExpiresAt  *time.Time `gorm:"type:timestamptz" json:"expires_at,omitempty"`
	RevokedAt  *time.Time `gorm:"type:timestamptz" json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `gorm:"type:timestamptz" json:"last_used_at,omitempty"`
	// CallCount 异步刷、非精确：多副本下由各副本追加增量，不允许对账时引用（§3.2）。
	CallCount int64 `gorm:"not null;default:0" json:"call_count"`
	BaseModel
}

func (APIKey) TableName() string { return "api_keys" }

// APIKeyStore 是 api_keys 表的仓储：拿到什么存什么，不做校验也不判权限。
type APIKeyStore struct{ db *gorm.DB }

func NewAPIKeyStore(db *gorm.DB) *APIKeyStore { return &APIKeyStore{db: db} }

func (s *APIKeyStore) Create(ctx context.Context, k *APIKey) error {
	if err := s.db.WithContext(ctx).Create(k).Error; err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("api key store: create: %w", ErrDuplicate)
		}
		return fmt.Errorf("api key store: create: %w", err)
	}
	return nil
}

// GetByLookup 是校验热路径上唯一的一次查询：按唯一索引点查。
func (s *APIKeyStore) GetByLookup(ctx context.Context, lookup string) (*APIKey, error) {
	var k APIKey
	if err := s.db.WithContext(ctx).First(&k, "lookup = ?", lookup).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("api key store: get by lookup: %w", err)
	}
	return &k, nil
}

func (s *APIKeyStore) ListByUser(ctx context.Context, userID string) ([]APIKey, error) {
	var list []APIKey
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("api key store: list: %w", err)
	}
	return list, nil
}

func (s *APIKeyStore) CountByUser(ctx context.Context, userID string) (int64, error) {
	var n int64
	if err := s.db.WithContext(ctx).Model(&APIKey{}).Where("user_id = ?", userID).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("api key store: count: %w", err)
	}
	return n, nil
}

// Revoke 把 key 置为已撤销（软删）。归属不符与不存在一律 ErrNotFound——两者不做区分，
// 免得接口变成“探测别人有没有这把 key”的工具。已撤销的行是终态，重复调用直接成功。
func (s *APIKeyStore) Revoke(ctx context.Context, id, userID string, at time.Time) error {
	var k APIKey
	if err := s.db.WithContext(ctx).First(&k, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("api key store: revoke lookup: %w", err)
	}
	if k.RevokedAt != nil {
		return nil
	}
	err := s.db.WithContext(ctx).Model(&APIKey{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Updates(map[string]any{"revoked_at": at, "updated_at": time.Now()}).Error
	if err != nil {
		return fmt.Errorf("api key store: revoke: %w", err)
	}
	return nil
}

// AddUsage 累加用量：call_count 写增量、last_used_at 写最大（§5.4）。
// 绝对值写法在单副本下看不出差别，但多副本时会互相覆盖——增量写法扩副本不用改代码。
//
// 用 UpdateColumns 而不是 Updates：计数是热路径上的副作用，不该顺手把 updated_at
// 刷成当前时间（那会把"配置变更时间"变成一个每 30 秒抖一次的字段）。
func (s *APIKeyStore) AddUsage(ctx context.Context, id string, calls int64, lastUsed time.Time) error {
	err := s.db.WithContext(ctx).Model(&APIKey{}).Where("id = ?", id).
		UpdateColumns(map[string]any{
			"call_count":   gorm.Expr("call_count + ?", calls),
			"last_used_at": gorm.Expr("GREATEST(COALESCE(last_used_at, to_timestamp(0)), ?)", lastUsed),
		}).Error
	if err != nil {
		return fmt.Errorf("api key store: add usage: %w", err)
	}
	return nil
}

// isUniqueViolation 靠错误文本认 PG 的唯一索引冲突（SQLSTATE 23505）。
// GORM 自带的 gorm.ErrDuplicatedKey 要求打开 TranslateError（全局配置），
// 为了一个创建路径的错误码去改全局配置不划算。
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLSTATE 23505") ||
		strings.Contains(msg, "duplicate key value violates unique constraint")
}
