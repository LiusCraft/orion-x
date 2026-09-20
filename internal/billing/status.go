package billing

// 这一组枚举值都是**开放枚举**：新增一种入账来源、一种事件状态，不应该改结算
// 代码，也不该在所有 switch 里穷举。落库的是字符串，判等用这里的常量。

// 账户状态。
const (
	AccountStatusActive    = "active"
	AccountStatusSuspended = "suspended"
	AccountStatusClosed    = "closed"
)

// 账本方向。
const (
	DirectionDebit  = "debit"  // 减余额
	DirectionCredit = "credit" // 加余额
)

// 账本条目类型。order / manual 是特意留的开放位。
const (
	LedgerKindCharge   = "charge"
	LedgerKindGrant    = "grant"
	LedgerKindRecharge = "recharge"
	LedgerKindRefund   = "refund"
	LedgerKindAdjust   = "adjust"
	LedgerKindReserve  = "reserve"
	LedgerKindRelease  = "release"
	LedgerKindExpire   = "expire"
)

// 用量事件状态：pending → charged / skipped，扣款失败落 unpaid。
const (
	EventStatusPending  = "pending"
	EventStatusCharged  = "charged"
	EventStatusSkipped  = "skipped"
	EventStatusUnpaid   = "unpaid"
	EventStatusRejected = "rejected"
)

// 预冻结状态：open → settled / expired；authorize 降级补的行是 degraded。
const (
	ReservationStatusOpen     = "open"
	ReservationStatusSettled  = "settled"
	ReservationStatusExpired  = "expired"
	ReservationStatusDegraded = "degraded"
)

// 计费主体类型。
const (
	SubjectTypeUser = "user"
	SubjectTypeOrg  = "org"
)

// 透支策略。
const (
	OverdraftDeny  = "deny"  // 预付费：拒绝并停服
	OverdraftAllow = "allow" // 后付：允许余额转负至额度下限
)

// 结算模式。
const (
	SettlementAsync = "async"
	SettlementSync  = "sync"
)
