package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireAdmin 只放行 is_admin 的 JWT（docs/billing-design.md §8：计费的定价、账户
// 与人工调整都在管理端）。它必须挂在 middleware.Auth 之后，自己不看 token；
// API Key 走的路径不会写入 is_admin，所以这里天然拦掉密钥调用。
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsAdmin(c) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin only"})
			return
		}
		c.Next()
	}
}
