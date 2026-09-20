package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 计费仓储：9 张 billing_* 表的读写入口（docs/billing-design.md §4 / §19）。
//
// 只有 internal/billing/service 该 import 这个文件里的东西；计费的领域包
// （internal/billing）零依赖，不认识 store，所以这里的字段全是字符串 / 整数 / JSON，
// domain ↔ row 的转换在 service 里做。
//
// 写钱的规矩（§19）：billing_accounts / billing_ledger / billing_grants /
// billing_usage_events 的写入口只有计费自己的接口，订单、管理端、业务模块都不许直接写。

// 列表查询的默认与上限 limit：调用方传 0 表示“用默认值，别全表扫”。
const (
	defaultBillingQueryLimit = 100
	maxBillingQueryLimit     = 1000
)

// BillingStore 计费表的仓储。账户 / 流水 / 用量 / 预冻结的方法也挂在这个类型上，
// 分别定义在 billing_account.go / billing_usage.go / billing_reservation.go。
type BillingStore struct{ db *gorm.DB }

func NewBillingStore(db *gorm.DB) *BillingStore { return &BillingStore{db: db} }

// DB 暴露底层连接，供调用方开事务（结算事务由 service 组装，仓储只提供步骤）。
func (s *BillingStore) DB() *gorm.DB { return s.db }

// normalizeLimit 把调用方给的 limit 收进 [1, maxBillingQueryLimit]：<= 0 用默认值，
// 免得“忘了传 limit”变成全表扫。
func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultBillingQueryLimit
	}
	if limit > maxBillingQueryLimit {
		return maxBillingQueryLimit
	}
	return limit
}

// dbOr 返回事务句柄；tx 为 nil 时退回连接池。所有“结算事务里会用到的”方法都走它，
// 这样同一个方法既能独立调用，也能参与外部事务。
func (s *BillingStore) dbOr(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return s.db
}

// ---------------------------------------------------------------- 计费项目录

// UpsertItems 按 code 幂等写入计费项：新行按入参落库，老行只更新名称 / 模式 / 单位 /
// 计量点 / is_system，**不覆盖 enabled**（管理员在管理端关掉的项，重启后不会被 seed 打开）。
func (s *BillingStore) UpsertItems(items []BillingItem) error {
	if len(items) == 0 {
		return nil
	}
	err := s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "code"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "charge_mode", "unit", "meter_source", "is_system", "updated_at",
		}),
	}).CreateInBatches(items, 50).Error
	if err != nil {
		return fmt.Errorf("billing store: upsert items: %w", err)
	}
	return nil
}

// ListItems 返回全部计费项，按 code 排序（目录是开放集合，全量返回）。
func (s *BillingStore) ListItems() ([]BillingItem, error) {
	var items []BillingItem
	if err := s.db.Order("code ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("billing store: list items: %w", err)
	}
	return items, nil
}

// GetItem 按 code 查计费项，未找到返回 ErrNotFound。
func (s *BillingStore) GetItem(code string) (*BillingItem, error) {
	var item BillingItem
	if err := s.db.Where("code = ?", code).Take(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("billing store: get item: %w", err)
	}
	return &item, nil
}

// SetItemEnabled 开关一个计费项（口径互斥的两组项就靠它只启用一组）。code 不存在返回 ErrNotFound。
func (s *BillingStore) SetItemEnabled(code string, enabled bool) error {
	res := s.db.Model(&BillingItem{}).Where("code = ?", code).Update("enabled", enabled)
	if res.Error != nil {
		return fmt.Errorf("billing store: set item enabled: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------- 价格

// PriceQuery 是价格列表的过滤条件。
//
// PlatformOnly 只看平台标准价（account_id = ”），AccountID 只看某个账户的协议价，
// 两个都不给就是全量；两个都给时 PlatformOnly 优先。
type PriceQuery struct {
	ItemCode     string
	AccountID    string
	PlatformOnly bool
	ResourceType string
	ResourceID   string
	ActiveAt     *time.Time // 只看这一刻生效的价格
	Limit        int
	Offset       int
}

// CreatePrice 新增一版价格。ID 为空时自动生成；同一 scope 同一 effective_from 重复写入
// 会撞唯一索引失败——那不是错误，是调用方该决定“改价还是加版本”。
func (s *BillingStore) CreatePrice(p *BillingPrice) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if err := s.db.Create(p).Error; err != nil {
		return fmt.Errorf("billing store: create price: %w", err)
	}
	return nil
}

// GetPrice 按 ID 查价格，未找到返回 ErrNotFound。
func (s *BillingStore) GetPrice(id string) (*BillingPrice, error) {
	var p BillingPrice
	if err := s.db.Where("id = ?", id).Take(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("billing store: get price: %w", err)
	}
	return &p, nil
}

// UpdatePrice 局部更新一版价格并返回最新值。停用一版已生效的价格就是把 effective_to
// 收到当前时刻（不删行，历史账单还指着它）；取消一版还没生效的价格直接 DeletePrice。
func (s *BillingStore) UpdatePrice(id string, updates map[string]any) (*BillingPrice, error) {
	res := s.db.Model(&BillingPrice{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return nil, fmt.Errorf("billing store: update price: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return s.GetPrice(id)
}

// DeletePrice 删除一版价格。只该删“还没生效过”的价格（它没匹配过事件、也没被流水引用）。
func (s *BillingStore) DeletePrice(id string) error {
	res := s.db.Delete(&BillingPrice{}, "id = ?", id)
	if res.Error != nil {
		return fmt.Errorf("billing store: delete price: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPrices 按过滤条件分页返回价格，最新的版本在前。
func (s *BillingStore) ListPrices(q PriceQuery) ([]BillingPrice, error) {
	var prices []BillingPrice
	err := s.db.Model(&BillingPrice{}).Scopes(priceScope(q)).
		Order("effective_from DESC").Order("id DESC").
		Offset(q.Offset).Limit(normalizeLimit(q.Limit)).
		Find(&prices).Error
	if err != nil {
		return nil, fmt.Errorf("billing store: list prices: %w", err)
	}
	return prices, nil
}

func priceScope(q PriceQuery) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if q.ItemCode != "" {
			db = db.Where("item_code = ?", q.ItemCode)
		}
		switch {
		case q.PlatformOnly:
			db = db.Where("account_id = ''")
		case q.AccountID != "":
			db = db.Where("account_id = ?", q.AccountID)
		}
		if q.ResourceType != "" {
			db = db.Where("resource_type = ?", q.ResourceType)
		}
		if q.ResourceID != "" {
			db = db.Where("resource_id = ?", q.ResourceID)
		}
		if q.ActiveAt != nil {
			db = db.Where("effective_from <= ? AND (effective_to IS NULL OR effective_to > ?)", *q.ActiveAt, *q.ActiveAt)
		}
		return db
	}
}

// FindPriceCandidates 返回某一时刻生效的候选价格，顺序即 §3.2 的优先级：
//
//	账户协议价优先 → 资源越具体越优先（voice > model > provider > item）→ effective_from 最新
//
// refs 是 (resource_type, resource_id) 组合（由 billing.ResourceRef.Pairs() 生成）；
// refs 为空时只匹配 item 级价格。多条候选谁最终胜出由 billing.Select 决定——SQL 的
// ORDER BY 只是同一套规则的镜像，服务层拿到候选后仍应再过一次 Select。
//
// 匹配必须按事件的 occurred_at，而不是 now：异步结算会晚几秒到几分钟，跨调价时刻时
// 必须用事件发生时的价格（§3.2）。
func (s *BillingStore) FindPriceCandidates(itemCode, accountID string, refs [][2]string, at time.Time, limit int) ([]BillingPrice, error) {
	q := s.db.Model(&BillingPrice{}).Where("item_code = ?", itemCode)
	if accountID == "" {
		q = q.Where("account_id = ''")
	} else {
		q = q.Where("account_id IN ?", []string{"", accountID})
	}

	// 资源维度是「择一停在某一级」：item 级兜底，加上快照里所有非空的 (粒度, ID) 组合。
	// 组合交给 GORM 里的子条件包一层括号，别让 OR 漏到外层的 AND 上。
	scope := s.db.Where("resource_type = ?", "item")
	if len(refs) > 0 {
		scope = scope.Or("(resource_type, resource_id) IN ?", refs)
	}
	q = q.Where(scope)

	q = q.Where("effective_from <= ? AND (effective_to IS NULL OR effective_to > ?)", at, at)

	var prices []BillingPrice
	err := q.
		Order("(account_id <> '') DESC").
		Order("CASE resource_type WHEN 'voice' THEN 3 WHEN 'model' THEN 2 WHEN 'provider' THEN 1 ELSE 0 END DESC").
		Order("effective_from DESC").
		Order("id DESC").
		Limit(normalizeLimit(limit)).
		Find(&prices).Error
	if err != nil {
		return nil, fmt.Errorf("billing store: find price candidates: %w", err)
	}
	return prices, nil
}
