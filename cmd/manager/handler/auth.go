package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/internal/store"
)

type AuthHandler struct {
	users     *store.UserStore
	signToken func(userID string, isAdmin bool) (string, error)
}

func NewAuthHandler(users *store.UserStore, signToken func(userID string, isAdmin bool) (string, error)) *AuthHandler {
	return &AuthHandler{users: users, signToken: signToken}
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
	u, err := h.users.Create(email, username, string(hash), "self")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "注册失败，请稍后重试"})
		return
	}

	token, err := h.signToken(u.ID, u.IsAdmin)
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

	token, err := h.signToken(u.ID, u.IsAdmin)
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

// POST /api/auth/unbind-github (JWT)
// 解绑 GitHub 前必须已有密码，否则账号将失去所有登录方式
func (h *AuthHandler) UnbindGithub(c *gin.Context) {
	userID := c.GetString("userID")
	u, err := h.users.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if u.GithubID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未绑定 GitHub 账号"})
		return
	}
	if u.PasswordHash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先设置密码，再解绑 GitHub"})
		return
	}
	if err := h.users.UpdateGithubID(u.ID, ""); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解绑失败，请稍后重试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "GitHub 解绑成功"})
}

// GET /api/auth/profile (JWT)
func (h *AuthHandler) Profile(c *gin.Context) {
	userID := c.GetString("userID")
	u, err := h.users.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user_id":      u.ID,
		"email":        u.Email,
		"username":     u.Username,
		"is_admin":     u.IsAdmin,
		"github_id":    u.GithubID,
		"has_password": u.PasswordHash != "",
	})
}
