package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

const (
	// verifyTokenTTL 是验证链接的有效期。
	verifyTokenTTL = 24 * time.Hour
	// ResendCooldown 是同一账号两次重发验证邮件之间的最小间隔。
	ResendCooldown = time.Minute
	// verifySendTimeout 是单封验证邮件的发送上限（mailer 自身另有 15s 连接上限）。
	verifySendTimeout = 20 * time.Second
)

type verifyEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

// POST /api/auth/verify-email
//
// 消费邮件链接里的令牌：成功后账号标记为已验证。无效/过期/已消费一律同一文案，
// 不暴露令牌处于哪种状态（防枚举）。
func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var req verifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "验证链接无效或已过期"})
		return
	}

	userID, err := h.verify.ConsumeToken(c.Request.Context(), req.Token)
	if err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "验证链接无效或已过期"})
			return
		}
		logging.Errorf("auth: consume verify token: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		return
	}
	if err := h.users.MarkEmailVerified(userID); err != nil {
		logging.Errorf("auth: mark user %s email verified: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "邮箱验证成功"})
}

type resendVerificationRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// POST /api/auth/resend-verification
//
// 恒返回同一文案：邮箱不存在、已验证、冷却中都一样（不泄露账号是否存在）。
func (h *AuthHandler) ResendVerification(c *gin.Context) {
	var req resendVerificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if h.cfg.EmailVerify && h.mail != nil {
		h.resendVerification(c.Request.Context(), strings.TrimSpace(strings.ToLower(req.Email)))
	}

	c.JSON(http.StatusOK, gin.H{"message": "如果该邮箱需要验证，验证邮件已发送"})
}

func (h *AuthHandler) resendVerification(ctx context.Context, email string) {
	u, err := h.users.GetByEmail(email)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			logging.Errorf("auth: resend verification lookup %s: %v", email, err)
		}
		return
	}
	if u.EmailVerifiedAt != nil {
		return
	}
	allowed, err := h.verify.AllowResend(ctx, u.ID, ResendCooldown)
	if err != nil {
		logging.Errorf("auth: resend cooldown for user %s: %v", u.ID, err)
		return
	}
	if !allowed {
		return
	}
	h.sendVerificationMail(ctx, u)
}

// sendVerificationMail 签发新令牌并发送验证邮件，返回是否送达。
// 失败只记日志：账号已经建好，用户可以走重发入口，不把 SMTP 故障变成注册失败。
func (h *AuthHandler) sendVerificationMail(ctx context.Context, u *store.User) bool {
	if h.mail == nil {
		return false
	}
	token, err := h.verify.IssueToken(ctx, u.ID, verifyTokenTTL)
	if err != nil {
		logging.Errorf("auth: issue verify token for user %s: %v", u.ID, err)
		return false
	}

	link := h.verifyLink(token)
	body := fmt.Sprintf("你好，\n\n请点击下面的链接完成邮箱验证（24 小时内有效）：\n\n%s\n\n如果这不是你本人的操作，请忽略此邮件。\n", link)
	sendCtx, cancel := context.WithTimeout(ctx, verifySendTimeout)
	defer cancel()
	if err := h.mail.Send(sendCtx, u.Email, "验证你的邮箱 · Orion-X", body); err != nil {
		logging.Errorf("auth: send verify email to %s: %v", u.Email, err)
		return false
	}
	return true
}

func (h *AuthHandler) verifyLink(token string) string {
	return strings.TrimRight(h.cfg.VerifyURLBase, "/") + "/login?verify_token=" + url.QueryEscape(token)
}
