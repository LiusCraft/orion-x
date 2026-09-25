package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

// API Key 的自助管理面（docs/api-key-design.md §5.3 的 HTTP API）：
//
//	GET    /api/api-keys?page=&page_size=  列出自己的 key（只有掩码）
//	POST   /api/api-keys                   创建（明文只在这一条响应里出现）
//	DELETE /api/api-keys/:id               撤销（软删，幂等）
//	GET    /api/api-keys/scopes            scope 目录与预设（前端渲染表单用）
//
// 这一层只做三件事：参数归一、错误映射、组织响应体。生成规则、校验、限流、计数全在
// internal/apikey；一次都不在这里判断"这把 key 能不能用"。
//
// 参数必须全部取自 token（middleware.UserID）：接受调用方传 user_id 等于让任何人
// 管理别人的 key。这四条路由**不挂 middleware.Auth**（只挂 JWT）——机器凭证不能
// 给自己或别人签发凭证，这是路由表里的"key 不可达"白名单成员：一把泄漏的 key 若
// 能造 key，撤销原 key 就挡不住攻击者（持久化后门）。
type APIKeyHandler struct {
	svc *apikey.Service
	// adminOnly 是灰度期的收口（§7.3）：true 时只有管理员能创建新 Key。
	adminOnly bool
}

func NewAPIKeyHandler(svc *apikey.Service, adminOnly bool) *APIKeyHandler {
	return &APIKeyHandler{svc: svc, adminOnly: adminOnly}
}

// apiKeyView 是列表与创建响应共用的元数据视图。
//
// 刻意不把 store.APIKey 直接序列化出去：明文从来没有第二个入口（§5.4 不变量），
// 掩码也只能由公开段拼出来，所以"能返回什么"这件事必须在出口处收口一次。
type apiKeyView struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	MaskedKey  string     `json:"masked_key"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  *time.Time `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CallCount  int64      `json:"call_count"`
	CreatedAt  time.Time  `json:"created_at"`
}

func newAPIKeyView(k store.APIKey) apiKeyView {
	return apiKeyView{
		ID:         k.ID,
		Name:       k.Name,
		MaskedKey:  apikey.Mask(k.Lookup),
		Scopes:     append([]string{}, k.Scopes...),
		ExpiresAt:  k.ExpiresAt,
		RevokedAt:  k.RevokedAt,
		LastUsedAt: k.LastUsedAt,
		CallCount:  k.CallCount,
		CreatedAt:  k.CreatedAt,
	}
}

// createdAPIKey 是创建响应：`{"key": "ox_sk_...", "api_key": {...}}`。
// 唯一一次带明文的返回——之后再也拿不到（G2）。
type createdAPIKey struct {
	Key    string     `json:"key"`
	APIKey apiKeyView `json:"api_key"`
}

// apiKeyReady 判断凭证功能是否启用。关掉时（apikey.enabled: false）路由仍然注册着，
// 每个请求回 503——对齐计费/充值的既有约定（§7.3）。
func apiKeyReady(c *gin.Context, svc *apikey.Service) bool {
	if svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "api keys are disabled"})
		return false
	}
	return true
}

// createAPIKeyRequest 是创建请求。scopes 与 preset 二选一或同时给：preset 由服务端
// 展开成显式列表，scopes 是前端按目录勾好的显式列表（§5.3 的展开规则）。
type createAPIKeyRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	Preset    string     `json:"preset"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// List 列出自己的 key。已撤销与已过期的也在列表里：用户要看的是"这把 key 还在不在"。
//
// 分页参数按仓库惯例（page 从 1 起，page_size 最大 100）；真的切片在内存里做——
// §3.2 的容量假设是单账号 ≤ 100 把，为它多一条分页查询不划算。
func (h *APIKeyHandler) List(c *gin.Context) {
	if !apiKeyReady(c, h.svc) {
		return
	}
	page, pageSize := parsePageParams(c)

	keys, err := h.svc.List(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		writeAPIKeyError(c, err)
		return
	}
	total := len(keys)
	offset := (page - 1) * pageSize
	data := make([]apiKeyView, 0, pageSize)
	for i := offset; i < total && len(data) < pageSize; i++ {
		data = append(data, newAPIKeyView(keys[i]))
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  data,
		"total": total,
		"page":  page,
	})
}

// Create 创建一把 key，返回明文一次 + 元数据。
func (h *APIKeyHandler) Create(c *gin.Context) {
	if !apiKeyReady(c, h.svc) {
		return
	}
	if h.adminOnly && !middleware.IsAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	var req createAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	scopes := append([]string(nil), req.Scopes...)
	if preset := strings.TrimSpace(req.Preset); preset != "" {
		expanded, ok := apikey.ExpandPreset(preset)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown preset: " + preset})
			return
		}
		scopes = append(scopes, expanded...)
	}

	plaintext, key, err := h.svc.Create(c.Request.Context(), middleware.UserID(c), req.Name, scopes, req.ExpiresAt)
	if err != nil {
		writeAPIKeyError(c, err)
		return
	}
	c.JSON(http.StatusCreated, createdAPIKey{Key: plaintext, APIKey: newAPIKeyView(*key)})
}

// Revoke 撤销一把 key（软删）。重复调用是幂等成功；别人的 key 一律 404。
func (h *APIKeyHandler) Revoke(c *gin.Context) {
	if !apiKeyReady(c, h.svc) {
		return
	}
	if err := h.svc.Revoke(c.Request.Context(), middleware.UserID(c), c.Param("id")); err != nil {
		writeAPIKeyError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Scopes 下发 scope 目录、分组与预设。
//
// 目录是服务端单一事实源：改一条 scope 的描述或分组不需要重新构建前端；预设也由
// 服务端展开（前端只回传勾选结果）。分组与预设都不参与授权判定，授权永远是精确匹配。
func (h *APIKeyHandler) Scopes(c *gin.Context) {
	if !apiKeyReady(c, h.svc) {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"scopes":     apikey.Catalog(),
		"groups":     apikey.Groups(),
		"presets":    apikey.Presets(),
		"can_create": !h.adminOnly || middleware.IsAdmin(c),
	})
}

// writeAPIKeyError 把错误映射成 §5.3 里写明的响应体。
//
// 400 的文案是给调用方（脚本作者）看的字面契约，所以不复用带 `apikey:` 包名前缀的
// error.Error()：把"改哪个参数"说清楚，比带上我们的包名有用。
func writeAPIKeyError(c *gin.Context, err error) {
	var unknown *apikey.UnknownScopeError
	switch {
	case errors.As(err, &unknown):
		body := gin.H{"error": "unknown scope"}
		if len(unknown.Unknown) > 0 {
			body["unknown"] = unknown.Unknown
		}
		if len(unknown.Deprecated) > 0 {
			body["deprecated"] = unknown.Deprecated
		}
		c.JSON(http.StatusBadRequest, body)
	case errors.Is(err, apikey.ErrNameRequired):
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
	case errors.Is(err, apikey.ErrNameTooLong):
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is too long"})
	case errors.Is(err, apikey.ErrNoScopes):
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one scope is required"})
	case errors.Is(err, apikey.ErrExpiresAtPast):
		c.JSON(http.StatusBadRequest, gin.H{"error": "expires_at must be in the future"})
	case errors.Is(err, apikey.ErrKeyLimitReached):
		// 账号的 key 数到顶了：先删掉不用的，重试不会变。
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many keys for this account"})
	case errors.Is(err, store.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
	default:
		logging.Errorf("apikey: request failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
