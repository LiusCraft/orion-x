package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// 账户 / 流水 / 赠送额度。写钱的路径（结算事务）在 billing_usage.go 的
// “结算事务的步骤”一节，这里只放查询与前缀写入口。

// AccountQuery 是账户列表的过滤条件。Keyword 同时匹配 subject_id 与 id（模糊）。
type AccountQuery struct {
	SubjectType string
	Status      string
	Keyword     string
	Limit       int
	Offset      int
}

// LedgerQuery 是流水列表的过滤条件。From/To 作用在 occurred_at 上（左闭右开）。
type LedgerQuery struct {
	AccountID string
	ItemCode  string
	Kind      string
	From      *time.Time
	To        *time.Time
	Limit     int
	Offset    int
}

// CreateAccount 新建账户。ID 为空时生成 uuid。
//
// 账户 ID 的对外形态（`acct_` 前缀）由调用方决定。注意列宽是 varchar(36)：
// `acct_` 再拼一个完整 uuid 是 41 字符，会被数据库拒掉，要拼就得截短 uuid 部分。
func (s *BillingStore) CreateAccount(a *BillingAccount) error {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if err := s.db.Create(a).Error; err != nil {
		return fmt.Errorf("billing store: create account: %w", err)
	}
	return nil
}

// GetAccount 按内部 ID 查账户，未找到返回 ErrNotFound。
func (s *BillingStore) GetAccount(id string) (*BillingAccount, error) {
	var a BillingAccount
	if err := s.db.Where("id = ?", id).Take(&a).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("billing store: get account: %w", err)
	}
	return &a, nil
}

// GetAccountBySubject 按计费主体查账户，未找到返回 ErrNotFound。
// subject_type 是 user | org，subject_id 是业务实体 ID。
func (s *BillingStore) GetAccountBySubject(subjectType, subjectID string) (*BillingAccount, error) {
	var a BillingAccount
	err := s.db.Where("subject_type = ? AND subject_id = ?", subjectType, subjectID).Take(&a).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("billing store: get account by subject: %w", err)
	}
	return &a, nil
}

// ListAccounts 按过滤条件分页返回账户，新建的在前。
func (s *BillingStore) ListAccounts(q AccountQuery) ([]BillingAccount, error) {
	var list []BillingAccount
	err := s.db.Model(&BillingAccount{}).Scopes(accountScope(q)).
		Order("created_at DESC").Order("id DESC").
		Offset(q.Offset).Limit(normalizeLimit(q.Limit)).
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("billing store: list accounts: %w", err)
	}
	return list, nil
}

// CountAccounts 返回符合过滤条件的账户总数（分页配套）。
func (s *BillingStore) CountAccounts(q AccountQuery) (int64, error) {
	var total int64
	if err := s.db.Model(&BillingAccount{}).Scopes(accountScope(q)).Count(&total).Error; err != nil {
		return 0, fmt.Errorf("billing store: count accounts: %w", err)
	}
	return total, nil
}

func accountScope(q AccountQuery) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if q.SubjectType != "" {
			db = db.Where("subject_type = ?", q.SubjectType)
		}
		if q.Status != "" {
			db = db.Where("status = ?", q.Status)
		}
		if q.Keyword != "" {
			like := "%" + escapeLike(q.Keyword) + "%"
			db = db.Where("subject_id ILIKE ? OR id ILIKE ?", like, like)
		}
		return db
	}
}

// ListLedger 按过滤条件分页返回流水，最新的在前。append-only 的表只查不改。
func (s *BillingStore) ListLedger(q LedgerQuery) ([]BillingLedger, error) {
	var list []BillingLedger
	err := s.db.Model(&BillingLedger{}).Scopes(ledgerScope(q)).
		Order("occurred_at DESC").Order("id DESC").
		Offset(q.Offset).Limit(normalizeLimit(q.Limit)).
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("billing store: list ledger: %w", err)
	}
	return list, nil
}

// CountLedger 返回符合过滤条件的流水条数（分页配套）。
func (s *BillingStore) CountLedger(q LedgerQuery) (int64, error) {
	var total int64
	if err := s.db.Model(&BillingLedger{}).Scopes(ledgerScope(q)).Count(&total).Error; err != nil {
		return 0, fmt.Errorf("billing store: count ledger: %w", err)
	}
	return total, nil
}

// LedgerExists 判断某个幂等键是不是已经入过账了。
//
// 入账方（充值、外部业务模块）在调 Credit 之前用它短路重放：Credit 撞上
// idempotency_key 唯一索引会返回错误，而「已经入过账」不是失败——主动查一次比
// 去解析驱动层的唯一键错误码可靠。
func (s *BillingStore) LedgerExists(idempotencyKey string) (bool, error) {
	var count int64
	err := s.db.Model(&BillingLedger{}).Where("idempotency_key = ?", idempotencyKey).Limit(1).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("billing store: ledger exists: %w", err)
	}
	return count > 0, nil
}

// ExpiredGrants 返回已过期但还没作废过的赠款：作废与否用 `expire:<grant_id>` 这条
// 流水判断，所以重复调用不会重复写流水（过期赠款不删行、也不改数，§12）。
func (s *BillingStore) ExpiredGrants(tx *gorm.DB, now time.Time) ([]BillingGrant, error) {
	var grants []BillingGrant
	err := s.dbOr(tx).Model(&BillingGrant{}).
		Where("expires_at < ? AND used_micro < granted_micro", now).
		Where("NOT EXISTS (SELECT 1 FROM billing_ledger WHERE idempotency_key = 'expire:' || billing_grants.id)").
		Order("expires_at ASC").Order("id ASC").
		Find(&grants).Error
	if err != nil {
		return nil, fmt.Errorf("billing store: expired grants: %w", err)
	}
	return grants, nil
}

func ledgerScope(q LedgerQuery) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if q.AccountID != "" {
			db = db.Where("account_id = ?", q.AccountID)
		}
		if q.ItemCode != "" {
			db = db.Where("item_code = ?", q.ItemCode)
		}
		if q.Kind != "" {
			db = db.Where("kind = ?", q.Kind)
		}
		if q.From != nil {
			db = db.Where("occurred_at >= ?", *q.From)
		}
		if q.To != nil {
			db = db.Where("occurred_at < ?", *q.To)
		}
		return db
	}
}

// ListGrants 返回账户在 at 时刻还能用的赠送额度，先到期的排前面（FEFO：先扣快过期的）。
// 只返回未过期且没用满的行。tx 允许为 nil（用连接池），结算事务里必须传 tx，
// 否则读到的额度会和同一事务里的扣减对不上。
func (s *BillingStore) ListGrants(tx *gorm.DB, accountID string, at time.Time) ([]BillingGrant, error) {
	var grants []BillingGrant
	err := s.dbOr(tx).Model(&BillingGrant{}).
		Where("account_id = ? AND expires_at > ? AND used_micro < granted_micro", accountID, at).
		Order("expires_at ASC").Order("id ASC").
		Find(&grants).Error
	if err != nil {
		return nil, fmt.Errorf("billing store: list grants: %w", err)
	}
	return grants, nil
}
