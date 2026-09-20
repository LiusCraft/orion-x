package store

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/datatypes"
)

// 计费模块的 9 张表（docs/billing-design.md §4）。
//
// store 层对计费领域一无所知：这里只出现字符串 / 整数 / JSON 列，domain ↔ row 的
// 转换住在 internal/billing/service。唯一的口径约定是**所有金额都是 int64 微单位**
// （1e-6 元），全链路不用 float。

// BillingItem 计费项目录（internal/billing.DefaultItems 的投影）。
//
// seed 见 billing_seed.go：按 code upsert，管理员改过的 enabled 不会被 seed 覆盖。
type BillingItem struct {
	Code        string `gorm:"primaryKey;type:varchar(64)" json:"code"`
	Name        string `gorm:"not null;type:varchar(128)" json:"name"`
	ChargeMode  string `gorm:"not null;type:varchar(16)" json:"charge_mode"` // count | duration | usage | recurring
	Unit        string `gorm:"not null;type:varchar(16)" json:"unit"`        // call | second | token | char | byte | period
	MeterSource string `gorm:"not null;index;type:varchar(32)" json:"meter_source"`
	Enabled     bool   `gorm:"not null;default:true" json:"enabled"`
	IsSystem    bool   `gorm:"not null;default:false" json:"is_system"`
	BaseModel
}

func (BillingItem) TableName() string { return "billing_items" }

// BillingPrice 版本化价格。唯一索引 = (item_code, account_id, resource_type, resource_id, effective_from)，
// 保证同一 scope 同一时刻只有一版价格（§3.2）。
//
// 价格表上没有独立的 enabled 开关：这版价格生不生效只能由 effective_from / effective_to
// 这个区间决定。account_id 为空 = 平台标准价；resource_type 停在 item / provider /
// model / voice 中的某一级，resource_id 是该级实体 ID（item 级为空）。
type BillingPrice struct {
	ID             string       `gorm:"primaryKey;type:varchar(36)" json:"id"`
	ItemCode       string       `gorm:"not null;type:varchar(64);uniqueIndex:idx_billing_price_scope,priority:1" json:"item_code"`
	AccountID      string       `gorm:"not null;default:'';type:varchar(36);uniqueIndex:idx_billing_price_scope,priority:2" json:"account_id"`
	ResourceType   string       `gorm:"not null;default:'item';type:varchar(16);uniqueIndex:idx_billing_price_scope,priority:3" json:"resource_type"`
	ResourceID     string       `gorm:"not null;default:'';type:varchar(64);uniqueIndex:idx_billing_price_scope,priority:4" json:"resource_id"`
	EffectiveFrom  time.Time    `gorm:"not null;uniqueIndex:idx_billing_price_scope,priority:5" json:"effective_from"`
	EffectiveTo    *time.Time   `gorm:"index" json:"effective_to,omitempty"`
	Currency       string       `gorm:"not null;default:'CNY';type:varchar(8)" json:"currency"`
	UnitPriceMicro int64        `gorm:"not null;default:0" json:"unit_price_micro"`
	UnitSize       int64        `gorm:"not null;default:1" json:"unit_size"`
	MinChargeMicro int64        `gorm:"not null;default:0" json:"min_charge_micro"`
	Rounding       string       `gorm:"not null;default:'none';type:varchar(16)" json:"rounding"` // none | ceil | half:up
	Tiers          BillingTiers `gorm:"type:jsonb" json:"tiers,omitempty"`
	BaseModel
}

func (BillingPrice) TableName() string { return "billing_prices" }

// BillingTier 是阶梯价的一档：账期内累计 quantity 落在 (上一档 up_to, 本档 up_to]
// 区间内的量按 unit_price_micro 计。up_to = 0 表示最后一档、无上限。
type BillingTier struct {
	UpTo           int64 `json:"up_to"`
	UnitPriceMicro int64 `json:"unit_price_micro"`
}

// BillingTiers 是 billing_prices.tiers 这个 jsonb 列的 Go 形态（[{up_to, unit_price_micro}]）。
// 空值写入 NULL，NULL 也能扫回来；这样“没有阶梯”在库里只有一种表示。
type BillingTiers []BillingTier

// Value 实现 driver.Valuer。空切片写 NULL，其它情况写 JSON 数组文本。
func (t BillingTiers) Value() (driver.Value, error) {
	if len(t) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("billing store: marshal tiers: %w", err)
	}
	return raw, nil
}

// Scan 实现 sql.Scanner。NULL / 空串都扫成 nil（等价于“没有阶梯”）。
func (t *BillingTiers) Scan(value any) error {
	if value == nil {
		*t = nil
		return nil
	}

	var raw []byte
	switch v := value.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("billing store: scan tiers: unsupported type %T", value)
	}
	if len(raw) == 0 {
		*t = nil
		return nil
	}

	var tiers []BillingTier
	if err := json.Unmarshal(raw, &tiers); err != nil {
		return fmt.Errorf("billing store: scan tiers: %w", err)
	}
	*t = tiers
	return nil
}

// BillingAccount 账户 / 钱包。
//
// 主键是内部 ID（`acct_` 前缀），另加 (subject_type, subject_id) 唯一索引：P3 支持
// Organization 时只需多插一行账户，历史 usage_events / ledger / daily_stats 一个字
// 都不用回填（§12）。
//
// 预付费与后付费是同一条代码路径，差别只在 credit_limit_micro 是否为 0：
// 准入判断统一为 `amount ≤ balance + credit_limit − frozen`。
type BillingAccount struct {
	ID               string `gorm:"primaryKey;type:varchar(36)" json:"id"`
	SubjectType      string `gorm:"not null;uniqueIndex:idx_billing_account_subject,priority:1;type:varchar(16)" json:"subject_type"`
	SubjectID        string `gorm:"not null;uniqueIndex:idx_billing_account_subject,priority:2;type:varchar(36)" json:"subject_id"`
	Currency         string `gorm:"not null;default:'CNY';type:varchar(8)" json:"currency"`
	BalanceMicro     int64  `gorm:"not null;default:0" json:"balance_micro"` // 可用余额，可以为负 = 后付欠款
	FrozenMicro      int64  `gorm:"not null;default:0" json:"frozen_micro"`  // 会话预授权占用
	CreditLimitMicro int64  `gorm:"not null;default:0" json:"credit_limit_micro"`
	Status           string `gorm:"not null;default:'active';index;type:varchar(16)" json:"status"` // active | suspended | closed
	BaseModel
}

func (BillingAccount) TableName() string { return "billing_accounts" }

// BillingLedger 流水，append-only：只增不改。
//
// idempotency_key 唯一索引是幂等兜底（`settle:<event_id>` / `charge:<ref_type>:<ref_id>`）；
// 重放撞上唯一冲突时由调用方按“已经入过账”处理。direction / kind / ref_type 都是
// 开放枚举，别在这里穷举（§19）。
type BillingLedger struct {
	ID                int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	AccountID         string    `gorm:"not null;index;type:varchar(36)" json:"account_id"`
	Direction         string    `gorm:"not null;type:varchar(8)" json:"direction"` // debit | credit
	AmountMicro       int64     `gorm:"not null" json:"amount_micro"`
	BalanceAfterMicro int64     `gorm:"not null" json:"balance_after_micro"`
	Kind              string    `gorm:"not null;index;type:varchar(24)" json:"kind"` // charge | grant | recharge | refund | adjust | reserve | release | expire
	ItemCode          string    `gorm:"not null;default:'';type:varchar(64)" json:"item_code"`
	RefType           string    `gorm:"not null;default:'';type:varchar(32)" json:"ref_type"` // usage:event | reservation | order | manual
	RefID             string    `gorm:"not null;default:'';type:varchar(64)" json:"ref_id"`
	IdempotencyKey    string    `gorm:"not null;uniqueIndex;type:varchar(128)" json:"idempotency_key"`
	OccurredAt        time.Time `gorm:"not null;index" json:"occurred_at"`
	Note              string    `gorm:"type:text" json:"note,omitempty"`
	Creator           string    `gorm:"type:varchar(36)" json:"creator,omitempty"`
}

func (BillingLedger) TableName() string { return "billing_ledger" }

// BillingUsageEvent 原始用量事实，也是结算队列（outbox 模式）。
//
// 主键 = 数据面生成的 event_id → 重试天然去重。账户、价格、金额全部由控制面填，
// 不信任上报体（§14.2）。事件表只存事实和量，不存内容：合成了哪段文本、收到了什么
// 音频都不进这张表，原始量留在 dimensions 里（§13）。
type BillingUsageEvent struct {
	ID          string            `gorm:"primaryKey;type:varchar(64)" json:"id"`
	AccountID   string            `gorm:"not null;index;type:varchar(36)" json:"account_id"`
	VoicebotID  string            `gorm:"not null;default:'';type:varchar(36)" json:"voicebot_id"`
	DeviceID    string            `gorm:"not null;default:'';index;type:varchar(128)" json:"device_id"`
	SessionID   string            `gorm:"not null;default:'';index;type:varchar(64)" json:"session_id"`
	ItemCode    string            `gorm:"not null;index;type:varchar(64)" json:"item_code"`
	Quantity    int64             `gorm:"not null;default:0" json:"quantity"`
	AIModelID   string            `gorm:"column:aimodel_id;not null;default:'';type:varchar(36)" json:"aimodel_id"`
	ProviderID  string            `gorm:"not null;default:'';type:varchar(36)" json:"provider_id"`
	VoiceID     string            `gorm:"not null;default:'';type:varchar(36)" json:"voice_id"`
	BYOK        bool              `gorm:"not null;default:false" json:"byok"`
	OccurredAt  time.Time         `gorm:"not null;index" json:"occurred_at"`
	ReceivedAt  time.Time         `gorm:"not null;index:idx_billing_usage_status,priority:2" json:"received_at"`
	TurnIndex   int64             `gorm:"not null;default:0" json:"turn_index"`
	Dimensions  datatypes.JSONMap `gorm:"type:jsonb" json:"dimensions,omitempty"`
	Status      string            `gorm:"not null;default:'pending';index:idx_billing_usage_status,priority:1;type:varchar(16)" json:"status"` // pending | charged | skipped | unpaid | rejected
	PriceID     string            `gorm:"not null;default:'';type:varchar(36)" json:"price_id"`
	AmountMicro int64             `gorm:"not null;default:0" json:"amount_micro"`
	LastError   string            `gorm:"type:text" json:"last_error,omitempty"`
}

func (BillingUsageEvent) TableName() string { return "billing_usage_events" }

// BillingReservation 会话级预冻结 + 定价快照。
//
// session_id 唯一 → authorize 幂等（断线重连拿回同一条，不重复冻结，§14.3）。
// expires_at 每收到这个会话的上报就顺延（TouchReservation，§14.4）：它的语义是
// “沉默多久算死”，不是“会话最多活多久”。resource_snapshot 是
// billing.SessionProfile 的 JSON，price_snapshot 是 map[item_code]PriceSnapshot。
type BillingReservation struct {
	ID               string            `gorm:"primaryKey;type:varchar(36)" json:"id"`
	SessionID        string            `gorm:"not null;uniqueIndex;type:varchar(64)" json:"session_id"`
	AccountID        string            `gorm:"not null;index;type:varchar(36)" json:"account_id"`
	DeviceID         string            `gorm:"not null;default:'';index;type:varchar(128)" json:"device_id"`
	VoicebotID       string            `gorm:"not null;default:'';type:varchar(36)" json:"voicebot_id"`
	ReservedMicro    int64             `gorm:"not null;default:0" json:"reserved_micro"`
	Status           string            `gorm:"not null;default:'open';index;type:varchar(16)" json:"status"` // open | settled | expired | degraded
	ExpiresAt        time.Time         `gorm:"not null;index" json:"expires_at"`
	ResourceSnapshot datatypes.JSONMap `gorm:"type:jsonb" json:"resource_snapshot,omitempty"`
	PriceSnapshot    datatypes.JSONMap `gorm:"type:jsonb" json:"price_snapshot,omitempty"`
	SettledAt        *time.Time        `json:"settled_at,omitempty"`
	ReleasedMicro    int64             `gorm:"not null;default:0" json:"released_micro"`
	LastError        string            `gorm:"type:text" json:"last_error,omitempty"`
	BaseModel
}

func (BillingReservation) TableName() string { return "billing_reservations" }

// BillingGrant 限定计费项的赠送额度。
//
// 注册赠送额度是 P1 的一部分（credit_limit = 0 + 余额 0 = 新用户第一次连设备就被拒），
// 但赠款必须走这张表，不能图省事直接写进 balance_micro：那样赠款和充值款在账上就
// 分不开了，退款、对账、算实收全说不清（§12）。消耗优先级 grant → balance → credit_limit。
type BillingGrant struct {
	ID           string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	AccountID    string    `gorm:"not null;uniqueIndex:idx_billing_grant_scope,priority:1;type:varchar(36)" json:"account_id"`
	ItemCode     string    `gorm:"not null;uniqueIndex:idx_billing_grant_scope,priority:2;type:varchar(64)" json:"item_code"`
	Source       string    `gorm:"not null;uniqueIndex:idx_billing_grant_scope,priority:3;type:varchar(32)" json:"source"`
	GrantedMicro int64     `gorm:"not null;default:0" json:"granted_micro"`
	UsedMicro    int64     `gorm:"not null;default:0" json:"used_micro"`
	ExpiresAt    time.Time `gorm:"not null;index" json:"expires_at"`
	BaseModel
}

func (BillingGrant) TableName() string { return "billing_grants" }

// BillingPeriodState 账期累计量：阶梯价 / 起步价做边际计价的依据（§16.1）。
//
// 主键 (account_id, item_code, price_id, period_start)：改价意味着 price_id 变了，
// 那就另起一行 state，别拿新价格去套已经走过的档位；跨账期同样新建一行，阶梯重新起算。
// 这一行必须与 ledger 在同一个事务里更新，并且更新前先 SELECT ... FOR UPDATE。
type BillingPeriodState struct {
	AccountID      string    `gorm:"primaryKey;type:varchar(36)" json:"account_id"`
	ItemCode       string    `gorm:"primaryKey;type:varchar(64)" json:"item_code"`
	PriceID        string    `gorm:"primaryKey;type:varchar(36)" json:"price_id"`
	PeriodStart    time.Time `gorm:"primaryKey" json:"period_start"`
	Quantity       int64     `gorm:"not null;default:0" json:"quantity"`
	AmountMicro    int64     `gorm:"not null;default:0" json:"amount_micro"`
	GrantUsedMicro int64     `gorm:"not null;default:0" json:"grant_used_micro"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (BillingPeriodState) TableName() string { return "billing_period_states" }

// BillingDailyStat 日聚合报表（报表用，原始事件 90 天后可以清掉）。
type BillingDailyStat struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	AccountID   string    `gorm:"not null;uniqueIndex:idx_billing_daily_scope,priority:1;type:varchar(36)" json:"account_id"`
	Date        string    `gorm:"not null;uniqueIndex:idx_billing_daily_scope,priority:2;type:varchar(10)" json:"date"` // YYYY-MM-DD
	ItemCode    string    `gorm:"not null;uniqueIndex:idx_billing_daily_scope,priority:3;type:varchar(64)" json:"item_code"`
	AIModelID   string    `gorm:"column:aimodel_id;not null;default:'';uniqueIndex:idx_billing_daily_scope,priority:4;type:varchar(36)" json:"aimodel_id"`
	VoiceID     string    `gorm:"not null;default:'';uniqueIndex:idx_billing_daily_scope,priority:5;type:varchar(36)" json:"voice_id"`
	Quantity    int64     `gorm:"not null;default:0" json:"quantity"`
	AmountMicro int64     `gorm:"not null;default:0" json:"amount_micro"`
	EventCount  int64     `gorm:"not null;default:0" json:"event_count"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (BillingDailyStat) TableName() string { return "billing_daily_stats" }
