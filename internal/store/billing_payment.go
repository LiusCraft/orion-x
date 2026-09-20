package store

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 充值链路的两张表：订单表是我们自己下的单，通知表是网关送来的原始事实。
//
// 和 billing_* 的其它表一样，金额一律 int64 微单位（￥1 = 1000000），只有出站请求和
// 回调解析那两处才转成网关要的「元」（见 internal/billing/gateway/epay）。
//
// 这两张表不参与「用量 × 单价」的结算：充值不是计量项，不建 billing_items seed，
// 也不录价格行。钱进余额走的是 billing.Service.Credit（写 recharge 流水）。

// 订单状态字面量。store 层不能 import internal/billing，这里与 billing.OrderStatus*
// 保持字面一致（改一边要同步另一边）。
const (
	paymentOrderPending  = "order:pending"
	paymentOrderPaid     = "order:paid"
	paymentOrderCredited = "order:credited"
	paymentOrderClosed   = "order:closed"
	paymentOrderRefunded = "order:refunded"
)

// PaymentOrderQuery 是充值订单列表的过滤条件。Keyword 同时匹配订单号与网关流水号
// （模糊）；SubjectType/SubjectID 为空 = 不过滤（管理端看全部）。
type PaymentOrderQuery struct {
	SubjectType string
	SubjectID   string
	Channel     string
	Status      string
	Keyword     string
	Limit       int
	Offset      int
}

// PaymentOrder 是一笔充值订单。
//
// out_trade_no 是我们生成的、对网关唯一的单号，同时是入账流水幂等键的一半
// （credit:order:<out_trade_no>）——所以这两处用的必须是同一个字符串。
//
// 状态机 pending → paid → credited，中间那步不能省：网关确认收款（paid）和余额
// 加上（credited）分开落库，进程在两步之间挂掉时 Sweep 能把账补齐。closed 是超时
// 未支付的终态，但晚到的成功通知仍然接受（钱是真的，不能因为本地超时就吞掉）。
type PaymentOrder struct {
	ID             int64      `gorm:"primaryKey;autoIncrement"`
	OutTradeNo     string     `gorm:"column:out_trade_no;not null;uniqueIndex;type:varchar(64)" json:"out_trade_no"`
	SubjectType    string     `gorm:"not null;default:'user';index:idx_payment_order_subject,priority:1;type:varchar(16)" json:"subject_type"`
	SubjectID      string     `gorm:"not null;default:'';index:idx_payment_order_subject,priority:2;type:varchar(36)" json:"subject_id"`
	Channel        string     `gorm:"not null;default:'';type:varchar(32)" json:"channel"`
	AmountMicro    int64      `gorm:"not null;default:0" json:"amount_micro"`
	Currency       string     `gorm:"not null;default:'CNY';type:varchar(8)" json:"currency"`
	Title          string     `gorm:"not null;default:'';type:varchar(128)" json:"title"`
	Status         string     `gorm:"not null;default:'order:pending';index;type:varchar(24)" json:"status"`
	GatewayTradeNo string     `gorm:"not null;default:'';index;type:varchar(64)" json:"gateway_trade_no,omitempty"`
	ClientIP       string     `gorm:"not null;default:'';type:varchar(64)" json:"client_ip,omitempty"`
	ExpiresAt      time.Time  `gorm:"not null;index" json:"expires_at"`
	PaidAt         *time.Time `json:"paid_at,omitempty"`
	CreditedAt     *time.Time `json:"credited_at,omitempty"`
	RefundedAt     *time.Time `json:"refunded_at,omitempty"`
	BaseModel
}

func (PaymentOrder) TableName() string { return "billing_payment_orders" }

// PaymentNotification 是网关回调的原始事实，append-only：只插入，不更新不删除。
//
// 网关重复通知是正常现象（它没收到 success 就会一直重试），所以这里不做唯一约束：
// 每一次投递都是一条事实，排障时要能看到「到底通知了几次、每次长什么样」。
type PaymentNotification struct {
	ID             int64  `gorm:"primaryKey;autoIncrement"`
	OutTradeNo     string `gorm:"not null;default:'';index;type:varchar(64)"`
	GatewayTradeNo string `gorm:"not null;default:'';type:varchar(64)"`
	AmountRaw      string `gorm:"not null;default:'';type:varchar(32)"`
	Status         string `gorm:"not null;default:'';type:varchar(24)"`
	Payload        string `gorm:"not null;default:'';type:text"`
	CreatedAt      time.Time
}

func (PaymentNotification) TableName() string { return "billing_payment_notifications" }

// PaymentStore 是支付订单与回调事实的仓储。和 BillingStore 分开：它不是计费引擎的
// 结算路径（不写余额、不写流水），只是「钱进来之前」的单据。
type PaymentStore struct{ db *gorm.DB }

func NewPaymentStore(db *gorm.DB) *PaymentStore { return &PaymentStore{db: db} }

// DB 暴露底层连接，供调用方开事务。
func (s *PaymentStore) DB() *gorm.DB { return s.db }

func (s *PaymentStore) dbOr(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return s.db
}

// CreateOrder 落一笔新订单。out_trade_no 撞唯一索引会返回错误——那不是 bug，是
// 调用方该换个号（我们用 80 位随机，正常不会撞）。
func (s *PaymentStore) CreateOrder(tx *gorm.DB, order *PaymentOrder) error {
	if err := s.dbOr(tx).Create(order).Error; err != nil {
		return fmt.Errorf("payment store: create order: %w", err)
	}
	return nil
}

// GetOrder 按商户单号查订单，未找到返回 ErrNotFound。
func (s *PaymentStore) GetOrder(outTradeNo string) (*PaymentOrder, error) {
	var order PaymentOrder
	if err := s.db.Where("out_trade_no = ?", outTradeNo).Take(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("payment store: get order: %w", err)
	}
	return &order, nil
}

// LockOrder 用 SELECT ... FOR UPDATE 锁住订单行，必须在事务里调用。
// 回调可能同时到达多次，行锁是「同一笔钱只入一次账」的第一道闸。
func (s *PaymentStore) LockOrder(tx *gorm.DB, outTradeNo string) (*PaymentOrder, error) {
	var order PaymentOrder
	err := s.dbOr(tx).Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
		Where("out_trade_no = ?", outTradeNo).
		Take(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("payment store: lock order: %w", err)
	}
	return &order, nil
}

// InsertNotification 追加一条回调事实。
func (s *PaymentStore) InsertNotification(tx *gorm.DB, n *PaymentNotification) error {
	if err := s.dbOr(tx).Create(n).Error; err != nil {
		return fmt.Errorf("payment store: insert notification: %w", err)
	}
	return nil
}

// MarkOrderPaid 把订单标成已收款。条件是 pending / closed：重复通知打不进 paid_at
// （幂等），而超时关掉的单子收到晚到的成功通知时照样接受——通知是验过签的，钱是真的。
func (s *PaymentStore) MarkOrderPaid(tx *gorm.DB, outTradeNo, gatewayTradeNo string, at time.Time) error {
	res := s.dbOr(tx).Model(&PaymentOrder{}).
		Where("out_trade_no = ? AND status IN ?", outTradeNo, []string{paymentOrderPending, paymentOrderClosed}).
		Updates(map[string]any{
			"status":           paymentOrderPaid,
			"gateway_trade_no": gatewayTradeNo,
			"paid_at":          at,
		})
	if res.Error != nil {
		return fmt.Errorf("payment store: mark order paid: %w", res.Error)
	}
	return nil
}

// MarkOrderCredited 把订单标成已入账。条件只有 paid：不能从 pending 直接跳到
// credited，那样「网关到底收没收到钱」这件事就没证据了。
func (s *PaymentStore) MarkOrderCredited(outTradeNo string, at time.Time) error {
	res := s.db.Model(&PaymentOrder{}).
		Where("out_trade_no = ? AND status = ?", outTradeNo, paymentOrderPaid).
		Updates(map[string]any{"status": paymentOrderCredited, "credited_at": at})
	if res.Error != nil {
		return fmt.Errorf("payment store: mark order credited: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CloseOrder 关闭一笔超时未支付的订单。
func (s *PaymentStore) CloseOrder(outTradeNo string) error {
	res := s.db.Model(&PaymentOrder{}).
		Where("out_trade_no = ? AND status = ?", outTradeNo, paymentOrderPending).
		Update("status", paymentOrderClosed)
	if res.Error != nil {
		return fmt.Errorf("payment store: close order: %w", res.Error)
	}
	return nil
}

// MarkOrderRefunded 把订单标成已退款。条件只有 credited：只能退已经真的进了余额的
// 钱，pending / paid 的单子没有可退的账（后者应该先入账再退）。
func (s *PaymentStore) MarkOrderRefunded(outTradeNo string, at time.Time) error {
	res := s.db.Model(&PaymentOrder{}).
		Where("out_trade_no = ? AND status = ?", outTradeNo, paymentOrderCredited).
		Updates(map[string]any{"status": paymentOrderRefunded, "refunded_at": at})
	if res.Error != nil {
		return fmt.Errorf("payment store: mark order refunded: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ListOrders 按过滤条件分页返回充值订单，最新的在前。
func (s *PaymentStore) ListOrders(q PaymentOrderQuery) ([]PaymentOrder, error) {
	var orders []PaymentOrder
	err := s.db.Model(&PaymentOrder{}).Scopes(paymentOrderScope(q)).
		Order("created_at DESC").Order("id DESC").
		Offset(q.Offset).Limit(normalizeLimit(q.Limit)).
		Find(&orders).Error
	if err != nil {
		return nil, fmt.Errorf("payment store: list orders: %w", err)
	}
	return orders, nil
}

// CountOrders 返回符合过滤条件的订单总数（分页配套）。
func (s *PaymentStore) CountOrders(q PaymentOrderQuery) (int64, error) {
	var total int64
	if err := s.db.Model(&PaymentOrder{}).Scopes(paymentOrderScope(q)).Count(&total).Error; err != nil {
		return 0, fmt.Errorf("payment store: count orders: %w", err)
	}
	return total, nil
}

func paymentOrderScope(q PaymentOrderQuery) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if q.SubjectType != "" {
			db = db.Where("subject_type = ?", q.SubjectType)
		}
		if q.SubjectID != "" {
			db = db.Where("subject_id = ?", q.SubjectID)
		}
		if q.Channel != "" {
			db = db.Where("channel = ?", q.Channel)
		}
		if q.Status != "" {
			db = db.Where("status = ?", q.Status)
		}
		if q.Keyword != "" {
			like := "%" + escapeLike(q.Keyword) + "%"
			db = db.Where("out_trade_no ILIKE ? OR gateway_trade_no ILIKE ?", like, like)
		}
		return db
	}
}

// ListExpiredOrders 取已过期还没付款的订单（对账兜底要拿它们去主动查单）。
func (s *PaymentStore) ListExpiredOrders(now time.Time, limit int) ([]PaymentOrder, error) {
	var orders []PaymentOrder
	err := s.db.Model(&PaymentOrder{}).
		Where("status = ? AND expires_at < ?", paymentOrderPending, now).
		Order("expires_at ASC").
		Limit(normalizeLimit(limit)).
		Find(&orders).Error
	if err != nil {
		return nil, fmt.Errorf("payment store: list expired orders: %w", err)
	}
	return orders, nil
}

// ListPaidUncreditedOrders 取已收款但还没入账的订单。这个查询就是那张「进程在中间
// 挂掉」的网：paid 是网关给的事实，credited 才是我们这边的账，两者之间的空隙由
// Sweep 反复扫出来补上。
func (s *PaymentStore) ListPaidUncreditedOrders(limit int) ([]PaymentOrder, error) {
	var orders []PaymentOrder
	err := s.db.Model(&PaymentOrder{}).
		Where("status = ?", paymentOrderPaid).
		Order("paid_at ASC").
		Limit(normalizeLimit(limit)).
		Find(&orders).Error
	if err != nil {
		return nil, fmt.Errorf("payment store: list paid uncredited orders: %w", err)
	}
	return orders, nil
}
