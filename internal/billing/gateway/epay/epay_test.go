package epay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	sdk "github.com/liuscraft/epay-sdk-go"

	"github.com/liuscraft/orion-x/internal/billing"
)

const (
	testPID = 1001
	testKey = "epay-test-key"
)

// newTestGateway 直接构造 Gateway，绕过 New() 的 https 校验——httptest 给的是
// http://127.0.0.1:port，而 validate() 在测试里没意义（它只保证线上别用明文）。
func newTestGateway(t *testing.T, apiBaseURL string) *Gateway {
	t.Helper()
	client, err := sdk.NewClient(&sdk.Config{
		PID:        testPID,
		Key:        testKey,
		APIBaseURL: apiBaseURL,
		Timeout:    5,
	})
	if err != nil {
		t.Fatalf("new sdk client: %v", err)
	}
	return &Gateway{
		client:    client,
		pid:       testPID,
		notifyURL: "https://api.example.test/pay/epay/notify",
		returnURL: "https://www.example.test/billing/recharge/result",
	}
}

func TestConfigValidate(t *testing.T) {
	valid := Config{
		APIBaseURL: "https://pay.example.com",
		PID:        1001,
		Key:        "k",
		NotifyURL:  "https://api.example.com/pay/epay/notify",
	}
	if err := valid.validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	// 每一处改动都必须被拒：这些都是「跑起来了但收不到钱」的经典配置。
	broken := map[string]func(c *Config){
		"missing api url": func(c *Config) { c.APIBaseURL = "" },
		"plain http":      func(c *Config) { c.APIBaseURL = "http://pay.example.com" },
		"missing pid":     func(c *Config) { c.PID = 0 },
		"missing key":     func(c *Config) { c.Key = "" },
		"missing notify":  func(c *Config) { c.NotifyURL = "" },
	}
	for name, mutate := range broken {
		t.Run(name, func(t *testing.T) {
			cfg := valid
			mutate(&cfg)
			if err := cfg.validate(); err == nil {
				t.Fatalf("config should be rejected")
			}
		})
	}
}

func TestMoneyOf(t *testing.T) {
	// 整分是唯一能出站的金额：微单位比「分」更细，多出来的精度会被网关悄悄抹掉，
	// 所以必须在下单前就拒掉。
	if got, err := moneyOf(9_990_000); err != nil || got != 9.99 {
		t.Fatalf("moneyOf(9990000) = %v, %v; want 9.99", got, err)
	}
	if got, err := moneyOf(billing.MicroPerUnit); err != nil || got != 1 {
		t.Fatalf("moneyOf(1000000) = %v, %v; want 1", got, err)
	}

	for _, bad := range []int64{0, -1, 10, 1_005_000 /* ¥1.005，比「分」细 */} {
		if _, err := moneyOf(bad); err == nil {
			t.Errorf("moneyOf(%d) should fail", bad)
		}
	}
}

func TestCreate(t *testing.T) {
	var received url.Values
	var signOK bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != sdk.APIPathMapi {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		received = r.URL.Query()

		// 用 SDK 自己的签名器验一遍出站参数：出站签名错了，网关只会回一句
		// “签名错误”，在真实环境里很难往回调这两个包的接缝上想。
		params := make(map[string]string, len(received))
		for k, v := range received {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}
		signOK = sdk.NewSigner(testKey).Verify(params, params["sign"])

		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":     1,
			"trade_no": "20260920001",
			"payurl":   "https://pay.example.com/pay/20260920001",
			"qrcode":   "https://pay.example.com/qr/20260920001",
		})
	}))
	defer server.Close()

	gw := newTestGateway(t, server.URL)
	result, err := gw.Create(context.Background(), billing.CreatePaymentRequest{
		OutTradeNo:  "p0123456789abcdef0123",
		Channel:     billing.PayChannelWxpay,
		AmountMicro: 9_990_000,
		Title:       "余额充值 9.99",
		ClientIP:    "203.0.113.7",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !signOK {
		t.Fatal("outbound signature does not verify with the merchant key")
	}
	if result.PayURL == "" || result.QRCode == "" || result.TradeNo != "20260920001" {
		t.Fatalf("unexpected result: %+v", result)
	}

	// 金额、渠道、两个回调地址都必须原样出现在请求里。
	want := map[string]string{
		"pid":          "1001",
		"type":         sdk.PayTypeWxpay,
		"out_trade_no": "p0123456789abcdef0123",
		"money":        "9.99",
		"name":         "余额充值 9.99",
		"notify_url":   "https://api.example.test/pay/epay/notify",
		"return_url":   "https://www.example.test/billing/recharge/result",
		"clientip":     "203.0.113.7",
	}
	for k, v := range want {
		if got := received.Get(k); got != v {
			t.Errorf("outbound %s = %q, want %q", k, got, v)
		}
	}
}

func TestCreateRejectsUnknownChannel(t *testing.T) {
	gw := newTestGateway(t, "http://127.0.0.1:1")
	_, err := gw.Create(context.Background(), billing.CreatePaymentRequest{
		OutTradeNo:  "p1",
		Channel:     "epay:paypal",
		AmountMicro: billing.MicroPerUnit,
	})
	if !errors.Is(err, billing.ErrUnknownChannel) {
		t.Fatalf("Create error = %v, want ErrUnknownChannel", err)
	}
}

func TestQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("act"); got != "order" {
			t.Errorf("act = %q, want order", got)
		}
		if got := r.URL.Query().Get("out_trade_no"); got != "p0123456789abcdef0123" {
			t.Errorf("out_trade_no = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":         1,
			"trade_no":     "20260920001",
			"money":        "9.99",
			"status":       sdk.OrderStatusPaid,
			"type":         sdk.PayTypeAlipay,
			"addtime":      "2026-09-20 10:00:00",
			"name":         "余额充值",
			"out_trade_no": "p0123456789abcdef0123",
		})
	}))
	defer server.Close()

	order, err := newTestGateway(t, server.URL).Query(context.Background(), "p0123456789abcdef0123")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !order.Paid || order.AmountMicro != 9_990_000 || order.TradeNo != "20260920001" {
		t.Fatalf("unexpected order: %+v", order)
	}
}

// signedNotify 造一份「网关发过来的」回调参数（用同一个商户密钥签）。
func signedNotify(overrides map[string]string) map[string]string {
	params := map[string]string{
		"pid":          "1001",
		"trade_no":     "20260920001",
		"out_trade_no": "p0123456789abcdef0123",
		"type":         "alipay",
		"name":         "余额充值 9.99",
		"money":        "9.99",
		"trade_status": sdk.TradeStatusSuccess,
		"sign_type":    "MD5",
	}
	for k, v := range overrides {
		if v == "" {
			delete(params, k)
			continue
		}
		params[k] = v
	}
	params["sign"] = sdk.NewSigner(testKey).Sign(params)
	return params
}

func TestVerifyNotify(t *testing.T) {
	gw := newTestGateway(t, "http://127.0.0.1:1")

	event, err := gw.VerifyNotify(signedNotify(nil))
	if err != nil {
		t.Fatalf("VerifyNotify: %v", err)
	}
	if event.OutTradeNo != "p0123456789abcdef0123" || event.AmountMicro != 9_990_000 ||
		event.AmountRaw != "9.99" || event.Status != sdk.TradeStatusSuccess || event.TradeNo != "20260920001" {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestVerifyNotifyRejects(t *testing.T) {
	gw := newTestGateway(t, "http://127.0.0.1:1")

	t.Run("tampered money", func(t *testing.T) {
		params := signedNotify(nil)
		params["money"] = "0.01" // 改了金额却不重签
		_, err := gw.VerifyNotify(params)
		if !errors.Is(err, billing.ErrBadSignature) {
			t.Fatalf("err = %v, want ErrBadSignature", err)
		}
	})

	t.Run("missing sign", func(t *testing.T) {
		params := signedNotify(nil)
		delete(params, "sign")
		if _, err := gw.VerifyNotify(params); !errors.Is(err, billing.ErrBadSignature) {
			t.Fatalf("err = %v, want ErrBadSignature", err)
		}
	})

	t.Run("another merchant's pid", func(t *testing.T) {
		// 别人家商户的合法通知：签名是对的，但 pid 不是我们——不能收。
		params := signedNotify(map[string]string{"pid": "2002"})
		if _, err := gw.VerifyNotify(params); err == nil {
			t.Fatal("notify with foreign pid should be rejected")
		}
	})

	t.Run("missing out_trade_no", func(t *testing.T) {
		params := signedNotify(map[string]string{"out_trade_no": ""})
		if _, err := gw.VerifyNotify(params); err == nil {
			t.Fatal("notify without out_trade_no should be rejected")
		}
	})

	t.Run("sub-micro money", func(t *testing.T) {
		params := signedNotify(map[string]string{"money": "1.0000001"})
		if _, err := gw.VerifyNotify(params); err == nil {
			t.Fatal("notify with sub-micro money should be rejected")
		}
	})
}
