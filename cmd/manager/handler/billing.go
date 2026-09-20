package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/billing/service"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

// 计费的三个 HTTP 面（docs/billing-design.md §6.1 / §8）：
//
//   - InternalBillingHandler：数据面（wsserver）调的 /internal/billing/*，只挂内部
//     鉴权，不挂 JWT；
//   - BillingAdminHandler：管理端（is_admin）的定价目录、价格版本、账户、人工调整、
//     流水与报表；
//   - BillingUserHandler：用户端（JWT）的余额、用量明细与价格公示。
//
// 这一层只做参数归一与错误映射，一分钱的判断都不在这里：金额、免费额度、阶梯、
// 透支策略全是 service 的事（§6.4）。所有 handler 在 svc 为 nil（billing.enabled:
// false）时回 503。

func NewInternalBillingHandler(svc *service.Service) *InternalBillingHandler {
	return &InternalBillingHandler{svc: svc}
}

func NewBillingAdminHandler(svc *service.Service) *BillingAdminHandler {
	return &BillingAdminHandler{svc: svc}
}

func NewBillingUserHandler(svc *service.Service) *BillingUserHandler {
	return &BillingUserHandler{svc: svc}
}

// billingReady 判断计费服务是否可用。计费关闭时路由仍然注册着（前端不用为此分支），
// 但每个请求都回 503——响应里直说是没开，而不是假装查不到。
func billingReady(c *gin.Context, svc *service.Service) bool {
	if svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "billing is disabled"})
		return false
	}
	return true
}

// writeBillingError 把 service 的错误映射成 HTTP 状态码。
//
// Authorize 的“业务拒绝”不走这里：allowed=false 是业务判断不是传输错误，HTTP 200
// 能让数据面少一条错误分支（§14.3）。
func writeBillingError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrBatchTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidRequest):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrInsufficientBalance):
		// 402 还是 409：选 409。402 Payment Required 在 HTTP 里的语义一直是“先付钱
		// 再给内容”，浏览器与代理对它的处理历史上很暧昧；而这里要表达的是“当前账户
		// 状态与这次请求冲突，充值或调额后重试即可”，那是 409 的语义。
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrItemNotFound),
		errors.Is(err, service.ErrAccountNotFound),
		errors.Is(err, service.ErrReservationNotFound),
		errors.Is(err, store.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		logging.Errorf("billing: request failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// bindBillingJSON 绑定请求体，失败回 400。
func bindBillingJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return false
	}
	return true
}

// ---------------------------------------------------------------- 内部接口（数据面）

// InternalBillingHandler 是数据面调的三个内部接口（§6.1）。
//
// 路径常量取自 internal/billing（billing.PathAuthorize / PathUsageEvents /
// PathSettle），server.go 把开头的 /internal 去掉后挂在 /internal 组下，并在这三条
// 上挂 middleware.InternalAuth。存量 /internal/* 接口不挂鉴权，那是 §14.1 里说好
// 的 P1 范围。
type InternalBillingHandler struct {
	svc *service.Service
}

// Authorize POST /internal/billing/authorize
//
// 请求只有 device_id / session_id / channel，账户与价格由控制面自己推（§14.2）。
func (h *InternalBillingHandler) Authorize(c *gin.Context) {
	if !billingReady(c, h.svc) {
		return
	}
	var req billing.AuthorizeRequest
	if !bindBillingJSON(c, &req) {
		return
	}
	resp, err := h.svc.Authorize(c.Request.Context(), req)
	if err != nil {
		writeBillingError(c, err)
		return
	}
	// 被拒也是 200：{"allowed":false,"reject_reason":"insufficient_balance"}（§14.3）。
	c.JSON(http.StatusOK, resp)
}

// UsageEvents POST /internal/billing/usage-events
//
// 落库就返回，不做定价：定价要匹配价格、要锁账户行，塞进上报路径等于让语音热路径
// 去等数据库的锁（§14.4）。
func (h *InternalBillingHandler) UsageEvents(c *gin.Context) {
	if !billingReady(c, h.svc) {
		return
	}
	// 单批上限 256KB，超了整批拒绝：半批成功会让重试语义变复杂。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, billing.MaxUsageBatchBytes)
	var req billing.UsageBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": fmt.Sprintf("batch exceeds %d bytes", billing.MaxUsageBatchBytes),
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := h.svc.ReportUsage(c.Request.Context(), req)
	if err != nil {
		writeBillingError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// Settle POST /internal/billing/settle
//
// 幂等键是 settle:<session_id>，重复调用返回第一次的结果（§14.5）。
func (h *InternalBillingHandler) Settle(c *gin.Context) {
	if !billingReady(c, h.svc) {
		return
	}
	var req billing.SettleRequest
	if !bindBillingJSON(c, &req) {
		return
	}
	resp, err := h.svc.Settle(c.Request.Context(), req)
	if err != nil {
		writeBillingError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// ---------------------------------------------------------------- 管理端（is_admin）

// BillingAdminHandler 是管理面的计费接口（§8）。
type BillingAdminHandler struct {
	svc *service.Service
}

// GET /api/billing/items — 计费项目录。系统内置项只读，但 enabled 可以改（互斥
// 口径的两组计费项就靠它只启用一组）。
func (h *BillingAdminHandler) Items(c *gin.Context) {
	if !billingReady(c, h.svc) {
		return
	}
	items, err := h.svc.Items(c.Request.Context())
	if err != nil {
		writeBillingError(c, err)
		return
	}
	if items == nil {
		items = []store.BillingItem{}
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

type billingItemUpdate struct {
	Enabled *bool `json:"enabled"`
}

// PUT /api/billing/items/:code — 启停一个计费项。
func (h *BillingAdminHandler) SetItem(c *gin.Context) {
	if !billingReady(c, h.svc) {
		return
	}
	var req billingItemUpdate
	if !bindBillingJSON(c, &req) {
		return
	}
	if req.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enabled is required"})
		return
	}
	code := c.Param("code")
	if err := h.svc.SetItemEnabled(c.Request.Context(), code, *req.Enabled); err != nil {
		writeBillingError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": code, "enabled": *req.Enabled})
}

// GET /api/billing/prices?item_code=&account_id=&resource_type=&resource_id=&active_only=1&page=&page_size=
//
// 管理端看的是全部价格版本（含还没生效的）。分页只给 limit/offset：价格表按 scope
// 唯一，量级是“几十条版本”，不值得为它加一个 COUNT。
func (h *BillingAdminHandler) ListPrices(c *gin.Context) {
	if !billingReady(c, h.svc) {
		return
	}
	page, pageSize := parsePageParams(c)
	q := store.PriceQuery{
		ItemCode:     strings.TrimSpace(c.Query("item_code")),
		AccountID:    strings.TrimSpace(c.Query("account_id")),
		ResourceType: strings.TrimSpace(c.Query("resource_type")),
		ResourceID:   strings.TrimSpace(c.Query("resource_id")),
		Limit:        pageSize,
		Offset:       (page - 1) * pageSize,
	}
	if activeOnly, err := strconv.ParseBool(strings.TrimSpace(c.Query("active_only"))); err == nil && activeOnly {
		now := h.svc.Config().Now()
		q.ActiveAt = &now
	}
	prices, err := h.svc.Prices(c.Request.Context(), q)
	if err != nil {
		writeBillingError(c, err)
		return
	}
	if prices == nil {
		prices = []store.BillingPrice{}
	}
	c.JSON(http.StatusOK, gin.H{"items": prices, "page": page, "page_size": pageSize})
}

// POST /api/billing/prices — 落一版新价格。
//
// 改价 = 新增一条 effective_from 更晚的版本，不覆盖历史：历史账单永远按当时的
// 价格解释（§3.2）。阶梯与起步价的不变量在 service 里校验，错了在录入时就报。
func (h *BillingAdminHandler) CreatePrice(c *gin.Context) {
	if !billingReady(c, h.svc) {
		return
	}
	var req store.BillingPrice
	if !bindBillingJSON(c, &req) {
		return
	}
	price, err := h.svc.CreatePrice(c.Request.Context(), req)
	if err != nil {
		writeBillingError(c, err)
		return
	}
	c.JSON(http.StatusCreated, price)
}

// billingPriceUpdate 是价格版本的局部更新。用结构体而不是 map[string]any：JSON 里
// 写错的字段名会直接 400，而不是变成一句写不进数据库的 GORM Updates。
type billingPriceUpdate struct {
	Currency       *string              `json:"currency"`
	UnitPriceMicro *int64               `json:"unit_price_micro"`
	UnitSize       *int64               `json:"unit_size"`
	MinChargeMicro *int64               `json:"min_charge_micro"`
	Rounding       *string              `json:"rounding"`
	Tiers          *[]store.BillingTier `json:"tiers"`
	EffectiveFrom  *time.Time           `json:"effective_from"`
	EffectiveTo    *time.Time           `json:"effective_to"`
}

func (u billingPriceUpdate) updates() map[string]any {
	updates := make(map[string]any, 8)
	if u.Currency != nil {
		updates["currency"] = *u.Currency
	}
	if u.UnitPriceMicro != nil {
		updates["unit_price_micro"] = *u.UnitPriceMicro
	}
	if u.UnitSize != nil {
		updates["unit_size"] = *u.UnitSize
	}
	if u.MinChargeMicro != nil {
		updates["min_charge_micro"] = *u.MinChargeMicro
	}
	if u.Rounding != nil {
		updates["rounding"] = *u.Rounding
	}
	if u.Tiers != nil {
		updates["tiers"] = store.BillingTiers(*u.Tiers)
	}
	if u.EffectiveFrom != nil {
		updates["effective_from"] = *u.EffectiveFrom
	}
	if u.EffectiveTo != nil {
		updates["effective_to"] = *u.EffectiveTo
	}
	return updates
}

// PUT /api/billing/prices/:id
//
// 要停用一版已经生效的价格，把 effective_to 收到当前时刻（不删行，历史账单还指着
// 它）；要取消一版还没生效的价格，用 DELETE。
func (h *BillingAdminHandler) UpdatePrice(c *gin.Context) {
	if !billingReady(c, h.svc) {
		return
	}
	var req billingPriceUpdate
	if !bindBillingJSON(c, &req) {
		return
	}
	updates := req.updates()
	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no updatable field provided"})
		return
	}
	price, err := h.svc.UpdatePrice(c.Request.Context(), c.Param("id"), updates)
	if err != nil {
		writeBillingError(c, err)
		return
	}
	c.JSON(http.StatusOK, price)
}

// DELETE /api/billing/prices/:id — 只能删还没生效过的价格。
func (h *BillingAdminHandler) DeletePrice(c *gin.Context) {
	if !billingReady(c, h.svc) {
		return
	}
	if err := h.svc.DeletePrice(c.Request.Context(), c.Param("id")); err != nil {
		writeBillingError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// GET /api/billing/accounts?subject_type=&status=&keyword=&page=&page_size=
func (h *BillingAdminHandler) Accounts(c *gin.Context) {
	if !billingReady(c, h.svc) {
		return
	}
	page, pageSize := parsePageParams(c)
	list, total, err := h.svc.Accounts(c.Request.Context(), store.AccountQuery{
		SubjectType: strings.TrimSpace(c.Query("subject_type")),
		Status:      strings.TrimSpace(c.Query("status")),
		Keyword:     strings.TrimSpace(c.Query("keyword")),
		Limit:       pageSize,
		Offset:      (page - 1) * pageSize,
	})
	if err != nil {
		writeBillingError(c, err)
		return
	}
	if list == nil {
		list = []store.BillingAccount{}
	}
	c.JSON(http.StatusOK, gin.H{"items": list, "total": total, "page": page, "page_size": pageSize})
}

// billingAdjustRequest 是一次人工调整请求。金额可正可负，note 必填（service 会再
// 校验一次）。
type billingAdjustRequest struct {
	AmountMicro int64  `json:"amount_micro"`
	ItemCode    string `json:"item_code,omitempty"`
	Note        string `json:"note"`
	Grant       bool   `json:"grant,omitempty"`
	// RefID 是赠款的幂等来源（同 account + item + ref_id 只发一次），留空则每次
	// 调整都是新的一笔——重试管理端请求时带上它才安全。
	RefID string `json:"ref_id,omitempty"`
}

// POST /api/billing/accounts/:id/adjust
//
// 账户按内部 ID（billing_accounts.id）定位，计费主体从这一行读出来，不接受请求体
// 另报一份 subject_*：多一个可以填错的字段，就多一条给别人的账户改钱的路（§19：
// 只有计费写钱，管理端只调它的接口）。
func (h *BillingAdminHandler) AdjustAccount(c *gin.Context) {
	if !billingReady(c, h.svc) {
		return
	}
	var req billingAdjustRequest
	if !bindBillingJSON(c, &req) {
		return
	}
	account, err := h.svc.Store().GetAccount(c.Param("id"))
	if err != nil {
		writeBillingError(c, err)
		return
	}

	ctx := c.Request.Context()
	if req.Grant {
		// 赠款走 billing_grants，不写进 balance_micro——否则赠款和充值款在账上就
		// 分不开了（§12）。
		if req.AmountMicro <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "grant requires a positive amount_micro"})
			return
		}
		if strings.TrimSpace(req.ItemCode) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "grant requires item_code"})
			return
		}
		source := strings.TrimSpace(req.RefID)
		if source == "" {
			source = "admin:" + uuid.NewString()
		}
		updated, err := h.svc.Grant(ctx, account.SubjectType, account.SubjectID, req.ItemCode, req.AmountMicro, source, req.Note)
		if err != nil {
			writeBillingError(c, err)
			return
		}
		c.JSON(http.StatusOK, updated)
		return
	}

	updated, err := h.svc.Adjust(ctx, service.AdjustRequest{
		SubjectType: account.SubjectType,
		SubjectID:   account.SubjectID,
		ItemCode:    req.ItemCode,
		AmountMicro: req.AmountMicro,
		Note:        req.Note,
		Creator:     middleware.UserID(c),
	})
	if err != nil {
		writeBillingError(c, err)
		return
	}
	c.JSON(http.StatusOK, updated)
}

// GET /api/billing/ledger?account_id=&item_code=&kind=&from=&to=&page=&page_size=
//
// 流水是 append-only 的事实，所以这里只有读；退款与人工调整写的是新的 adjust 流水，
// 不动历史行（§16.2）。
func (h *BillingAdminHandler) Ledger(c *gin.Context) {
	if !billingReady(c, h.svc) {
		return
	}
	from, to, err := billingTimeRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	page, pageSize := parsePageParams(c)
	list, total, err := h.svc.Ledger(c.Request.Context(), store.LedgerQuery{
		AccountID: strings.TrimSpace(c.Query("account_id")),
		ItemCode:  strings.TrimSpace(c.Query("item_code")),
		Kind:      strings.TrimSpace(c.Query("kind")),
		From:      optionalTime(from),
		To:        optionalTime(to),
		Limit:     pageSize,
		Offset:    (page - 1) * pageSize,
	})
	if err != nil {
		writeBillingError(c, err)
		return
	}
	if list == nil {
		list = []store.BillingLedger{}
	}
	c.JSON(http.StatusOK, gin.H{"items": list, "total": total, "page": page, "page_size": pageSize})
}

// GET /api/billing/stats?account_id=&from=&to=
//
// 报表口径（P1 就这两块，够排查用）：
//   - summary：账户余额 / 冻结 / 账期消耗 + Top 10 计费项（service.Summary）；
//   - usage_by_item：同一时间段按计费项聚合到 50 条（读的是已计费事件，未结算的
//     pending 事件不在里面，所以它和 ledger 的合计会差一个结算延迟）。
//
// account_id 必填：少了它就只能全表扫，那不是这个接口该干的事。
func (h *BillingAdminHandler) Stats(c *gin.Context) {
	if !billingReady(c, h.svc) {
		return
	}
	accountID := strings.TrimSpace(c.Query("account_id"))
	if accountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account_id is required"})
		return
	}
	account, err := h.svc.Store().GetAccount(accountID)
	if err != nil {
		writeBillingError(c, err)
		return
	}
	from, to, err := billingTimeRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	from, to = h.defaultRange(from, to)

	summary, err := h.svc.Summary(c.Request.Context(), account.SubjectType, account.SubjectID, from, to)
	if err != nil {
		writeBillingError(c, err)
		return
	}
	byItem, err := h.svc.Store().SumUsageByItem(account.ID, from, to, 50)
	if err != nil {
		writeBillingError(c, err)
		return
	}
	if byItem == nil {
		byItem = []store.UsageAggregate{}
	}
	c.JSON(http.StatusOK, gin.H{
		"account":       account,
		"summary":       summary,
		"usage_by_item": byItem,
		"from":          from,
		"to":            to,
	})
}

// ---------------------------------------------------------------- 用户端（JWT）

// BillingUserHandler 是用户端的计费接口（§8）。
//
// 这里所有查询都以 token 里的 user id 为准，**不接受**调用方传账户或 subject：
// 账户只能由控制面自己推（§14.2），否则任何人带一个别人的 account_id 就能看别人
// 的消费明细。
type BillingUserHandler struct {
	svc *service.Service
}

// GET /api/billing/summary?from=&to= — 余额 + 账期内消耗 + 按计费项 Top。
// 缺省区间是本账期（自然月，按 billing.period_timezone）到现在。
func (h *BillingUserHandler) Summary(c *gin.Context) {
	if !billingReady(c, h.svc) {
		return
	}
	from, to, err := billingTimeRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	summary, err := h.svc.Summary(c.Request.Context(), billing.SubjectTypeUser, middleware.UserID(c), from, to)
	if err != nil {
		writeBillingError(c, err)
		return
	}
	c.JSON(http.StatusOK, summary)
}

// GET /api/billing/usage?item_code=&from=&to=&page=&page_size= — 自己的用量明细。
func (h *BillingUserHandler) Usage(c *gin.Context) {
	if !billingReady(c, h.svc) {
		return
	}
	from, to, err := billingTimeRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	page, pageSize := parsePageParams(c)

	// 账户查不到不是错误：还没产生过任何用量的人就是没有账户，给空列表比给 404 好
	// ——前端不用为这个分支写一段“其实我没问题”的处理。
	account, err := h.svc.Store().GetAccountBySubject(billing.SubjectTypeUser, middleware.UserID(c))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusOK, gin.H{
				"items":     []store.BillingUsageEvent{},
				"total":     int64(0),
				"page":      page,
				"page_size": pageSize,
			})
			return
		}
		writeBillingError(c, err)
		return
	}

	list, total, err := h.svc.UsageEvents(c.Request.Context(), store.UsageQuery{
		AccountID: account.ID,
		ItemCode:  strings.TrimSpace(c.Query("item_code")),
		From:      optionalTime(from),
		To:        optionalTime(to),
		Limit:     pageSize,
		Offset:    (page - 1) * pageSize,
	})
	if err != nil {
		writeBillingError(c, err)
		return
	}
	if list == nil {
		list = []store.BillingUsageEvent{}
	}
	c.JSON(http.StatusOK, gin.H{"items": list, "total": total, "page": page, "page_size": pageSize})
}

// GET /api/billing/prices — 当前生效的平台标准价（价格公示）。
// 账户协议价不外露：那是别人和平台的约定。
func (h *BillingUserHandler) Prices(c *gin.Context) {
	if !billingReady(c, h.svc) {
		return
	}
	prices, err := h.svc.PublicPrices(c.Request.Context())
	if err != nil {
		writeBillingError(c, err)
		return
	}
	if prices == nil {
		prices = []store.BillingPrice{}
	}
	c.JSON(http.StatusOK, gin.H{"items": prices})
}

// ---------------------------------------------------------------- 查询参数的小工具

// billingTimeRange 解析 from / to 两个查询参数（RFC3339）。返回零值表示“调用方该
// 用默认口径”，service.Summary 与 store 的过滤条件都认这个约定。
func billingTimeRange(c *gin.Context) (time.Time, time.Time, error) {
	from, err := parseBillingTime("from", c.Query("from"))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	to, err := parseBillingTime("to", c.Query("to"))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return from, to, nil
}

func parseBillingTime(name, raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be RFC3339 (e.g. 2026-09-01T00:00:00+08:00)", name)
	}
	return t, nil
}

func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// defaultRange 把缺省的 from/to 补成“本账期到现在”。Summary 内部也会补，但 store
// 的聚合查询要具体时刻，所以聚合类接口统一在这里补一次。
func (h *BillingAdminHandler) defaultRange(from, to time.Time) (time.Time, time.Time) {
	cfg := h.svc.Config()
	now := cfg.Now()
	if from.IsZero() {
		local := now.In(cfg.PeriodLocation)
		from = time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, cfg.PeriodLocation)
	}
	if to.IsZero() {
		to = now
	}
	return from, to
}
