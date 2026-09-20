// Package epay 是易支付（EPay）网关的适配器，实现 billing.PaymentGateway。
//
// 它是计费包里唯一 import 支付 SDK 的地方：换一个网关只换这个包，service /
// handler / store 一行都不用动。
//
// 出站调用与回调验签全部走 github.com/liuscraft/epay-sdk-go（MD5 签名：非空参数按
// key 升序拼 k=v&k=v，末尾直接接商户密钥）。
package epay

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	sdk "github.com/liuscraft/epay-sdk-go"

	"github.com/liuscraft/orion-x/internal/billing"
)

// Config 是接一个易支付网关需要的参数。
type Config struct {
	APIBaseURL string // 网关地址，如 https://pay.example.com（不带路径）
	PID        int    // 商户 ID
	Key        string // 商户密钥；只该来自环境变量，不要写进配置文件
	NotifyURL  string // 异步通知地址，必须公网可达
	ReturnURL  string // 同步跳转地址（前端结果页），可以为空
	Timeout    int    // 请求超时（秒），0 = SDK 默认 30s
	// Debug 打开 SDK 的请求日志。它会打印完整 URL（含签名）与响应，生产环境别开。
	Debug bool
}

func (c Config) validate() error {
	switch {
	case strings.TrimSpace(c.APIBaseURL) == "":
		return errors.New("epay: api_base_url is required")
	case !strings.HasPrefix(c.APIBaseURL, "https://"):
		// 查询/退款接口把商户密钥当参数发出去，明文 HTTP 等于泄露密钥
		return errors.New("epay: api_base_url must be https")
	case c.PID <= 0:
		return errors.New("epay: pid is required")
	case strings.TrimSpace(c.Key) == "":
		return errors.New("epay: key is required")
	case strings.TrimSpace(c.NotifyURL) == "":
		return errors.New("epay: notify_url is required and must be publicly reachable")
	}
	return nil
}

// Gateway 实现 billing.PaymentGateway。
type Gateway struct {
	client    *sdk.Client
	pid       int
	notifyURL string
	returnURL string
}

// New 组装网关客户端。配置不合法时直接报错：带着空密钥跑起来，失败会推迟到
// 用户点「充值」那一刻。
func New(cfg Config) (*Gateway, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	client, err := sdk.NewClient(&sdk.Config{
		PID:        cfg.PID,
		Key:        cfg.Key,
		APIBaseURL: cfg.APIBaseURL,
		Timeout:    cfg.Timeout,
		Debug:      cfg.Debug,
	})
	if err != nil {
		return nil, fmt.Errorf("epay: new client: %w", err)
	}
	return &Gateway{
		client:    client,
		pid:       cfg.PID,
		notifyURL: cfg.NotifyURL,
		returnURL: cfg.ReturnURL,
	}, nil
}

// wireType 把渠道标识映射成易支付的 type 参数。
func wireType(ch billing.PayChannel) (string, error) {
	switch ch {
	case billing.PayChannelAlipay:
		return sdk.PayTypeAlipay, nil
	case billing.PayChannelWxpay:
		return sdk.PayTypeWxpay, nil
	case billing.PayChannelQqpay:
		return sdk.PayTypeQQpay, nil
	default:
		return "", fmt.Errorf("%w: %q", billing.ErrUnknownChannel, ch)
	}
}

// moneyOf 把微单位金额转成 SDK 要的元。
//
// 微单位比「分」更细，而网关的 money 只有两位小数：不是整分的金额在这里被拒掉，
// 而不是被四舍五入成另一个数——金额只允许被拒绝，不允许被悄悄改写。
func moneyOf(micro int64) (float64, error) {
	if micro <= 0 {
		return 0, fmt.Errorf("billing: amount must be positive, got %s", billing.FormatMicro(micro))
	}
	if micro%billing.MicroPerCent != 0 {
		return 0, fmt.Errorf("billing: amount %s is not a whole number of cents", billing.FormatMicro(micro))
	}
	return float64(micro) / float64(billing.MicroPerUnit), nil
}

// Create 下单：调 mapi.php 拿二维码 / 跳转链接。
func (g *Gateway) Create(_ context.Context, req billing.CreatePaymentRequest) (billing.PaymentResult, error) {
	wire, err := wireType(req.Channel)
	if err != nil {
		return billing.PaymentResult{}, err
	}
	money, err := moneyOf(req.AmountMicro)
	if err != nil {
		return billing.PaymentResult{}, err
	}

	resp, err := g.client.CreatePayment(&sdk.PaymentRequest{
		Type:       wire,
		OutTradeNo: req.OutTradeNo,
		NotifyURL:  g.notifyURL,
		ReturnURL:  g.returnURL,
		Name:       req.Title,
		Money:      money,
		ClientIP:   req.ClientIP,
	})
	if err != nil {
		return billing.PaymentResult{}, fmt.Errorf("epay: create payment %s: %w", req.OutTradeNo, err)
	}
	if resp.PayURL == "" && resp.QRCode == "" && resp.URLScheme == "" {
		return billing.PaymentResult{}, fmt.Errorf("epay: create payment %s returned no pay target (trade_no=%s)",
			req.OutTradeNo, resp.TradeNo)
	}

	return billing.PaymentResult{
		PayURL:    resp.PayURL,
		QRCode:    resp.QRCode,
		URLScheme: resp.URLScheme,
		TradeNo:   resp.TradeNo,
	}, nil
}

// Query 主动查单，用于回调丢失时的对账兜底。
//
// 注意：SDK 的 QueryOrder 只带 pid + 签名，不带商户 key；如果对面这个易支付实现
// 要求 api.php?act=order 必须带 key，这里要改成自己发请求。
func (g *Gateway) Query(_ context.Context, outTradeNo string) (billing.GatewayOrder, error) {
	order, err := g.client.QueryOrder(&sdk.OrderQueryRequest{OutTradeNo: outTradeNo})
	if err != nil {
		return billing.GatewayOrder{}, fmt.Errorf("epay: query order %s: %w", outTradeNo, err)
	}

	micro, err := billing.ParseMicro(order.Money)
	if err != nil {
		return billing.GatewayOrder{}, fmt.Errorf("epay: query order %s money %q: %w", outTradeNo, order.Money, err)
	}

	return billing.GatewayOrder{
		OutTradeNo:  outTradeNo,
		TradeNo:     order.TradeNo,
		AmountMicro: micro,
		Paid:        sdk.IsOrderPaid(order),
		Status:      strconv.Itoa(order.Status),
	}, nil
}

// Refund 提交退款。
func (g *Gateway) Refund(_ context.Context, req billing.RefundRequest) (billing.RefundResult, error) {
	money, err := moneyOf(req.AmountMicro)
	if err != nil {
		return billing.RefundResult{}, err
	}

	resp, err := g.client.Refund(&sdk.RefundRequest{
		TradeNo:    req.TradeNo,
		OutTradeNo: req.OutTradeNo,
		Money:      money,
	})
	if err != nil {
		return billing.RefundResult{}, fmt.Errorf("epay: refund %s: %w", req.OutTradeNo, err)
	}
	if !sdk.IsRefundSuccess(resp) {
		return billing.RefundResult{}, fmt.Errorf("epay: refund %s rejected: %s", req.OutTradeNo, resp.Msg)
	}

	return billing.RefundResult{TradeNo: req.TradeNo, Message: resp.Msg}, nil
}

// VerifyNotify 验签并解析回调。
//
// SDK 只验签名、不看商户号，所以这里必须自己核对 pid：不核对的话，别人家商户的
// 合法通知也能让我们的订单变成已支付。
func (g *Gateway) VerifyNotify(params map[string]string) (billing.NotifyEvent, error) {
	data, err := g.client.VerifyNotify(params)
	if err != nil {
		return billing.NotifyEvent{}, fmt.Errorf("%w: %v", billing.ErrBadSignature, err)
	}
	if data.PID != g.pid {
		return billing.NotifyEvent{}, fmt.Errorf("epay: notify pid %d does not match configured %d", data.PID, g.pid)
	}
	if strings.TrimSpace(data.OutTradeNo) == "" {
		return billing.NotifyEvent{}, errors.New("epay: notify missing out_trade_no")
	}

	micro, err := billing.ParseMicro(data.Money)
	if err != nil {
		return billing.NotifyEvent{}, fmt.Errorf("epay: notify money %q: %w", data.Money, err)
	}

	return billing.NotifyEvent{
		OutTradeNo:  data.OutTradeNo,
		TradeNo:     data.TradeNo,
		Channel:     data.Type,
		AmountMicro: micro,
		AmountRaw:   data.Money,
		Status:      data.TradeStatus,
	}, nil
}
