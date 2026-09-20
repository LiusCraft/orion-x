package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// InternalAuth 校验服务间调用的 Bearer token（docs/billing-design.md §14.1）。
//
// token 没配时**拒绝**，不能放行：“忘了配”如果等于“不鉴权”，那迟早会发生在生产上。
// 配了这个中间件的接口是往账单上写字的（/internal/billing/*），放行的代价是别人
// 可以随便伪造用量。启动时 main 还会额外 Warnf 提醒一次。
//
// 请求签名不做：token 都泄露了，签名密钥也一起泄露，收益不大，调试成本倒是实打实的。
// 链路完整性交给部署（同机、内网、TLS）。
func InternalAuth(token string) gin.HandlerFunc {
	expected := []byte(token)
	configured := strings.TrimSpace(token) != ""

	return func(c *gin.Context) {
		if !configured {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "internal token is not configured; set internal.token (env INTERNAL_TOKEN)",
			})
			return
		}
		provided, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok || subtle.ConstantTimeCompare(provided, expected) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid internal token"})
			return
		}
		c.Next()
	}
}

// bearerToken 从 Authorization 头里取出 Bearer 凭据。
func bearerToken(raw string) ([]byte, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(raw, prefix) {
		return nil, false
	}
	token := strings.TrimSpace(strings.TrimPrefix(raw, prefix))
	if token == "" {
		return nil, false
	}
	return []byte(token), true
}
