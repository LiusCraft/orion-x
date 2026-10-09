package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/internal/mailer"
	"github.com/liuscraft/orion-x/internal/store"
)

// AuthConfig 是 auth 模块的配置，同时用于 manager.yaml 的 auth 段
// （docs/auth-email-verification-design.md）；options 的组织与校验都留在模块内部。
type AuthConfig struct {
	// EmailVerify 开启后，邮箱密码注册的账号必须先完成邮箱验证才能登录；
	// GitHub 创建/匹配的账号免验证。默认 false = 与开启前一致。
	EmailVerify bool `yaml:"email_verify"`
	// VerifyURLBase 是控制台地址，用于拼接邮件里的验证链接
	// （<base>/login?verify_token=…）。开启验证时必填，不信任请求 Host/Origin。
	VerifyURLBase string `yaml:"verify_url_base"`
}

type AuthHandler struct {
	users    *store.UserStore
	bindings *store.OAuthBindingStore
	secret   []byte
	cfg      AuthConfig
	mail     *mailer.Mailer
	verify   *VerifyStore
}

// NewAuthHandler 构造 auth handler，并对 AuthConfig 做配置判定：开启邮箱验证时
// 缺 SMTP、Redis 或控制台地址直接返回错误，由启动装配 fail closed，而不是等第一
// 封注册邮件才发现发不出去。
func NewAuthHandler(users *store.UserStore, bindings *store.OAuthBindingStore, secret []byte, cfg AuthConfig, mail *mailer.Mailer, verify *VerifyStore) (*AuthHandler, error) {
	if cfg.EmailVerify {
		if mail == nil {
			return nil, errors.New("smtp.host is required when auth.email_verify is enabled")
		}
		if verify == nil {
			return nil, errors.New("redis.addr is required when auth.email_verify is enabled")
		}
		u, err := url.Parse(cfg.VerifyURLBase)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, fmt.Errorf("auth.verify_url_base %q must be an absolute http(s) URL", cfg.VerifyURLBase)
		}
	}
	return &AuthHandler{
		users:    users,
		bindings: bindings,
		secret:   secret,
		cfg:      cfg,
		mail:     mail,
		verify:   verify,
	}, nil
}

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6,max=128"`
	Username string `json:"username,omitempty" binding:"omitempty,min=1,max=32"`
}

// POST /api/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供有效的邮箱和密码（6-128字符）"})
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))

	// 检查邮箱是否已存在
	existing, err := h.users.GetByEmail(email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "该邮箱已被注册"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		return
	}

	username := strings.TrimSpace(req.Username)

	// 开启邮箱验证时：账号以未验证状态创建，不返回 JWT，改为发送验证邮件。
	if h.cfg.EmailVerify {
		u, err := h.users.CreateUnverified(email, username, string(hash), "self")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "注册失败，请稍后重试"})
			return
		}
		sent := h.sendVerificationMail(c.Request.Context(), u)
		message := "验证邮件已发送，请查收"
		if !sent {
			message = "账号已创建，但验证邮件发送失败，请稍后重试重发"
		}
		c.JSON(http.StatusCreated, gin.H{
			"email":             u.Email,
			"message":           message,
			"verification_sent": sent,
		})
		return
	}

	u, err := h.users.Create(email, username, string(hash), "self")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "注册失败，请稍后重试"})
		return
	}

	token, err := signToken(h.secret, u.ID, u.IsAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token 生成失败"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"token":    token,
		"user_id":  u.ID,
		"email":    u.Email,
		"username": u.Username,
		"is_admin": u.IsAdmin,
	})
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// POST /api/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	u, err := h.users.GetByEmail(email)
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "邮箱或密码不正确"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "邮箱或密码不正确"})
		return
	}

	if h.cfg.EmailVerify && u.EmailVerifiedAt == nil {
		c.JSON(http.StatusForbidden, gin.H{"code": "email_unverified", "error": "邮箱未验证，请先完成邮箱验证"})
		return
	}

	token, err := signToken(h.secret, u.ID, u.IsAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":    token,
		"user_id":  u.ID,
		"email":    u.Email,
		"username": u.Username,
		"is_admin": u.IsAdmin,
	})
}

type changePasswordRequest struct {
	// OldPassword 可为空——GitHub OAuth 创建的账号无密码，首次设置时留空
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

// POST /api/auth/change-password (JWT)
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID := c.GetString("userID")

	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	u, err := h.users.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// GitHub OAuth 创建的账号没有密码，跳过旧密码校验，允许首次设置密码
	if u.PasswordHash != "" {
		if req.OldPassword == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "旧密码不能为空"})
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.OldPassword)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "旧密码不正确"})
			return
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if err := h.users.UpdatePassword(userID, string(hash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "密码修改成功"})
}

type bindEmailRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// POST /api/auth/bind-email (JWT)
func (h *AuthHandler) BindEmail(c *gin.Context) {
	userID := c.GetString("userID")

	var req bindEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.users.UpdateEmail(userID, req.Email); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "邮箱绑定成功", "email": req.Email})
}

// GET /api/auth/profile (JWT)
func (h *AuthHandler) Profile(c *gin.Context) {
	userID := c.GetString("userID")
	u, err := h.users.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	binds, err := h.bindings.ListByUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		return
	}
	bindings := make([]gin.H, 0, len(binds))
	for _, b := range binds {
		bindings = append(bindings, gin.H{"provider": b.Provider, "provider_uid": b.ProviderUID})
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":      u.ID,
		"email":        u.Email,
		"username":     u.Username,
		"is_admin":     u.IsAdmin,
		"has_password": u.PasswordHash != "",
		"bindings":     bindings,
	})
}

// signToken 用 JWT secret 为用户签发 24 小时有效的 HS256 token，auth 与 oauth
// 两条登录路径共用。
func signToken(secret []byte, userID string, isAdmin bool) (string, error) {
	claims := jwt.MapClaims{
		"sub":      userID,
		"is_admin": isAdmin,
		"exp":      jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return tok, nil
}
