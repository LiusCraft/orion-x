package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

// 充值这条链路的控制面：下单 → 回调 / 对账 → 入账。
//
// 它自己一分钱都不碰：入账一律调 Service.Credit（写 recharge 流水，幂等键
// credit:order:<out_trade_no>）。这一层只负责回答「钱到底进没进来」——验签、核对
// 金额、把网关的事实落库，以及进程在中间挂掉之后把账补齐。
//
// 充值不是计量项：不建 billing_items seed，也不录价格行（那个是为
// quantity × unit_price 准备的）。钱只走 balance_micro 与流水。

// ErrPaymentDisabled 是充值通道没开（配置里没接支付渠道）时的错误。
var ErrPaymentDisabled = errors.New("billing: payment is disabled")

// PaymentConfig 是充值链路的运行参数。
type PaymentConfig struct {
	Enabled bool
	// Currency 是充值币种，与计费引擎的单币种保持一致。
	Currency string
	// MinAmountMicro / MaxAmountMicro 是单笔充值的上下限，服务端校验。
	MinAmountMicro int64
	MaxAmountMicro int64
	// OrderTTL 是订单有效期。超时后 Sweep 会先查一次单再决定关不关。
	OrderTTL time.Duration
	// Channels 是允许的支付渠道，其它渠道下单直接拒。
	Channels []billing.PayChannel
	// SubjectPrefix 是订单标题前缀，收银台上会显示。
	SubjectPrefix string
	// Now 便于测试注入。
	Now func() time.Time
}

// DefaultPaymentConfig 返回可用的默认配置。
func DefaultPaymentConfig() PaymentConfig {
	return PaymentConfig{
		Currency:       "CNY",
		MinAmountMicro: billing.MicroPerUnit,          // ¥1
		MaxAmountMicro: 10_000 * billing.MicroPerUnit, // ¥10000
		OrderTTL:       30 * time.Minute,
		Channels:       billing.PayChannels(),
		SubjectPrefix:  "余额充值 ",
		Now:            time.Now,
	}
}

func normalizePaymentConfig(cfg PaymentConfig) PaymentConfig {
	def := DefaultPaymentConfig()
	if strings.TrimSpace(cfg.Currency) == "" {
		cfg.Currency = def.Currency
	}
	if cfg.MinAmountMicro <= 0 {
		cfg.MinAmountMicro = def.MinAmountMicro
	}
	if cfg.MaxAmountMicro < cfg.MinAmountMicro {
		cfg.MaxAmountMicro = def.MaxAmountMicro
	}
	if cfg.OrderTTL <= 0 {
		cfg.OrderTTL = def.OrderTTL
	}
	if len(cfg.Channels) == 0 {
		cfg.Channels = def.Channels
	}
	if strings.TrimSpace(cfg.SubjectPrefix) == "" {
		// 只判断空，不改写：前缀是收银台上给用户看的文案，尾部空格有意义。
		cfg.SubjectPrefix = def.SubjectPrefix
	}
	if cfg.Now == nil {
		cfg.Now = def.Now
	}
	return cfg
}

// PaymentService 是充值链路的全部能力（下单、回调、对账、入账）。
type PaymentService struct {
	orders  *store.PaymentStore
	billing *Service
	gateway billing.PaymentGateway
	cfg     PaymentConfig
}

func NewPaymentService(orders *store.PaymentStore, billingSvc *Service, gateway billing.PaymentGateway, cfg PaymentConfig) *PaymentService {
	return &PaymentService{
		orders:  orders,
		billing: billingSvc,
		gateway: gateway,
		cfg:     normalizePaymentConfig(cfg),
	}
}

// Config 返回生效的配置（规范化之后）。
func (s *PaymentService) Config() PaymentConfig { return s.cfg }

func (s *PaymentService) now() time.Time { return s.cfg.Now() }

// Supports 判断某个渠道是否在允许列表里。
func (s *PaymentService) Supports(ch billing.PayChannel) bool {
	for _, allowed := range s.cfg.Channels {
		if allowed == ch {
			return true
		}
	}
	return false
}

// CreateRecharge 下一笔充值单，返回订单与网关给的付款入口。
//
// 金额有三道校验（服务端限额、整分、渠道支持），下单用的钱从头到尾由服务端决定：
// 客户端只能说要充多少，不能说要付多少。
func (s *PaymentService) CreateRecharge(
	ctx context.Context,
	subjectType, subjectID string,
	channel billing.PayChannel,
	amountMicro int64,
	clientIP string,
) (*store.PaymentOrder, billing.PaymentResult, error) {
	if !s.cfg.Enabled {
		return nil, billing.PaymentResult{}, ErrPaymentDisabled
	}
	if err := validateSubject(subjectType, subjectID); err != nil {
		return nil, billing.PaymentResult{}, err
	}
	if !s.Supports(channel) {
		return nil, billing.PaymentResult{}, fmt.Errorf("%w: %q", billing.ErrUnknownChannel, channel)
	}
	if amountMicro < s.cfg.MinAmountMicro || amountMicro > s.cfg.MaxAmountMicro {
		return nil, billing.PaymentResult{}, fmt.Errorf("%w: amount must be between %s and %s",
			ErrInvalidRequest, billing.FormatMicro(s.cfg.MinAmountMicro), billing.FormatMicro(s.cfg.MaxAmountMicro))
	}
	// 网关的 money 只有两位小数：不是整分的金额在这里就拒掉，别等到出站时才报错。
	if amountMicro%billing.MicroPerCent != 0 {
		return nil, billing.PaymentResult{}, fmt.Errorf("%w: amount %s is not a whole number of cents",
			ErrInvalidRequest, billing.FormatMicro(amountMicro))
	}

	order := &store.PaymentOrder{
		OutTradeNo:  "p" + shortID(),
		SubjectType: subjectType,
		SubjectID:   subjectID,
		Channel:     string(channel),
		AmountMicro: amountMicro,
		Currency:    s.cfg.Currency,
		Title:       s.cfg.SubjectPrefix + billing.FormatMicro(amountMicro),
		Status:      billing.OrderStatusPending,
		ClientIP:    clientIP,
		ExpiresAt:   s.now().Add(s.cfg.OrderTTL),
	}
	if err := s.orders.CreateOrder(nil, order); err != nil {
		return nil, billing.PaymentResult{}, err
	}

	result, err := s.gateway.Create(ctx, billing.CreatePaymentRequest{
		OutTradeNo:  order.OutTradeNo,
		Channel:     channel,
		AmountMicro: order.AmountMicro,
		Title:       order.Title,
		ClientIP:    clientIP,
	})
	if err != nil {
		// 订单留着让 Sweep 关掉：单号已经用出去了，删行反而容易把号复用出去。
		logging.Errorf("billing payment: create payment for %s failed: %v", order.OutTradeNo, err)
		return nil, billing.PaymentResult{}, err
	}

	logging.Infof("billing payment: order %s created: channel=%s amount=%s",
		order.OutTradeNo, order.Channel, billing.FormatMicro(order.AmountMicro))
	return order, result, nil
}

// GetOrder 按商户单号查订单。归属校验由调用方做（HTTP 层按 token 里的 user id 比对
// subject_id），这一层不认识 HTTP 身份。
func (s *PaymentService) GetOrder(outTradeNo string) (*store.PaymentOrder, error) {
	return s.orders.GetOrder(outTradeNo)
}

// ListOrders 分页返回充值订单（用户看自己的，管理端看全部，过滤条件由调用方给）。
func (s *PaymentService) ListOrders(q store.PaymentOrderQuery) ([]store.PaymentOrder, int64, error) {
	orders, err := s.orders.ListOrders(q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.orders.CountOrders(q)
	if err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

// RefundRecharge 全额退款一笔已入账的充值。
//
// 顺序是「先让网关退钱，再改自己的账」：网关那边成功之前，我们这边一分钱都不动。
// 反过来的话，网关调用挂了而账已经退了，钱就凭空多出来了。
//
// 三个步骤各自幂等：
//
//  1. 网关退款——对面通常会对同一笔订单拒绝二次退款（“已退款”），这是它那边的天然幂等；
//  2. 写 refund 流水——幂等键 refund:order:<out_trade_no>；
//  3. 订单 credited → refunded——条件更新，重复执行只会影响 0 行。
//
// 残留窗口：第 2 步失败时钱已经退给了付款人，但我们的账还没动。这时**只能靠人工
// 介入**（日志里是 error 级），所以第 2 步之前先查一次 LedgerExists，把重试短路掉，
// 别让运维「重试一下」变成在网关那里再退一次。
//
// 只支持整单退款：部分退款会让订单状态与实退金额对不上，那需要一张单独的退款单表。
func (s *PaymentService) RefundRecharge(ctx context.Context, outTradeNo, note, operator string) error {
	order, err := s.orders.GetOrder(outTradeNo)
	if err != nil {
		return err
	}

	// paid 但还没入账（sweeper 没跑到）：先把账补上再退，否则没有可退的钱。
	if order.Status == billing.OrderStatusPaid {
		if err := s.creditIfPaid(ctx, order.OutTradeNo); err != nil {
			return err
		}
		if order, err = s.orders.GetOrder(outTradeNo); err != nil {
			return err
		}
	}

	switch order.Status {
	case billing.OrderStatusCredited:
	case billing.OrderStatusRefunded:
		return nil // 幂等：已经退过了
	default:
		return fmt.Errorf("%w: order %s is %s, only a credited order can be refunded",
			billing.ErrIllegalOrderState, order.OutTradeNo, order.Status)
	}

	key := billing.RefundIdempotencyKey(order.OutTradeNo)
	alreadyRefunded, err := s.billing.Store().LedgerExists(key)
	if err != nil {
		return err
	}

	if !alreadyRefunded {
		if _, err := s.gateway.Refund(ctx, billing.RefundRequest{
			OutTradeNo:  order.OutTradeNo,
			TradeNo:     order.GatewayTradeNo,
			AmountMicro: order.AmountMicro,
		}); err != nil {
			return fmt.Errorf("billing payment: gateway refund %s: %w", order.OutTradeNo, err)
		}

		if _, err := s.billing.Refund(ctx, RefundRequest{
			SubjectType: order.SubjectType,
			SubjectID:   order.SubjectID,
			AmountMicro: order.AmountMicro,
			RefID:       order.OutTradeNo,
			Note:        refundNote(order, note),
			Creator:     operator,
		}); err != nil {
			// 钱已经退出去了，账却没动。这个窗口手工介入。
			logging.Errorf("billing payment: order %s was refunded at the gateway but the ledger write failed: %v",
				order.OutTradeNo, err)
			return fmt.Errorf("billing payment: credit back for %s failed (the gateway refund already went through): %w", order.OutTradeNo, err)
		}
	} else {
		logging.Warnf("billing payment: refund ledger %s already exists, advancing order %s", key, order.OutTradeNo)
	}

	if err := s.orders.MarkOrderRefunded(order.OutTradeNo, s.now()); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil // 并发下已经有人把它推上去了
		}
		return err
	}

	logging.Infof("billing payment: order %s refunded %s", order.OutTradeNo, billing.FormatMicro(order.AmountMicro))
	return nil
}

// refundNote 拼退款流水的备注。操作者填的那句必须留在账上——将来查“这笔钱为什么
// 退了”时，只看到系统自动生成的半句话是不够的。
func refundNote(order *store.PaymentOrder, note string) string {
	base := fmt.Sprintf("退款 %s（%s，%s）", billing.FormatMicro(order.AmountMicro), order.OutTradeNo, order.Channel)
	if strings.TrimSpace(note) == "" {
		return base
	}
	return base + "：" + strings.TrimSpace(note)
}

// HandleNotify 处理网关的异步通知。
//
// 返回值只有两种含义：nil = 回 "success"（网关不用再发了），error = 回 "fail"
// （让网关重试）。所以只有「验签没过 / 请求体坏了 / 数据库临时故障」才返回 error；
// 金额对不上、订单不认识、状态冲突这些**重试也解决不了**的问题一律记日志后返回
// nil，否则网关会重发到天荒地老。
func (s *PaymentService) HandleNotify(ctx context.Context, params map[string]string) error {
	event, err := s.gateway.VerifyNotify(params)
	if err != nil {
		return err
	}

	// 非成功状态（已关闭、已退款）不是入账信号，记一行就收工。
	if event.Status != "" && event.Status != billing.TradeStatusSuccess {
		logging.Infof("billing payment: notify ignored: out_trade_no=%s trade_status=%s", event.OutTradeNo, event.Status)
		return nil
	}

	var (
		unknown  bool
		conflict error
	)
	err = s.orders.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		order, err := s.orders.LockOrder(tx, event.OutTradeNo)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				unknown = true
				return nil
			}
			return err
		}

		// 先落 append-only 的事实，再动状态：排障时要能看到网关到底发了几次、长什么样。
		if err := s.orders.InsertNotification(tx, &store.PaymentNotification{
			OutTradeNo:     event.OutTradeNo,
			GatewayTradeNo: event.TradeNo,
			AmountRaw:      event.AmountRaw,
			Status:         event.Status,
			Payload:        encodeNotifyParams(params),
		}); err != nil {
			return err
		}

		switch order.Status {
		case billing.OrderStatusPaid, billing.OrderStatusCredited:
			return nil // 重复通知：钱已经收过了
		case billing.OrderStatusRefunded:
			conflict = fmt.Errorf("notify for refunded order %s", order.OutTradeNo)
			return nil
		}

		// 金额必须与订单一致。回调里的金额永远不参与入账，它只用来核对订单——
		// 真正的入账金额取自 order.AmountMicro。
		if order.AmountMicro != event.AmountMicro {
			conflict = fmt.Errorf("%w: out_trade_no=%s order_micro=%d notify_micro=%d",
				billing.ErrAmountMismatch, order.OutTradeNo, order.AmountMicro, event.AmountMicro)
			return nil
		}

		return s.orders.MarkOrderPaid(tx, order.OutTradeNo, event.TradeNo, s.now())
	})
	if err != nil {
		return err
	}

	if unknown {
		logging.Errorf("billing payment: notify for unknown order: out_trade_no=%s trade_no=%s", event.OutTradeNo, event.TradeNo)
		return nil
	}
	if conflict != nil {
		logging.Errorf("billing payment: notify conflict: %v", conflict)
		return nil
	}

	return s.creditIfPaid(ctx, event.OutTradeNo)
}

// creditIfPaid 把「已收款」变成「已入账」。
//
// 幂等靠两件事：流水表的 idempotency_key 唯一索引（真正的闸），以及入账前先查一次
// LedgerExists——Credit 撞唯一索引会返回错误，而「已经入过账」不是失败，先查一下
// 比去解析驱动层的错误码可靠。
//
// 入账失败会让回调返回 fail、网关重发，或者由 Sweep 反复扫 paid 未入账的订单，
// 两条路都会回到这里。
func (s *PaymentService) creditIfPaid(ctx context.Context, outTradeNo string) error {
	order, err := s.orders.GetOrder(outTradeNo)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
	if order.Status != billing.OrderStatusPaid {
		// pending 还没收到钱；credited 已经入过账；closed / refunded 不该走到这里。
		return nil
	}

	key := billing.RechargeIdempotencyKey(order.OutTradeNo)
	exists, err := s.billing.Store().LedgerExists(key)
	if err != nil {
		return err
	}

	if exists {
		// 流水在、订单还停在 paid：上次入完账就挂了，只差把状态推上去。
		logging.Warnf("billing payment: ledger %s already exists, advancing order %s", key, order.OutTradeNo)
	} else {
		if _, err := s.billing.Credit(ctx, CreditRequest{
			SubjectType: order.SubjectType,
			SubjectID:   order.SubjectID,
			AmountMicro: order.AmountMicro,
			RefType:     billing.RefOrder,
			RefID:       order.OutTradeNo,
			Note:        fmt.Sprintf("充值 %s（%s）", billing.FormatMicro(order.AmountMicro), order.Channel),
			Creator:     "payment",
		}); err != nil {
			return fmt.Errorf("billing payment: credit order %s: %w", outTradeNo, err)
		}
		logging.Infof("billing payment: order %s credited %s", order.OutTradeNo, billing.FormatMicro(order.AmountMicro))
	}

	if err := s.orders.MarkOrderCredited(order.OutTradeNo, s.now()); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil // 并发下已经被别人推上去了
		}
		return err
	}
	return nil
}

// StartSweeper 起一个对账 goroutine，每 interval 跑一次，直到 ctx 取消。
// 和结算 worker 同进程（manager），退出同样靠 ctx。
func (s *PaymentService) StartSweeper(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweepCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				s.Sweep(sweepCtx)
				cancel()
			}
		}
	}()
}

// Sweep 是「进程在中间挂掉」的兜底，两件事：
//
//  1. paid 但没入账 → 补入账；
//  2. 过期还没支付 → 主动查一次单，网关说付了就当补一条事实，没付就关单。
//
// 回调可能丢（网关没发、网络断了、我们重启了），所以第 2 条不是可选项：只靠回调
// 一定会有用户付了钱而余额没到账的单子。
func (s *PaymentService) Sweep(ctx context.Context) {
	if orders, err := s.orders.ListPaidUncreditedOrders(100); err != nil {
		logging.Errorf("billing payment: sweep list paid-uncredited: %v", err)
	} else {
		for i := range orders {
			if err := s.creditIfPaid(ctx, orders[i].OutTradeNo); err != nil {
				logging.Errorf("billing payment: sweep credit %s: %v", orders[i].OutTradeNo, err)
			}
		}
	}

	expired, err := s.orders.ListExpiredOrders(s.now(), 100)
	if err != nil {
		logging.Errorf("billing payment: sweep list expired: %v", err)
		return
	}
	for i := range expired {
		s.reconcileExpired(ctx, expired[i])
	}
}

// reconcileExpired 处理一笔过期未支付的订单。
func (s *PaymentService) reconcileExpired(ctx context.Context, order store.PaymentOrder) {
	// 过期太久就不查网关了（省调用），直接关掉。真要晚到一笔成功通知，
	// HandleNotify 会把它接住——closed 状态的订单照样能转 paid。
	if s.now().Sub(order.ExpiresAt) > 24*time.Hour {
		if err := s.orders.CloseOrder(order.OutTradeNo); err != nil {
			logging.Errorf("billing payment: sweep close %s: %v", order.OutTradeNo, err)
		}
		return
	}

	remote, err := s.gateway.Query(ctx, order.OutTradeNo)
	if err != nil {
		// 查不到就先留着，下一轮再说：关单的语义是「确认没收到钱」，不是「问不到就当你没付」。
		logging.Warnf("billing payment: sweep query %s: %v", order.OutTradeNo, err)
		return
	}

	if !remote.Paid {
		if err := s.orders.CloseOrder(order.OutTradeNo); err != nil {
			logging.Errorf("billing payment: sweep close %s: %v", order.OutTradeNo, err)
		}
		return
	}

	if remote.AmountMicro != order.AmountMicro {
		logging.Errorf("billing payment: sweep amount mismatch: out_trade_no=%s order_micro=%d gateway_micro=%d",
			order.OutTradeNo, order.AmountMicro, remote.AmountMicro)
		return
	}

	if err := s.orders.MarkOrderPaid(nil, order.OutTradeNo, remote.TradeNo, s.now()); err != nil {
		logging.Errorf("billing payment: sweep mark paid %s: %v", order.OutTradeNo, err)
		return
	}
	logging.Warnf("billing payment: order %s was paid but no usable notify arrived; reconciled from gateway", order.OutTradeNo)

	if err := s.creditIfPaid(ctx, order.OutTradeNo); err != nil {
		logging.Errorf("billing payment: sweep credit %s: %v", order.OutTradeNo, err)
	}
}

// encodeNotifyParams 把回调参数编成可读的一行，作为原始事实留档。
// 里面只有本次回调的字段与签名，不含商户密钥。
func encodeNotifyParams(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	return strings.Join(parts, "&")
}
