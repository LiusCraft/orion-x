package middleware

import (
	"strings"

	"github.com/liuscraft/orion-x/internal/apikey"
)

// apiKeyRouteScope 把一条 /api 路由前缀绑到它需要的权限范围。
type apiKeyRouteScope struct {
	prefix string
	scope  apikey.Scope
}

// apiKeyRouteScopes 是 API Key 能触达的 /api 路由，前缀匹配取最长命中。
//
// 表外路由（供应商密钥、模型与音色、语言字典、计费、密钥管理……）对 API Key 一律
// 拒绝：API Key 是程序化调用凭据，控制面能力只留给控制台会话。写成默认拒绝是为了
// 让新增路由不必记得加白名单——只有这张表能开口子。
var apiKeyRouteScopes = []apiKeyRouteScope{
	// MCP 绑定挂在 /api/voicebots/:id/mcps 下，算智能体的配置，归 agent 范围。
	{prefix: "/api/voicebots", scope: apikey.ScopeAgent},
	{prefix: "/api/agent-templates", scope: apikey.ScopeAgent},
	{prefix: "/api/mcp", scope: apikey.ScopeMCP},
	{prefix: "/api/data/memory", scope: apikey.ScopeData},
	{prefix: "/api/data/knowledge", scope: apikey.ScopeData},
	{prefix: "/api/assets", scope: apikey.ScopeData},
}

// requiredScope 返回该路由要求的权限范围。第二个返回值为 false 表示表里没有——
// 也就是 API Key 不该触达这个路由。
func requiredScope(fullPath string) (apikey.Scope, bool) {
	var (
		matched string
		scope   apikey.Scope
	)
	for _, route := range apiKeyRouteScopes {
		if !hasPathPrefix(fullPath, route.prefix) || len(route.prefix) <= len(matched) {
			continue
		}
		matched, scope = route.prefix, route.scope
	}
	if matched == "" {
		return "", false
	}
	return scope, true
}

// hasPathPrefix 要求前缀落在路径分段边界上：/api/mcp 命中 /api/mcp/servers，
// 但不该命中 /api/mcpfoo。
func hasPathPrefix(path, prefix string) bool {
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	return len(path) == len(prefix) || path[len(prefix)] == '/'
}
