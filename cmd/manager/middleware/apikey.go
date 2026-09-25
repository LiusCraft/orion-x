package middleware

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/logging"
)

// principal 是"这次请求的身份来源"，与 userID 并列放在上下文里。
const (
	principalKey = "principal"
	apiKeyIDKey  = "apiKeyID"
	scopesKey    = "scopes"
)

const (
	// PrincipalJWT 是人（控制台、浏览器）。
	PrincipalJWT = "jwt"
	// PrincipalAPIKey 是机器（脚本、CI、第三方集成）。
	PrincipalAPIKey = "api_key"
)

// Auth 取代资源路由上原来的 JWT 中间件：按前缀在 JWT 与 API key 之间分流，
// 两条路都只做一件事——把 userID（key 另加 scopes / keyID）放进上下文。
//
// 关键约束：API key 这条路**永远不设置 isAdmin**，所以 RequireAdmin 天然挡住它；
// 它产出的 userID 与 JWT 语义完全相同，既有 handler 一行都不用改（§5.2）。
//
// keys 为 nil 表示 apikey.enabled: false——此时 key 这条路只回 401，
// 与"还没实现这个功能"时的行为一致（§7.3）。
func Auth(jwtSecret []byte, keys *apikey.Service) gin.HandlerFunc {
	jwtMw := JWT(jwtSecret)
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if !strings.HasPrefix(raw, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		token := strings.TrimPrefix(raw, "Bearer ")

		// 老客户端（控制台 old build、既有脚本）的 Bearer 不带 ox_sk_ 前缀，
		// 一律走原来的 JWT 分支，行为逐字节不变（§7.2）。
		if !strings.HasPrefix(token, apikey.Prefix) {
			jwtMw(c)
			return
		}
		authenticateAPIKey(c, keys, token)
	}
}

func authenticateAPIKey(c *gin.Context, keys *apikey.Service, token string) {
	c.Set(principalKey, PrincipalAPIKey)

	if keys == nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
		return
	}

	identity, err := keys.Authenticate(c.Request.Context(), token)
	if err != nil {
		// R3 的红线：日志里只能出现公开段，绝不出现完整串。lookup 是排障时
		// 回答"是哪把 key 在报错"的最小信息（§7.4 的观测项就靠它）。
		logging.Warnf("apikey: authenticate failed: %v (lookup=%q)", err, apikey.LookupOf(token))
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": apiKeyErrorMessage(err)})
		return
	}

	decision := keys.Allow(identity.KeyID)
	setRateLimitHeaders(c, decision)
	if !decision.Allowed {
		// 超限的请求不打库、不计入 last_used/call_count：它没走到业务层（§5.2 路径 A）。
		c.Header("Retry-After", strconv.Itoa(retryAfterSeconds(decision)))
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
		return
	}

	keys.RecordUsage(identity.KeyID)
	c.Set(userIDKey, identity.UserID)
	c.Set(apiKeyIDKey, identity.KeyID)
	c.Set(scopesKey, identity.Scopes)
	c.Next()
}

// RequireScopes 按 deny-by-default 判定本条路由要求的 scope（AND 关系）。
//
// 它只对 API key 生效：JWT 是人，权限由角色与资源归属决定，不参与 scope 判定。
// 表里没有声明的路由对 key 一律不放行——deny-by-default 在路由级同样成立，
// 忘了加声明是"拒绝"而不是"放行"。
func RequireScopes(table ScopeTable) gin.HandlerFunc {
	return func(c *gin.Context) {
		if Principal(c) != PrincipalAPIKey {
			c.Next()
			return
		}
		granted := Scopes(c)
		required, declared := table.Required(c.Request.Method, c.FullPath())
		if !declared || len(apikey.Missing(granted, required)) > 0 {
			abortInsufficientScope(c, required, granted)
			return
		}
		c.Next()
	}
}

// Principal 返回本次请求的身份来源：PrincipalJWT / PrincipalAPIKey。
func Principal(c *gin.Context) string {
	v, _ := c.Get(principalKey)
	s, _ := v.(string)
	return s
}

// KeyID 返回本次请求用的 API key 的 ID（JWT 请求为空）。
func KeyID(c *gin.Context) string {
	v, _ := c.Get(apiKeyIDKey)
	s, _ := v.(string)
	return s
}

// Scopes 返回这把 key 的 scope 集合（JWT 请求为空）。
func Scopes(c *gin.Context) []string {
	v, _ := c.Get(scopesKey)
	s, _ := v.([]string)
	return s
}

func abortInsufficientScope(c *gin.Context, required, granted []string) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"error":    "insufficient scope",
		"required": emptyIfNil(required),
		"granted":  emptyIfNil(granted),
	})
}

func emptyIfNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// apiKeyErrorMessage 把领域错误变成调用方看得懂的一句话：区分"撤销/过期"与"查不到"
// 是有意的（§6 N7）——key 的主人就是我们的用户，排障信息比"不泄露 key 存在性"值钱。
func apiKeyErrorMessage(err error) string {
	switch {
	case errors.Is(err, apikey.ErrRevoked):
		return "api key revoked"
	case errors.Is(err, apikey.ErrExpired):
		return "api key expired"
	default:
		return "invalid api key"
	}
}

func setRateLimitHeaders(c *gin.Context, d apikey.LimitDecision) {
	if d.Limit <= 0 {
		// 不限流时不发这组头：报一个 0 会让客户端以为配额是 0。
		return
	}
	c.Header("X-RateLimit-Limit", strconv.Itoa(d.Limit))
	c.Header("X-RateLimit-Remaining", strconv.Itoa(d.Remaining))
	if !d.Allowed {
		c.Header("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(d.RetryAfter).Unix(), 10))
	}
}

// retryAfterSeconds 是 Retry-After 的取值：攒出下一个令牌的整秒数（向上取整），
// 最小 1 秒——0 会让客户端立刻重试，把限流变成一轮打满。
func retryAfterSeconds(d apikey.LimitDecision) int {
	secs := int(math.Ceil(d.RetryAfter.Seconds()))
	if secs < 1 {
		secs = 1
	}
	return secs
}
