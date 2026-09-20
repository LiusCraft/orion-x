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
	// usageStatusSkipped 是结算过、但本来就不该产生金额的事件（BYOK / 计费项未启用 / 量为 0）。
	usageStatusSkipped = "skipped"
	// usageStatusUnpaid 是没算成钱的事件：没命中价格、或者扣款时撞上透支策略。
	// 它需要人工处理（补价格 / 重放），报表里必须能看见。
	usageStatusUnpaid = "unpaid"
	// usageStatusRejected 是控制面拒绝掉的上报（账户被停用之类）：它既不是用量也不是
	// 待结算的账，聚合报表里不出现。
	usageStatusRejected = "rejected"
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

// UsageByModelQuery 是「模型 × 计费项」聚合的过滤条件。
// AccountID 为空表示不限账户（平台级报表）；From/To 作用在 occurred_at 上，左闭右开。
type UsageByModelQuery struct {
	AccountID string
	From      *time.Time
	To        *time.Time
	Limit     int
}

// UsageByModelRow 是（模型 × 计费项）粒度的一行用量。
//
//   - Quantity 是这段时间上报的全部原始量，**含还没结算的**；单位随 ItemCode 走（§13），
//     跨计费项不可加；
//   - AmountMicro 只累计已计费（charged）事件，所以它和 Quantity 不是同一批事件的产物；
//   - 后面三个计数器是事件的状态分布：Pending 还在结算队列里（秒级）、Unpaid 没算成钱
//     （没配价格 / 透支被拒，需要人工处理）、Skipped 本来就不计费（BYOK / 未启用 / 量为 0）。
//     剩下的就是已计费（charged）——四个状态加起来 = EventCount。缺失的那部分金额不是 0，
//     是“还没算”，报表要能把这件事说出来。
//
// 模型名 / 类型 / 厂商名是 join 出来的展示信息，不是事实的一部分：模型行被删掉时它们是
// 空串，调用方按 aimodel_id 显示即可。
//
// AIModelID 的 column tag 不能省：列名是 aimodel_id（历史拼写，见 BillingUsageEvent），
// 而 GORM 从字段名推出的是 ai_model_id——少了这个 tag，Scan 会静默扫回空串。
type UsageByModelRow struct {
	AIModelID     string `gorm:"column:aimodel_id" json:"aimodel_id"`
	ModelName     string `json:"model_name"`
	ModelType     string `json:"model_type"`
	ProviderID    string `json:"provider_id"`
	ProviderName  string `json:"provider_name"`
	ItemCode      string `json:"item_code"`
	Quantity      int64  `json:"quantity"`
	AmountMicro   int64  `json:"amount_micro"`
	EventCount    int64  `json:"event_count"`
	PendingEvents int64  `json:"pending_events"`
	UnpaidEvents  int64  `json:"unpaid_events"`
	SkippedEvents int64  `json:"skipped_events"`
}

// usageByModelSelect 是聚合的列。状态字面量由常量拼进来（不是另抄一份），拼出来的是
// 编译期常量，没有注入面。
var usageByModelSelect = `billing_usage_events.aimodel_id AS aimodel_id,` +
	` COALESCE(ai_models.name, '') AS model_name,` +
	` COALESCE(ai_models.type, '') AS model_type,` +
	` COALESCE(ai_models.provider_id, billing_usage_events.provider_id) AS provider_id,` +
	` COALESCE(providers.name, '') AS provider_name,` +
	` billing_usage_events.item_code AS item_code,` +
	` SUM(billing_usage_events.quantity) AS quantity,` +
	` SUM(CASE WHEN billing_usage_events.status = '` + usageStatusCharged + `' THEN billing_usage_events.amount_micro ELSE 0 END) AS amount_micro,` +
	` COUNT(*) AS event_count` +
	usageStatusCountSelect(usageStatusPending, "pending_events") +
	usageStatusCountSelect(usageStatusUnpaid, "unpaid_events") +
	usageStatusCountSelect(usageStatusSkipped, "skipped_events")

// usageStatusCountSelect 拼一个“数某个状态的事件条数”的聚合列。status 与 alias 都是
// 调用方给的常量（见 usageByModelSelect），不来自请求。
func usageStatusCountSelect(status, alias string) string {
	return `, SUM(CASE WHEN billing_usage_events.status = '` + status + `' THEN 1 ELSE 0 END) AS ` + alias
}

// SumUsageByModel 按（模型 × 计费项）汇总一段时间内的用量与已计费金额。
//
// rejected 事件不算用量（那是控制面拒绝的上报），其余状态全部计入——排查时正需要看到
// 「量到了但没算成钱」的那些（skipped / unpaid / pending），所以这里不按状态过滤。
//
// join ai_models / providers 只为了报表演示，两个都是 LEFT JOIN：模型或厂商行被删掉了，
// 那段用量也不该从报表里消失。表名是 GORM 从 AIModel 推出来的 ai_models（没有 TableName
// 方法），别按结构体名写成 aimodels。
func (s *BillingStore) SumUsageByModel(q UsageByModelQuery) ([]UsageByModelRow, error) {
	db := s.db.Model(&BillingUsageEvent{}).
		Select(usageByModelSelect).
		Joins("LEFT JOIN ai_models ON ai_models.id = billing_usage_events.aimodel_id").
		Joins("LEFT JOIN providers ON providers.id = COALESCE(ai_models.provider_id, billing_usage_events.provider_id)").
		Where("billing_usage_events.status <> ?", usageStatusRejected).
		Group("billing_usage_events.aimodel_id, billing_usage_events.item_code, ai_models.name, ai_models.type, " +
			"COALESCE(ai_models.provider_id, billing_usage_events.provider_id), providers.name").
		Order("billing_usage_events.aimodel_id ASC").
		Order("billing_usage_events.item_code ASC").
		Limit(normalizeLimit(q.Limit))

	if q.AccountID != "" {
		db = db.Where("billing_usage_events.account_id = ?", q.AccountID)
	}
	if q.From != nil {
		db = db.Where("billing_usage_events.occurred_at >= ?", *q.From)
	}
	if q.To != nil {
		db = db.Where("billing_usage_events.occurred_at < ?", *q.To)
	}

	var out []UsageByModelRow
	if err := db.Scan(&out).Error; err != nil {
		return nil, fmt.Errorf("billing store: sum usage by model: %w", err)
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
