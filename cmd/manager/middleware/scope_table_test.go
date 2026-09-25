package middleware

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/apikey"
)

// 覆盖性校验本身也要被测：一个永远返回"没问题"的校验函数比没有校验更危险。
func TestValidateCoverageDetectsUndeclaredRoute(t *testing.T) {
	problems := ValidateCoverage([]gin.RouteInfo{{Method: "GET", Path: "/api/brand-new-feature"}})
	if len(problems) == 0 {
		t.Fatal("ValidateCoverage accepted a route that is neither scoped nor whitelisted")
	}
	if !strings.Contains(strings.Join(problems, "\n"), "/api/brand-new-feature") {
		t.Fatalf("problems = %v, want them to name the route", problems)
	}
}

func TestValidateCoverageAcceptsDeclaredAndWhitelisted(t *testing.T) {
	problems := ValidateCoverage([]gin.RouteInfo{
		{Method: "GET", Path: "/api/voicebots"},
		{Method: "GET", Path: "/api/api-keys"},
		{Method: "GET", Path: "/api/auth/profile"},
		{Method: "POST", Path: "/api/billing/recharge/abc123/refund"},
		{Method: "GET", Path: "/healthz"}, // 不在 /api 下，不参与校验
	})
	if len(problems) > 0 {
		t.Fatalf("ValidateCoverage reported problems for known routes: %v", problems)
	}
}

func TestKeyUnreachableRules(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   bool
	}{
		// 账号与凭证安全。
		{method: "POST", path: "/api/auth/change-password", want: true},
		{method: "GET", path: "/api/api-keys", want: true},
		{method: "DELETE", path: "/api/api-keys/abc", want: true},
		{method: "GET", path: "/api/api-keys/scopes", want: true},
		// 动钱的动作。
		{method: "POST", path: "/api/billing/recharge", want: true},
		{method: "GET", path: "/api/billing/recharge/config", want: true},
		{method: "POST", path: "/api/billing/recharge/abc/refund", want: true},
		{method: "GET", path: "/api/billing/items", want: true},
		{method: "PUT", path: "/api/billing/items/llm:tokens:input", want: true},
		{method: "POST", path: "/api/billing/prices", want: true},
		// 只读的计费面是 key 可达的。
		{method: "GET", path: "/api/billing/prices", want: false},
		{method: "GET", path: "/api/billing/summary", want: false},
		// 前缀匹配要卡在分隔符上：/api/authz 不是 /api/auth 的子路径。
		{method: "GET", path: "/api/authz", want: false},
		{method: "GET", path: "/api/voicebots", want: false},
	}
	for _, tc := range cases {
		if got := isKeyUnreachable(tc.method, tc.path); got != tc.want {
			t.Errorf("isKeyUnreachable(%s %s) = %v, want %v", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestRouteScopesCoverTheAnchorRoutes(t *testing.T) {
	table := RouteScopes()
	// 抽查几条最容易搞错方向的：读接口给读 scope，写接口给写 scope，
	// 会扣费的克隆要写 scope，会执行外部工具的 MCP 调用单独一个 scope。
	cases := []struct {
		spec string
		want string
	}{
		{"GET /api/voicebots", apikey.ScopeAgentRead},
		{"POST /api/voicebots", apikey.ScopeAgentWrite},
		{"GET /api/voicebots/:id/devices", apikey.ScopeDeviceRead},
		{"DELETE /api/voicebots/:id/devices/:did", apikey.ScopeDeviceWrite},
		{"POST /api/models/:id/voices/clone", apikey.ScopeModelWrite},
		{"GET /api/models/:id/voices", apikey.ScopeModelRead},
		{"POST /api/mcp/call-tool", apikey.ScopeMCPCall},
		{"POST /api/mcp/servers", apikey.ScopeMCPWrite},
		{"GET /api/mcp/servers", apikey.ScopeMCPRead},
		{"DELETE /api/data/memory/:id", apikey.ScopeDataWrite},
		{"GET /api/sessions", apikey.ScopeDataRead},
		{"GET /api/billing/usage", apikey.ScopeBillingRead},
	}
	for _, tc := range cases {
		scopes, ok := table.Required(tc.spec[:strings.Index(tc.spec, " ")], tc.spec[strings.Index(tc.spec, " ")+1:])
		if !ok {
			t.Errorf("%s is not declared in the scope table", tc.spec)
			continue
		}
		if len(scopes) != 1 || scopes[0] != tc.want {
			t.Errorf("%s requires %v, want [%s]", tc.spec, scopes, tc.want)
		}
	}
}
