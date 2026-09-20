package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/store"
)

const (
	userIDKey = "userID"
	adminKey  = "isAdmin"
)

// APIKeyHeader 是 API Key 的专用请求头。同一个凭据也接受
// `Authorization: Bearer ox:sk:...`，与 JWT 靠 apikey.Prefix 区分。
const APIKeyHeader = "X-API-Key"

// APIKeyAuthenticator 把明文密钥换出记录（实现是 store.APIKeyStore）。
type APIKeyAuthenticator interface {
	Authenticate(plain string) (*store.APIKey, error)
}

// Auth 是 /api 下受保护路由的统一入口，一次做完认证与授权：
//
//   - 认证：`Authorization: Bearer <JWT>`（控制台会话），或 API Key
//     （X-API-Key 头，或 `Authorization: Bearer ox:sk:...`）。
//   - 授权：控制台会话拥有账号本身的全部权限；API Key 按 apiKeyRouteScopes
//     逐条放行，表外路由一律拒绝。
//
// 授权写在认证里而不是拆成一个中间件，是为了让“新加的路由忘了挂授权”不可能发生：
// 默认拒绝，只有表里列出的命名空间才对 API Key 开放。
func Auth(secret []byte, keys APIKeyAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		// X-API-Key 是密钥专用头：塞进来别的东西（比如 JWT）直接拒绝，
		// 免得两个头的作用域互相串门。
		if credential := strings.TrimSpace(c.GetHeader(APIKeyHeader)); credential != "" {
			if !apikey.LooksLike(credential) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
				return
			}
			authorizeAPIKey(c, keys, credential)
			return
		}

		raw := c.GetHeader("Authorization")
		if !strings.HasPrefix(raw, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		credential := strings.TrimSpace(strings.TrimPrefix(raw, "Bearer "))
		if apikey.LooksLike(credential) {
			authorizeAPIKey(c, keys, credential)
			return
		}
		authorizeSession(c, secret, credential)
	}
}

// authorizeSession 走 JWT 路径。控制台会话不受权限范围限制：它就是账号本人。
func authorizeSession(c *gin.Context, secret []byte, tokenStr string) {
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return secret, nil
	}, jwt.WithExpirationRequired(), jwt.WithTimeFunc(func() time.Time { return time.Now() }))
	if err != nil || !tok.Valid {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	sub, err := tok.Claims.GetSubject()
	if err != nil || sub == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
		return
	}
	c.Set(userIDKey, sub)
	if isAdmin, ok := tok.Claims.(jwt.MapClaims)["is_admin"].(bool); ok {
		c.Set(adminKey, isAdmin)
	}
	c.Next()
}

// authorizeAPIKey 走 API Key 路径：先认证，再按 apiKeyRouteScopes 校验权限范围。
// 认证通过后把密钥属主写进 userID，业务 handler 照旧用 middleware.UserID(c)
// 做属主隔离，不需要知道调用方是会话还是密钥。
func authorizeAPIKey(c *gin.Context, keys APIKeyAuthenticator, credential string) {
	if keys == nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "api key authentication is not configured"})
		return
	}
	key, err := keys.Authenticate(credential)
	if err != nil {
		// 不区分“格式对但查无此钥”，避免给探测者额外信息。
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
		return
	}

	required, ok := requiredScope(c.FullPath())
	if !ok {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": "api key cannot call this endpoint; use a console session instead",
		})
		return
	}
	if !apikey.Allows(apikey.FromStrings(key.Scopes), required, c.Request.Method) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": "api key is missing the required permission: " + string(required),
		})
		return
	}

	c.Set(userIDKey, key.OwnerID)
	c.Next()
}

// UserID extracts the authenticated userID set by the auth middleware.
func UserID(c *gin.Context) string {
	id, _ := c.Get(userIDKey)
	s, _ := id.(string)
	return s
}

func IsAdmin(c *gin.Context) bool {
	isAdmin, _ := c.Get(adminKey)
	v, _ := isAdmin.(bool)
	return v
}
