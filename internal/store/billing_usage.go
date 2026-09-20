package store

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 用量事件 / 账期累计量 / 日聚合，以及结算事务里用到的每一步（§5 / §16.2）。
//
// 用量事件是唯一的事实来源，也是结算队列（outbox 模式）：数据面批量上报只落库、
// 不做定价；worker 认领 pending 事件，按 occurred_at 匹配价格，在一个事务里走完
// “锁账户行 → 锁账期行 → 扣额度/余额 → 写流水 → 标记事件 → 更新日聚合”。
// 锁序固定下来，多个 worker 并发结算才不会互相咬住。

// 事件状态字面量。store 层不能 import internal/billing，这里与 billing.EventStatus*
// 保持字面一致（改一边要同步另一边）。
const (
	usageStatusPending = "pending"
	usageStatusCharged = "charged"
)

// usageEventInsertBatch 是批量插入的分批大小。
const usageEventInsertBatch = 200

// UsageQuery 是事件列表的过滤条件。From/To 作用在 occurred_at 上（左闭右开）。
type UsageQuery struct {
	AccountID string
	DeviceID  string
	SessionID string
	ItemCode  string
	Status    string
	From      *time.Time
	To        *time.Time
	Limit     int
	Offset    int
}

// UsageAggregate 是按计费项汇总的用量与金额。
type UsageAggregate struct {
	ItemCode    string `json:"item_code"`
	Quantity    int64  `json:"quantity"`
	AmountMicro int64  `json:"amount_micro"`
}

// InsertUsageEvents 批量写入用量事件，主键冲突（同一条事件重发）直接跳过。
// 返回真正插入的条数，调用方用 len(events) - inserted 得到 duplicated（正常现象，不告警）。
//
// 上报路径不做定价：这里只落事实，定价是 worker 的事（§14.4）。
func (s *BillingStore) InsertUsageEvents(events []BillingUsageEvent) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}
	res := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoNothing: true,
	}).CreateInBatches(events, usageEventInsertBatch)
	if res.Error != nil {
		return 0, fmt.Errorf("billing store: insert usage events: %w", res.Error)
	}
	return int(res.RowsAffected), nil
}

// ListUsageEvents 按过滤条件分页返回用量事件，最新的在前。
func (s *BillingStore) ListUsageEvents(q UsageQuery) ([]BillingUsageEvent, error) {
	var events []BillingUsageEvent
	err := s.db.Model(&BillingUsageEvent{}).Scopes(usageScope(q)).
		Order("occurred_at DESC").Order("id DESC").
		Offset(q.Offset).Limit(normalizeLimit(q.Limit)).
		Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("billing store: list usage events: %w", err)
	}
	return events, nil
}

// CountUsageEvents 返回符合过滤条件的事件条数（分页配套）。
func (s *BillingStore) CountUsageEvents(q UsageQuery) (int64, error) {
	var total int64
	if err := s.db.Model(&BillingUsageEvent{}).Scopes(usageScope(q)).Count(&total).Error; err != nil {
		return 0, fmt.Errorf("billing store: count usage events: %w", err)
	}
	return total, nil
}

func usageScope(q UsageQuery) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if q.AccountID != "" {
			db = db.Where("account_id = ?", q.AccountID)
		}
		if q.DeviceID != "" {
			db = db.Where("device_id = ?", q.DeviceID)
		}
		if q.SessionID != "" {
			db = db.Where("session_id = ?", q.SessionID)
		}
		if q.ItemCode != "" {
			db = db.Where("item_code = ?", q.ItemCode)
		}
		if q.Status != "" {
			db = db.Where("status = ?", q.Status)
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

// SumUsageByItem 按计费项汇总某个时间段内已计费（charged）事件的用量与金额。
// 时间区间是 [from, to)，作用在 occurred_at 上。
func (s *BillingStore) SumUsageByItem(accountID string, from, to time.Time, limit int) ([]UsageAggregate, error) {
	var out []UsageAggregate
	err := s.db.Model(&BillingUsageEvent{}).
		Select("item_code, SUM(quantity) AS quantity, SUM(amount_micro) AS amount_micro").
		Where("account_id = ? AND status = ?", accountID, usageStatusCharged).
		Where("occurred_at >= ? AND occurred_at < ?", from, to).
		Group("item_code").
		Order("amount_micro DESC").
		Limit(normalizeLimit(limit)).
		Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("billing store: sum usage by item: %w", err)
	}
	return out, nil
}

// GetUsageEvent 按主键读一条事件（控制面 Charge 拿回结算结果用）。
func (s *BillingStore) GetUsageEvent(id string) (*BillingUsageEvent, error) {
	var ev BillingUsageEvent
	if err := s.db.Where("id = ?", id).Take(&ev).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("billing store: get usage event: %w", err)
	}
	return &ev, nil
}

// ListPendingEventsBySession 取某个会话还没结算的事件，供数据面调 settle 时
// 立刻结算（不等 worker 下一轮 tick，§14.5）。按 id 排序让锁序稳定。
func (s *BillingStore) ListPendingEventsBySession(sessionID string, limit int) ([]BillingUsageEvent, error) {
	var events []BillingUsageEvent
	err := s.db.Model(&BillingUsageEvent{}).
		Where("session_id = ? AND status = ?", sessionID, usageStatusPending).
		Order("account_id ASC").Order("id ASC").
		Limit(normalizeLimit(limit)).
		Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("billing store: list pending events by session: %w", err)
	}
	return events, nil
}

// SumSessionAmount 汇总某个会话已计费事件的金额，settle 幂等重放时用来回填
// charged_micro（不重新推导定价，只读事实）。
func (s *BillingStore) SumSessionAmount(sessionID string) (int64, error) {
	var total struct {
		Amount int64
	}
	err := s.db.Model(&BillingUsageEvent{}).
		Select("COALESCE(SUM(amount_micro), 0) AS amount").
		Where("session_id = ? AND status = ?", sessionID, usageStatusCharged).
		Scan(&total).Error
	if err != nil {
		return 0, fmt.Errorf("billing store: sum session amount: %w", err)
	}
	return total.Amount, nil
}

// ClaimPendingEvents 认领一批待结算事件：SELECT ... FOR UPDATE SKIP LOCKED。
//
// 必须在事务里调用（tx 允许为 nil 只是为了测试方便，事务外调用等于没锁）。按
// account_id 排序再取，是为了让多实例 worker 拿到的事件顺序一致，配合“先锁账户行
// 再锁账期行”的固定锁序规避死锁（§5）。
func (s *BillingStore) ClaimPendingEvents(tx *gorm.DB, limit int) ([]BillingUsageEvent, error) {
	var events []BillingUsageEvent
	err := s.dbOr(tx).
		Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate, Options: clause.LockingOptionsSkipLocked}).
		Where("status = ?", usageStatusPending).
		Order("account_id ASC").Order("received_at ASC").Order("id ASC").
		Limit(normalizeLimit(limit)).
		Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("billing store: claim pending events: %w", err)
	}
	return events, nil
}

// UpsertDailyStat 累加日聚合（INSERT ... ON CONFLICT DO UPDATE）。
// 聚合表永久保留，原始事件 90 天后可以清掉。
func (s *BillingStore) UpsertDailyStat(tx *gorm.DB, stat BillingDailyStat) error {
	err := s.dbOr(tx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "account_id"}, {Name: "date"}, {Name: "item_code"},
			{Name: "aimodel_id"}, {Name: "voice_id"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"quantity":     gorm.Expr("billing_daily_stats.quantity + EXCLUDED.quantity"),
			"amount_micro": gorm.Expr("billing_daily_stats.amount_micro + EXCLUDED.amount_micro"),
			"event_count":  gorm.Expr("billing_daily_stats.event_count + EXCLUDED.event_count"),
			"updated_at":   gorm.Expr("EXCLUDED.updated_at"),
		}),
	}).Create(&stat).Error
	if err != nil {
		return fmt.Errorf("billing store: upsert daily stat: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------- 结算事务的步骤

// LockPeriodState 先建行（ON CONFLICT DO NOTHING）再 SELECT ... FOR UPDATE。
// 顺序不能反：先建行是为了躲开并发插入时的幻读，后加锁是为了拿到这一行的排他锁。
//
// 行不存在时返回刚建好的零值行，调用方拿它做边际计价（§16.1）。
func (s *BillingStore) LockPeriodState(tx *gorm.DB, accountID, itemCode, priceID string, periodStart time.Time) (*BillingPeriodState, error) {
	db := s.dbOr(tx)
	row := BillingPeriodState{
		AccountID:   accountID,
		ItemCode:    itemCode,
		PriceID:     priceID,
		PeriodStart: periodStart,
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return nil, fmt.Errorf("billing store: ensure period state: %w", err)
	}

	var st BillingPeriodState
	err := db.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
		Where("account_id = ? AND item_code = ? AND price_id = ? AND period_start = ?",
			accountID, itemCode, priceID, periodStart).
		Take(&st).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("billing store: lock period state: %w", err)
	}
	return &st, nil
}

// UpdatePeriodState 写入账期累计量的绝对值（增量由调用方在锁内算）。行必须已存在。
func (s *BillingStore) UpdatePeriodState(tx *gorm.DB, st BillingPeriodState) error {
	res := s.dbOr(tx).Model(&BillingPeriodState{}).
		Where("account_id = ? AND item_code = ? AND price_id = ? AND period_start = ?",
			st.AccountID, st.ItemCode, st.PriceID, st.PeriodStart).
		Updates(map[string]any{
			"quantity":         st.Quantity,
			"amount_micro":     st.AmountMicro,
			"grant_used_micro": st.GrantUsedMicro,
		})
	if res.Error != nil {
		return fmt.Errorf("billing store: update period state: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// LockAccount 用 SELECT ... FOR UPDATE 锁住账户行，返回锁内读到的余额。
// 锁序固定为「先 account 再 period state」，多实例并发结算才不会互相咬住。
func (s *BillingStore) LockAccount(tx *gorm.DB, accountID string) (*BillingAccount, error) {
	var a BillingAccount
	err := s.dbOr(tx).Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
		Where("id = ?", accountID).
		Take(&a).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("billing store: lock account: %w", err)
	}
	return &a, nil
}

// InsertLedger 追加一条流水（append-only，只增不改）。entry.ID 由数据库填回。
//
// 唯一键（idempotency_key）冲突时返回 error，由调用方按“这笔账已经入过了”处理——
// 这正是幂等键存在的意义（§5）。退款与人工调整写 adjust 流水，不动历史事件行。
func (s *BillingStore) InsertLedger(tx *gorm.DB, entry *BillingLedger) error {
	if err := s.dbOr(tx).Create(entry).Error; err != nil {
		return fmt.Errorf("billing store: insert ledger: %w", err)
	}
	return nil
}

// UpdateAccountBalance 把账户的可用余额与冻结额写成绝对值（不是增量）。
//
// 调用方负责先 LockAccount 拿锁，并在锁内判定透支策略（§5）：余额减到 -credit_limit_micro
// 以下属于业务判断，deny 就把事件标 unpaid、账户置 suspended，allow 就放行。账户不存在返回 ErrNotFound。
func (s *BillingStore) UpdateAccountBalance(tx *gorm.DB, accountID string, balanceMicro, frozenMicro int64) error {
	res := s.dbOr(tx).Model(&BillingAccount{}).Where("id = ?", accountID).
		Updates(map[string]any{"balance_micro": balanceMicro, "frozen_micro": frozenMicro})
	if res.Error != nil {
		return fmt.Errorf("billing store: update account balance: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkEventSettled 回写事件的结算产物：状态、命中价格、金额、失败原因。
// status 取值见 billing.EventStatus*；lastError 为空表示成功（会把上一次的失败原因清掉）。
func (s *BillingStore) MarkEventSettled(tx *gorm.DB, eventID, status, priceID string, amountMicro int64, lastError string) error {
	res := s.dbOr(tx).Model(&BillingUsageEvent{}).Where("id = ?", eventID).
		Updates(map[string]any{
			"status":       status,
			"price_id":     priceID,
			"amount_micro": amountMicro,
			"last_error":   lastError,
		})
	if res.Error != nil {
		return fmt.Errorf("billing store: mark event settled: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UseGrant 消耗一笔赠送额度：used_micro += useMicro，条件是加完不超过 granted_micro。
// 额度已被并发用光（或行不存在）时返回 ErrNotFound，调用方把差额交给余额承担。
func (s *BillingStore) UseGrant(tx *gorm.DB, grantID string, useMicro int64) error {
	res := s.dbOr(tx).Model(&BillingGrant{}).
		Where("id = ? AND used_micro + ? <= granted_micro", grantID, useMicro).
		Updates(map[string]any{"used_micro": gorm.Expr("used_micro + ?", useMicro)})
	if res.Error != nil {
		return fmt.Errorf("billing store: use grant: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
