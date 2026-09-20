package store

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/logging"
)

// APIKey 用户自建的访问密钥。
//
// 明文只在创建响应里出现一次，之后只能靠“验证密码后再复制”取回：库里存摘要
// （KeyHash，认证走它）+ AES-GCM 密文（KeySecret，只在揭示时解开），另存前缀与
// 尾四位供列表页脱敏展示。密文与摘要并存是故意的：封装密钥缺失或轮换时，
// 已有密钥照旧能认证，只是复制不出来。
type APIKey struct {
	ID         string         `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name       string         `gorm:"not null;type:varchar(64)" json:"name"`
	OwnerID    string         `gorm:"not null;index;type:varchar(36)" json:"owner_id"`
	KeyHash    string         `gorm:"not null;uniqueIndex;type:varchar(64)" json:"-"`
	KeySecret  string         `gorm:"type:varchar(256)" json:"-"`
	KeyPrefix  string         `gorm:"not null;type:varchar(32)" json:"key_prefix"`
	KeyLast4   string         `gorm:"not null;type:varchar(8)" json:"key_last4"`
	Scopes     pq.StringArray `gorm:"type:text[]" json:"scopes"`
	CallCount  int64          `gorm:"not null;default:0" json:"call_count"`
	LastUsedAt *time.Time     `json:"last_used_at,omitempty"`
	BaseModel
}

type APIKeyStore struct {
	db *gorm.DB
	// boxKey 是 apikey.DeriveSecretKey 派生出来的封装密钥，空值时无法签发可复制的密钥。
	boxKey []byte
}

func NewAPIKeyStore(db *gorm.DB, boxKey []byte) *APIKeyStore {
	return &APIKeyStore{db: db, boxKey: boxKey}
}

// Create 生成并落库一个新密钥，返回记录与明文。明文只在这里出现一次，调用方负责
// 立刻回给用户；之后取回明文必须走 Reveal（且要求重新验证密码）。
func (s *APIKeyStore) Create(name, ownerID string, scopes []apikey.Scope, creator string) (*APIKey, string, error) {
	plain, err := apikey.Generate()
	if err != nil {
		return nil, "", err
	}
	prefix, last4, err := apikey.Display(plain)
	if err != nil {
		return nil, "", err
	}
	sealed, err := apikey.Seal(s.boxKey, plain)
	if err != nil {
		return nil, "", fmt.Errorf("api key store: seal: %w", err)
	}
	k := &APIKey{
		ID:        uuid.NewString(),
		Name:      name,
		OwnerID:   ownerID,
		KeyHash:   apikey.Hash(plain),
		KeySecret: sealed,
		KeyPrefix: prefix,
		KeyLast4:  last4,
		Scopes:    pq.StringArray(apikey.Strings(scopes)),
		BaseModel: BaseModel{Creator: creator},
	}
	if err := s.db.Create(k).Error; err != nil {
		return nil, "", fmt.Errorf("api key store: create: %w", err)
	}
	return k, plain, nil
}

// ListByOwner 按创建时间倒序返回某个用户的密钥。
func (s *APIKeyStore) ListByOwner(ownerID string) ([]APIKey, error) {
	var list []APIKey
	if err := s.db.Where("owner_id = ?", ownerID).
		Order("created_at DESC, id DESC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("api key store: list: %w", err)
	}
	return list, nil
}

// Delete 删除属于 ownerID 的密钥；不属于该用户（或不存在）时返回 ErrNotFound。
func (s *APIKeyStore) Delete(ownerID, id string) error {
	res := s.db.Where("id = ? AND owner_id = ?", id, ownerID).Delete(&APIKey{})
	if res.Error != nil {
		return fmt.Errorf("api key store: delete: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Reveal 解开某个属主密钥的明文，供控制台“验证密码后复制”使用。
// 解密失败与“旧版本签发的行没有密文”是两种不同的失败：前者说明主密钥变了，
// 后者说明这行本来就取不回来。
func (s *APIKeyStore) Reveal(ownerID, id string) (string, error) {
	var k APIKey
	if err := s.db.Where("id = ? AND owner_id = ?", id, ownerID).First(&k).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("api key store: reveal: %w", err)
	}
	if strings.TrimSpace(k.KeySecret) == "" {
		return "", apikey.ErrSecretUnavailable
	}
	return apikey.Open(s.boxKey, k.KeySecret)
}

// Authenticate 用明文密钥换出记录，并记一次调用统计。
func (s *APIKeyStore) Authenticate(plain string) (*APIKey, error) {
	if !apikey.LooksLike(plain) {
		return nil, ErrNotFound
	}
	var k APIKey
	if err := s.db.Where("key_hash = ?", apikey.Hash(plain)).First(&k).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("api key store: authenticate: %w", err)
	}
	s.touch(&k)
	return &k, nil
}

// touch 记录 last_used_at 与调用次数。统计不该影响可用性：写失败只记日志，
// 请求照常放行（下一次调用会把它补上）。
func (s *APIKeyStore) touch(k *APIKey) {
	now := time.Now()
	if err := s.db.Model(&APIKey{}).Where("id = ?", k.ID).Updates(map[string]any{
		"last_used_at": now,
		"call_count":   gorm.Expr("call_count + 1"),
	}).Error; err != nil {
		logging.Warnf("api key store: touch %s: %v", k.ID, err)
		return
	}
	k.LastUsedAt = &now
	k.CallCount++
}
