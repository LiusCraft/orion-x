package handler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/oauth"
	"github.com/liuscraft/orion-x/internal/store"
)

// OAuthHandler 通用第三方 OAuth 登录/绑定处理，按 :provider 路由分发到
// internal/oauth 注册表。平台行为（授权 URL、token 换取、用户信息）由
// Provider 实现，账号匹配/绑定逻辑在此统一处理。
type OAuthHandler struct {
	users     *store.UserStore
	bindings  *store.OAuthBindingStore
	signToken func(userID string, isAdmin bool) (string, error)
	state     *oauth.StateStore
}

func NewOAuthHandler(users *store.UserStore, bindings *store.OAuthBindingStore, signToken func(userID string, isAdmin bool) (string, error)) *OAuthHandler {
	return &OAuthHandler{
		users:     users,
		bindings:  bindings,
		signToken: signToken,
		state:     oauth.NewStateStore(),
	}
}

// ---------------------------------------------------------------------------
// Providers — 可用平台列表（无需认证，登录页与账号页共用）
// GET /api/auth/oauth/providers
// ---------------------------------------------------------------------------

func (h *OAuthHandler) Providers(c *gin.Context) {
	names := oauth.Names()
	list := make([]gin.H, 0, len(names))
	for _, name := range names {
		p, _ := oauth.Get(name)
		list = append(list, gin.H{"provider": p.Name(), "name": p.DisplayName()})
	}
	c.JSON(http.StatusOK, gin.H{"providers": list})
}

// ---------------------------------------------------------------------------
// Login — redirect to platform authorize page
// GET /api/auth/oauth/:provider/login?from=http://localhost:5173
// ---------------------------------------------------------------------------

func (h *OAuthHandler) Login(c *gin.Context) {
	provider, ok := oauth.Get(c.Param("provider"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "未知的 OAuth 平台"})
		return
	}

	frontend := c.Query("from")
	if frontend == "" {
		frontend = "/"
	}

	stateKey := newCSRFToken()
	h.state.Put(stateKey, frontend)

	c.Redirect(http.StatusFound, provider.AuthURL(stateKey))
}

// ---------------------------------------------------------------------------
// Callback — handle platform OAuth callback
// GET /api/auth/oauth/:provider/callback?code=xxx&state=yyy
// ---------------------------------------------------------------------------

func (h *OAuthHandler) Callback(c *gin.Context) {
	provider, ok := oauth.Get(c.Param("provider"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "未知的 OAuth 平台"})
		return
	}

	stateKey := c.Query("state")
	if stateKey == "" {
		h.redirectError("/", c, "缺少 state 参数")
		return
	}

	redirectTo, ok := h.state.Take(stateKey)
	if !ok {
		h.redirectError("/", c, "安全验证失败，请重新授权")
		return
	}
	if redirectTo == "" {
		redirectTo = "/"
	}

	code := c.Query("code")
	if code == "" {
		h.redirectError(redirectTo, c, "缺少 code 参数")
		return
	}

	// Exchange code for access token
	ctx := c.Request.Context()
	oauthToken, err := provider.Exchange(ctx, code)
	if err != nil {
		logging.Errorf("oauth %s: exchange code: %v", provider.Name(), err)
		h.redirectError(redirectTo, c, "授权失败")
		return
	}

	// Fetch user info from platform
	info, err := provider.FetchUserInfo(ctx, oauthToken)
	if err != nil {
		logging.Errorf("oauth %s: fetch user info: %v", provider.Name(), err)
		h.redirectError(redirectTo, c, "获取平台用户信息失败")
		return
	}
	info.Email = strings.TrimSpace(strings.ToLower(info.Email))
	if info.Email == "" {
		h.redirectError(redirectTo, c, "无法获取平台邮箱")
		return
	}

	// 1) 该平台账号已有绑定 → 直接登录绑定用户
	binding, err := h.bindings.GetByProviderAndUID(provider.Name(), info.ProviderUID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		logging.Errorf("oauth %s: lookup binding: %v", provider.Name(), err)
		h.redirectError(redirectTo, c, "内部错误")
		return
	}
	if binding != nil {
		u, err := h.users.GetByID(binding.UserID)
		if err != nil {
			logging.Errorf("oauth %s: binding user %s: %v", provider.Name(), binding.UserID, err)
			h.redirectError(redirectTo, c, "内部错误")
			return
		}
		h.redirectWithToken(redirectTo, c, u)
		return
	}

	// 2) 邮箱匹配已有账号 → 绑定到该账号
	u, err := h.users.GetByEmail(info.Email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		logging.Errorf("oauth %s: lookup by email: %v", provider.Name(), err)
		h.redirectError(redirectTo, c, "内部错误")
		return
	}
	if u != nil {
		if err := h.bindings.Bind(u.ID, provider.Name(), info.ProviderUID, provider.Name()); err != nil {
			logging.Errorf("oauth %s: bind user %s: %v", provider.Name(), u.ID, err)
			h.redirectError(redirectTo, c, "绑定失败")
			return
		}
		logging.Infof("oauth %s: bound %s (%s) to user %q", provider.Name(), info.ProviderUID, info.Email, u.Username)
		h.redirectWithToken(redirectTo, c, u)
		return
	}

	// 3) 新用户 → 创建账号并绑定
	u, err = h.users.CreateWithOAuth(info.Email, info.ProviderUID, provider.Name())
	if err != nil {
		logging.Errorf("oauth %s: create user: %v", provider.Name(), err)
		h.redirectError(redirectTo, c, "创建用户失败")
		return
	}
	if err := h.bindings.Bind(u.ID, provider.Name(), info.ProviderUID, provider.Name()); err != nil {
		logging.Errorf("oauth %s: bind new user %s: %v", provider.Name(), u.ID, err)
		h.redirectError(redirectTo, c, "绑定失败")
		return
	}
	logging.Infof("oauth %s: created user %q (email=%s, uid=%s)", provider.Name(), u.Username, info.Email, info.ProviderUID)

	h.redirectWithToken(redirectTo, c, u)
}

// ---------------------------------------------------------------------------
// Unbind — 解除平台绑定（JWT）
// POST /api/auth/oauth/:provider/unbind
// ---------------------------------------------------------------------------

func (h *OAuthHandler) Unbind(c *gin.Context) {
	providerName := c.Param("provider")
	if _, ok := oauth.Get(providerName); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "未知的 OAuth 平台"})
		return
	}

	userID := c.GetString("userID")
	if _, err := h.bindings.GetByUserAndProvider(userID, providerName); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "未绑定该平台账号"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		return
	}

	u, err := h.users.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// 安全约束：无密码且仅剩这一个绑定 → 解绑后没有任何登录方式
	count, err := h.bindings.CountByUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		return
	}
	if u.PasswordHash == "" && count <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先设置密码，再解绑"})
		return
	}

	if err := h.bindings.Unbind(userID, providerName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解绑失败，请稍后重试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "解绑成功"})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (h *OAuthHandler) redirectWithToken(redirectTo string, c *gin.Context, u *store.User) {
	token, err := h.signToken(u.ID, u.IsAdmin)
	if err != nil {
		h.redirectError(redirectTo, c, "token 生成失败")
		return
	}
	sep := "?"
	if strings.Contains(redirectTo, "?") {
		sep = "&"
	}
	c.Redirect(http.StatusFound, fmt.Sprintf("%s%stoken=%s&user_id=%s&username=%s",
		redirectTo, sep,
		url.QueryEscape(token),
		url.QueryEscape(u.ID),
		url.QueryEscape(u.Username),
	))
}

func (h *OAuthHandler) redirectError(redirectTo string, c *gin.Context, msg string) {
	sep := "?"
	if strings.Contains(redirectTo, "?") {
		sep = "&"
	}
	c.Redirect(http.StatusFound, fmt.Sprintf("%s%serror=%s", redirectTo, sep, url.QueryEscape(msg)))
}

func newCSRFToken() string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(buf)
}
