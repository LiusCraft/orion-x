package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/billing/service"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

// 充值链路的 HTTP 面（docs/billing-design.md §19）：
//
//	POST /api/billing/recharge           下单（JWT）
//	GET  /api/billing/recharge/:no       查单（JWT，只有本人能看）
//	GET  /api/billing/recharge/config    额度与可选支付方式（JWT）
//	POST|GET /pay/epay/notify            网关异步通知（匿名、公网可达）
//	GET  /pay/epay/return                网关同步跳转（匿名）
//
// 这一层只做参数归一与错误映射，一分钱的判断都不在这里：金额上下限、渠道白名单、
// 金额一致性、幂等与对账全是 service 的事。
//
// 返回给前端的文案要当客户界面写：配置项名、表名、包名、“网关 / 回调 / 流水”这些
// 我们自己的说法一律不出现在这里或者前端（service 的错误信息也一样，前端用
// userFacingError 拦内部前缀）。上下限和渠道清单这类要靠数字说话的校验，走
// RechargeConfig 下发给前端自己校验。
//
// 回调那两条**绝对不能挂 JWT**：网关不会带 token，挂上等于所有支付都收不到通知。
// 它们的安全性来自签名（service 里验）而不是身份。
type PaymentHandler struct {
	svc *service.PaymentService
}

func NewPaymentHandler(svc *service.PaymentService) *PaymentHandler {
	return &PaymentHandler{svc: svc}
}

// paymentReady 判断充值通道是否可用。没接支付渠道时路由仍然注册着（前端不用为此
// 写分支），每个请求回 503——直说是没开，而不是假装查不到。
func paymentReady(c *gin.Context, svc *service.PaymentService) bool {
	if svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "payment is disabled"})
		return false
	}
	return true
}

// writePaymentError 把 service 的错误映射成 HTTP 状态码。
func writePaymentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrPaymentDisabled):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidRequest), errors.Is(err, billing.ErrUnknownChannel):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, billing.ErrIllegalOrderState):
		// 状态冲突：比如退一笔还没到账的钱。充值或调额后重试就能解决，不是服务端故障。
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, store.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		logging.Errorf("billing payment: request failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

type rechargeRequest struct {
	Channel string `json:"channel" binding:"required"`
	// AmountMicro 是微单位金额（￥1 = 1000000），与计费其它接口一个口径。
	// 客户端只说「要充多少」，不能说要「付多少」——付多少由服务端算。
	AmountMicro int64 `json:"amount_micro" binding:"required"`
}

// CreateRecharge 下单：POST /api/billing/recharge。
// 返回 out_trade_no 与付款入口（pay_url 跳网页 / qrcode 扫码，取决于网关返回什么）。
func (h *PaymentHandler) CreateRecharge(c *gin.Context) {
	if !paymentReady(c, h.svc) {
		return
	}
	var req rechargeRequest
	if !bindBillingJSON(c, &req) {
		return
	}

	channel, err := billing.ParsePayChannel(req.Channel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 主体必须取自 token：接受调用方传 subject 等于让任何人给别人充值（或者反过来，
	// 用别人的账户下单）——账户只能由控制面自己推（§14.2）。
	order, result, err := h.svc.CreateRecharge(c.Request.Context(),
		billing.SubjectTypeUser, middleware.UserID(c), channel, req.AmountMicro, c.ClientIP())
	if err != nil {
		writePaymentError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"out_trade_no": order.OutTradeNo,
		"amount_micro": order.AmountMicro,
		"currency":     order.Currency,
		"channel":      order.Channel,
		"status":       order.Status,
		"expires_at":   order.ExpiresAt,
		"pay_url":      result.PayURL,
		"qrcode":       result.QRCode,
		"url_scheme":   result.URLScheme,
	})
}

// GetRecharge 查单：GET /api/billing/recharge/:out_trade_no。
// 前端的「支付完成」页轮询它，状态一律以服务端库为准。
func (h *PaymentHandler) GetRecharge(c *gin.Context) {
	if !paymentReady(c, h.svc) {
		return
	}

	order, err := h.svc.GetOrder(c.Param("out_trade_no"))
	if err != nil {
		writePaymentError(c, err)
		return
	}
	// 订单号是随机的，但它不是凭据：查到别人的单子不该在这条接口上回出状态。
	if order.SubjectType != billing.SubjectTypeUser || order.SubjectID != middleware.UserID(c) {
		c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"out_trade_no": order.OutTradeNo,
		"amount_micro": order.AmountMicro,
		"currency":     order.Currency,
		"channel":      order.Channel,
		"status":       order.Status,
		"expires_at":   order.ExpiresAt,
		"paid_at":      order.PaidAt,
		"credited_at":  order.CreditedAt,
	})
}

// Notify 处理网关的异步通知（POST form，也有实现用 GET query；无鉴权）。
//
// 响应体必须是**纯文本** success：不能包成 JSON、不能带额外空白。网关只认这个
// 字符串，认不出来就一直重发。回 fail 的语义是「请你重试」，所以只有服务端临时
// 故障才回它——金额对不上之类的业务冲突在 service 里已经被咽掉了。
func (h *PaymentHandler) Notify(c *gin.Context) {
	if h.svc == nil {
		c.String(http.StatusOK, "fail")
		return
	}

	params, err := notifyParams(c)
	if err != nil {
		logging.Errorf("billing payment: parse notify form: %v", err)
		c.String(http.StatusOK, "fail")
		return
	}

	if err := h.svc.HandleNotify(c.Request.Context(), params); err != nil {
		logging.Errorf("billing payment: notify rejected: %v", err)
		c.String(http.StatusOK, "fail")
		return
	}

	c.String(http.StatusOK, "success")
}

// Return 处理同步跳转：用户付完款被浏览器带回这里，我们把他送去前端结果页。
//
// return_url 上的参数不可信（可以伪造，也可能早于异步通知到达），所以只拿它取单号，
// 状态一律回库查——前端看到「已到账」的唯一依据是服务端订单状态。
func (h *PaymentHandler) Return(c *gin.Context) {
	outTradeNo := c.Query("out_trade_no")
	if outTradeNo == "" {
		c.Redirect(http.StatusFound, rechargeResultPath)
		return
	}

	var status string
	if h.svc != nil {
		order, err := h.svc.GetOrder(outTradeNo)
		switch {
		case err == nil:
			status = order.Status
		case errors.Is(err, store.ErrNotFound):
			logging.Warnf("billing payment: return for unknown order %s", outTradeNo)
		default:
			logging.Errorf("billing payment: return lookup %s: %v", outTradeNo, err)
		}
	}

	c.Redirect(http.StatusFound, rechargeResultPath+"?out_trade_no="+url.QueryEscape(outTradeNo)+"&status="+url.QueryEscape(status))
}

// rechargeResultPath 是同步跳转落地的前端路由（web/manager 的 `/billing/*` 一族）。
// 前端在那个页面上调 GET /api/billing/recharge/:out_trade_no 拿最终状态
// ——这个 status 参数只是个提示，页面不该只信它。
const rechargeResultPath = "/billing/recharge"

// RechargeConfig 返回充值页需要的额度与支付方式：GET /api/billing/recharge/config。
//
// 这些数只存在于服务端配置里。不下发的话，前端只能把上下限和渠道写死——写死的结果
// 就是「界面上能选、提交后被打回」，而且打回时只能把 "billing: amount must be
// between ..." 这种给我们自己看的错误直接贴给客户。
func (h *PaymentHandler) RechargeConfig(c *gin.Context) {
	if !paymentReady(c, h.svc) {
		return
	}
	cfg := h.svc.Config()
	channels := make([]string, 0, len(cfg.Channels))
	for _, ch := range cfg.Channels {
		channels = append(channels, string(ch))
	}
	c.JSON(http.StatusOK, gin.H{
		"currency":         cfg.Currency,
		"min_amount_micro": cfg.MinAmountMicro,
		"max_amount_micro": cfg.MaxAmountMicro,
		"channels":         channels,
	})
}

// ListRecharges 查充值订单列表：GET /api/billing/recharge。
//
// 普通用户只能看自己的（subject 取自 token，不接受调用方传）；admin 可以看全部，
// 并且能用 subject_id / status / channel / 关键字过滤——同一个路径只注册一条、按
// is_admin 分流，和 /billing/prices 一个路子（Gin 不允许同一路径注册两次）。
func (h *PaymentHandler) ListRecharges(c *gin.Context) {
	if !paymentReady(c, h.svc) {
		return
	}

	page, pageSize := parsePageParams(c)
	query := store.PaymentOrderQuery{
		Channel: c.Query("channel"),
		Status:  c.Query("status"),
		Keyword: strings.TrimSpace(c.Query("keyword")),
		Limit:   pageSize,
		Offset:  (page - 1) * pageSize,
	}
	if middleware.IsAdmin(c) {
		query.SubjectType = c.Query("subject_type")
		query.SubjectID = c.Query("subject_id")
	} else {
		query.SubjectType = billing.SubjectTypeUser
		query.SubjectID = middleware.UserID(c)
	}

	orders, total, err := h.svc.ListOrders(query)
	if err != nil {
		writePaymentError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items":     orders,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

type refundRechargeRequest struct {
	// Note 会原样记进 refund 流水：将来查「这笔钱为什么退了」时，
	// 只有系统自动生成的半句话是不够的。
	Note string `json:"note" binding:"required"`
}

// RefundRecharge 全额退款：POST /api/billing/recharge/:out_trade_no/refund（仅 admin）。
//
// 钱要真的退出去，所以它不是幂等的“改一个状态”，而是先调网关再写账；重复点只是
// 把已经退过的单子再确认一次（见 service.RefundRecharge 里的两个幂等键）。
func (h *PaymentHandler) RefundRecharge(c *gin.Context) {
	if !paymentReady(c, h.svc) {
		return
	}
	var req refundRechargeRequest
	if !bindBillingJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.Note) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "note is required for a refund"})
		return
	}

	outTradeNo := c.Param("out_trade_no")
	if err := h.svc.RefundRecharge(c.Request.Context(), outTradeNo, req.Note, middleware.UserID(c)); err != nil {
		writePaymentError(c, err)
		return
	}

	order, err := h.svc.GetOrder(outTradeNo)
	if err != nil {
		writePaymentError(c, err)
		return
	}
	c.JSON(http.StatusOK, order)
}

// notifyParams 把回调参数取平。ParseForm 对 GET 与 POST 都管：查询串进 Form，
// 表单体覆盖同名键。重复参数取第一个。
func notifyParams(c *gin.Context) (map[string]string, error) {
	if err := c.Request.ParseForm(); err != nil {
		return nil, err
	}
	params := make(map[string]string, len(c.Request.Form))
	for k, values := range c.Request.Form {
		if len(values) == 0 {
			continue
		}
		params[k] = values[0]
	}
	return params, nil
}
