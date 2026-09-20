package store

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Asset 一次上传产生的资源记录：对象存储里的对象 + 元数据。
type Asset struct {
	ID        string `gorm:"primaryKey;type:varchar(36)" json:"id"`
	OwnerID   string `gorm:"not null;index;type:varchar(36)" json:"owner_id"`
	Purpose   string `gorm:"not null;index;type:varchar(32)" json:"purpose"`  // image | voice:sample | kb:document
	Name      string `gorm:"not null;type:varchar(256)" json:"name"`          // 原始文件名，仅展示
	ObjectKey string `gorm:"not null;uniqueIndex;type:varchar(512)" json:"-"` // 对象存储键
	MimeType  string `gorm:"type:varchar(128)" json:"mime_type"`
	Size      int64  `gorm:"not null;default:0" json:"size"`
	BaseModel
}

func (Asset) TableName() string { return "assets" }

// AssetStore 资源元数据持久化。
type AssetStore struct{ db *gorm.DB }

func NewAssetStore(db *gorm.DB) *AssetStore { return &AssetStore{db: db} }

func (s *AssetStore) Create(a *Asset) error {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if err := s.db.Create(a).Error; err != nil {
		return fmt.Errorf("asset store: create: %w", err)
	}
	return nil
}

func (s *AssetStore) GetByID(id string) (*Asset, error) {
	var a Asset
	if err := s.db.First(&a, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

// List 按归属分页查询，可按用途与文件名筛选。nameQuery 中的 LIKE 通配符会被转义。
func (s *AssetStore) List(ownerID, purpose, nameQuery string, offset, limit int) ([]Asset, int64, error) {
	scope := assetScope(ownerID, purpose, nameQuery)

	var total int64
	if err := s.db.Model(&Asset{}).Scopes(scope).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("asset store: count: %w", err)
	}

	var list []Asset
	if err := s.db.Model(&Asset{}).Scopes(scope).
		Order("created_at DESC").Offset(offset).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("asset store: list: %w", err)
	}
	return list, total, nil
}

func (s *AssetStore) DeleteByID(id string) error {
	if err := s.db.Delete(&Asset{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("asset store: delete: %w", err)
	}
	return nil
}

func assetScope(ownerID, purpose, nameQuery string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		db = db.Where("owner_id = ?", ownerID)
		if purpose != "" {
			db = db.Where("purpose = ?", purpose)
		}
		if nameQuery != "" {
			db = db.Where("name ILIKE ?", "%"+escapeLike(nameQuery)+"%")
		}
		return db
	}
}

// escapeLike 转义 LIKE 模式里的通配符，避免用户输入里的 % / _ 变成通配。
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	return strings.ReplaceAll(s, "_", `\_`)
}
