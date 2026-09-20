package billing

import (
	"context"
	"errors"
	"fmt"
)

// 充值（支付渠道 → 余额）这一小块领域模型。
//
// 计费本身不管钱从哪来：它只认 Service.Credit 那句“一笔钱进来了”。这个文件定义
// 的是“钱还没进来之前”的状态机与出站网关的 port，同样零依赖——适配器实现在
// internal/billing/gateway/<vendor>，领域层不 import 它（§19）。

// PayChannel 是支付渠道。
//
// wire 值（alipay / wxpay / qqpay）是厂商格式、原样保留，映射在网关适配器里；
// 这里是我们自己的标识，按 AGENTS.md 用 `:` 分段。
type PayChannel string

const (
	PayChannelAlipay PayChannel = "epay:alipay"
	PayChannelWxpay  PayChannel = "epay:wxpay"
	PayChannelQqpay  PayChannel = "epay:qqpay"
)

// PayChannels 是内置渠道全集，供配置校验与前端展示用。
func PayChannels() []PayChannel {
	return []PayChannel{PayChannelAlipay, PayChannelWxpay, PayChannelQqpay}
}

// ParsePayChannel 解析渠道标识。前端传来的字符串必须先过这一关，未知取值直接拒绝，
// 不然它会一路透传到网关的 type 参数上。
func ParsePayChannel(value string) (PayChannel, error) {
	ch := PayChannel(value)
	switch ch {
	case PayChannelAlipay, PayChannelWxpay, PayChannelQqpay:
		return ch, nil
	}
	return "", fmt.Errorf("%w: %q", ErrUnknownChannel, value)
}

// 支付订单状态。
//
// pending → paid → credited 是正常路径，中间那步不能省：网关确认收款（paid）和
// 余额加上（credited）分开落库，进程在两步之间挂掉时 Sweep 能把账补齐（§14）。
// 状态字面量与 store 层保持一致（改一边要同步另一边）。
const (
	OrderStatusPending  = "order:pending"  // 已下单，等网关收款
	OrderStatusPaid     = "order:paid"     // 网关已确认收款，还没入账
	OrderStatusCredited = "order:credited" // 已入账，余额已加
	OrderStatusClosed   = "order:closed"   // 超时未支付；晚到的成功通知仍会接受
	OrderStatusRefunded = "order:refunded"
)

// TradeStatusSuccess 是网关回调里的“支付成功”。其余取值（关闭、退款等）重试也
// 解决不了，回调处理记一行日志就收工。
const TradeStatusSuccess = "TRADE_SUCCESS"

var (
	// ErrUnknownChannel 是不认识的支付渠道。
	ErrUnknownChannel = errors.New("billing: unknown pay channel")
	// ErrBadSignature 是回调签名验证失败。它必须回 fail：这条通知不可信。
	ErrBadSignature = errors.New("billing: bad notify signature")
	// ErrAmountMismatch 是回调金额与订单金额不一致。绝不允许按回调金额入账。
	ErrAmountMismatch = errors.New("billing: notify amount mismatch")
	// ErrIllegalOrderState 是订单当前状态不允许这个操作（比如退一笔还没入账的钱）。
	ErrIllegalOrderState = errors.New("billing: illegal payment order state")
)

// CreatePaymentRequest 是一次下单请求。AmountMicro 由服务端算（或按档位查表），
// 永远不接受客户端传来的金额。
//
// Title 是订单标题（网关收银台上显示的那句话）。不要把它叫 Subject——这个代码库里
// Subject 专指计费主体（subject_type / subject_id）。
type CreatePaymentRequest struct {
	OutTradeNo  string
	Channel     PayChannel
	AmountMicro int64
	Title       string
	ClientIP    string
}

// PaymentResult 是下单结果。PayURL 给网页跳转，QRCode 给扫码，付款方式取决于网关
// 返回了什么——两个都为空才算失败。
type PaymentResult struct {
	PayURL    string
	QRCode    string
	URLScheme string
	TradeNo   string
}

// GatewayOrder 是主动查单的结果，用于回调丢失时对账兜底。
type GatewayOrder struct {
	OutTradeNo  string
	TradeNo     string
	AmountMicro int64
	Paid        bool
	Status      string
}

// NotifyEvent 是适配器验签之后的回调事件。金额已由适配器解析成微单位：wire 格式
// 的解析只存在于适配器一处，service 不做字符串算术。
type NotifyEvent struct {
	OutTradeNo  string
	TradeNo     string
	Channel     string // 厂商 wire 值，原样带回，不参与判断
	AmountMicro int64
	AmountRaw   string // 原始字符串，只用于日志与对账留痕
	Status      string // 厂商 wire 值，如 TRADE_SUCCESS
}

// RefundRequest 是一次退款请求。Recharge 的反向操作，走 refund 流水而不是改历史账。
type RefundRequest struct {
	OutTradeNo  string
	TradeNo     string
	AmountMicro int64
}

// RefundResult 是退款结果。
type RefundResult struct {
	TradeNo string
	Message string
}

// PaymentGateway 是出站支付网关的 port。
//
// 实现方在 gateway/<vendor>：网关怎么签名、用什么字段、返回什么 JSON 都是它的事；
// 上层只关心“下单 / 查单 / 退款 / 验回调”这四件事。
//
// VerifyNotify 只验签和解析，不做任何业务判断——金额对不对、订单该不该收，是
// service 的事（它才知道订单长什么样）。
type PaymentGateway interface {
	Create(ctx context.Context, req CreatePaymentRequest) (PaymentResult, error)
	Query(ctx context.Context, outTradeNo string) (GatewayOrder, error)
	Refund(ctx context.Context, req RefundRequest) (RefundResult, error)
	VerifyNotify(params map[string]string) (NotifyEvent, error)
}
