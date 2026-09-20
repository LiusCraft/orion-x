package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/store"
)

var testSecret = []byte("test-secret")

// fakeKeyStore 是 middleware.APIKeyAuthenticator 的内联 mock（仓库约定：
// 手写 mock struct，不引 mock 生成器）。它只认 registered 里的明文。
type fakeKeyStore struct {
	registered map[string]*store.APIKey
	calls      int
}

func (f *fakeKeyStore) Authenticate(plain string) (*store.APIKey, error) {
	f.calls++
	if key, ok := f.registered[plain]; ok {
		return key, nil
	}
	return nil, errors.New("not found")
}

func testJWT(t *testing.T, userID string, isAdmin bool, exp time.Time) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":      userID,
		"is_admin": isAdmin,
		"exp":      jwt.NewNumericDate(exp),
	}).SignedString(testSecret)
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}
	return tok
}

// newAuthTestRouter 注册一批真实存在的路由形态，覆盖“表内命名空间 / 表外控制面 /
// 安全方法与写方法”三种情况。handler 把认证结果回写出来，便于断言属主传递。
func newAuthTestRouter(keys APIKeyAuthenticator) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mw := Auth(testSecret, keys)

	r.GET("/api/voicebots", mw, echoIdentity)
	r.POST("/api/voicebots", mw, echoIdentity)
	r.GET("/api/mcp/servers", mw, echoIdentity)
	r.POST("/api/mcp/call-tool", mw, echoIdentity)
	r.GET("/api/data/memory/agents", mw, echoIdentity)
	r.DELETE("/api/data/knowledge/documents/:doc_id", mw, echoIdentity)
	r.GET("/api/providers", mw, echoIdentity)
	r.GET("/api/apikeys", mw, echoIdentity)
	r.POST("/api/billing/accounts/:id/adjust", mw, echoIdentity)
	return r
}

func echoIdentity(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"user_id": UserID(c), "is_admin": IsAdmin(c)})
}

func TestAuthWithSession(t *testing.T) {
	r := newAuthTestRouter(&fakeKeyStore{})

	cases := []struct {
		name       string
		header     string
		wantStatus int
	}{
		{name: "valid jwt", header: "Bearer " + testJWT(t, "user-1", true, time.Now().Add(time.Hour)), wantStatus: http.StatusOK},
		{name: "expired jwt", header: "Bearer " + testJWT(t, "user-1", false, time.Now().Add(-time.Hour)), wantStatus: http.StatusUnauthorized},
		{name: "malformed jwt", header: "Bearer not-a-jwt", wantStatus: http.StatusUnauthorized},
		{name: "empty bearer", header: "Bearer   ", wantStatus: http.StatusUnauthorized},
		{name: "missing header", header: "", wantStatus: http.StatusUnauthorized},
		{name: "non bearer scheme", header: "Basic dXNlcjpwYXNz", wantStatus: http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
			if tc.header != "" {
				request.Header.Set("Authorization", tc.header)
			}
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			if response.Code != tc.wantStatus {
				t.Fatalf("GET /api/providers = %d, want %d (body %s)", response.Code, tc.wantStatus, response.Body.String())
			}
		})
	}

	// 会话不受权限范围约束：控制面路由照常可达。
	request := httptest.NewRequest(http.MethodPost, "/api/billing/accounts/acct-1/adjust", nil)
	request.Header.Set("Authorization", "Bearer "+testJWT(t, "admin-1", true, time.Now().Add(time.Hour)))
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("session on a console-only route = %d, want 200 (body %s)", response.Code, response.Body.String())
	}
}

func TestAuthWithAPIKey(t *testing.T) {
	plain, err := apikey.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	keys := &fakeKeyStore{registered: map[string]*store.APIKey{
		plain: {ID: "key-1", OwnerID: "user-7", Scopes: apikey.Strings([]apikey.Scope{apikey.ScopeAgent})},
	}}

	cases := []struct {
		name       string
		method     string
		path       string
		key        string
		wantStatus int
	}{
		{name: "x-api-key header", method: http.MethodGet, path: "/api/voicebots", key: plain, wantStatus: http.StatusOK},
		{name: "bearer api key", method: http.MethodGet, path: "/api/voicebots", key: plain, wantStatus: http.StatusOK},
		{name: "unknown key", method: http.MethodGet, path: "/api/voicebots", key: apikey.Prefix + "deadbeef", wantStatus: http.StatusUnauthorized},
		{name: "x-api-key must not carry a jwt", method: http.MethodGet, path: "/api/voicebots", key: testJWT(t, "user-1", true, time.Now().Add(time.Hour)), wantStatus: http.StatusUnauthorized},
		{name: "write inside its namespace", method: http.MethodPost, path: "/api/voicebots", key: plain, wantStatus: http.StatusOK},
		{name: "other namespace", method: http.MethodGet, path: "/api/mcp/servers", key: plain, wantStatus: http.StatusForbidden},
		{name: "console-only route", method: http.MethodGet, path: "/api/providers", key: plain, wantStatus: http.StatusForbidden},
		{name: "key management is session-only", method: http.MethodGet, path: "/api/apikeys", key: plain, wantStatus: http.StatusForbidden},
		{name: "billing is session-only", method: http.MethodPost, path: "/api/billing/accounts/acct-1/adjust", key: plain, wantStatus: http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, nil)
			request.Header.Set(APIKeyHeader, tc.key)
			response := httptest.NewRecorder()
			r := newAuthTestRouter(keys)
			r.ServeHTTP(response, request)
			if response.Code != tc.wantStatus {
				t.Fatalf("%s %s = %d, want %d (body %s)", tc.method, tc.path, response.Code, tc.wantStatus, response.Body.String())
			}
			if tc.wantStatus == http.StatusOK && !strings.Contains(response.Body.String(), "user-7") {
				t.Errorf("api key request lost its owner: %s", response.Body.String())
			}
		})
	}

	// Authorization 里的 API Key 与 X-API-Key 等价。
	request := httptest.NewRequest(http.MethodGet, "/api/voicebots", nil)
	request.Header.Set("Authorization", "Bearer "+plain)
	response := httptest.NewRecorder()
	newAuthTestRouter(keys).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("Bearer api key = %d, want 200 (body %s)", response.Code, response.Body.String())
	}
}

func TestAuthWithAPIKeyScopes(t *testing.T) {
	plain, err := apikey.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	cases := []struct {
		name       string
		scopes     []apikey.Scope
		method     string
		path       string
		wantStatus int
	}{
		{name: "read covers a safe method", scopes: []apikey.Scope{apikey.ScopeRead}, method: http.MethodGet, path: "/api/mcp/servers", wantStatus: http.StatusOK},
		{name: "read does not cover deletes", scopes: []apikey.Scope{apikey.ScopeRead}, method: http.MethodDelete, path: "/api/data/knowledge/documents/doc-1", wantStatus: http.StatusForbidden},
		{name: "all covers writes", scopes: []apikey.Scope{apikey.ScopeAll}, method: http.MethodPost, path: "/api/mcp/call-tool", wantStatus: http.StatusOK},
		{name: "mcp covers mcp only", scopes: []apikey.Scope{apikey.ScopeMCP}, method: http.MethodGet, path: "/api/data/memory/agents", wantStatus: http.StatusForbidden},
		{name: "data covers knowledge", scopes: []apikey.Scope{apikey.ScopeData}, method: http.MethodDelete, path: "/api/data/knowledge/documents/doc-1", wantStatus: http.StatusOK},
		{name: "several scopes any match", scopes: []apikey.Scope{apikey.ScopeMCP, apikey.ScopeData}, method: http.MethodGet, path: "/api/data/memory/agents", wantStatus: http.StatusOK},
		{name: "no scopes denies even reads", scopes: nil, method: http.MethodGet, path: "/api/voicebots", wantStatus: http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keys := &fakeKeyStore{registered: map[string]*store.APIKey{
				plain: {ID: "key-1", OwnerID: "user-7", Scopes: apikey.Strings(tc.scopes)},
			}}
			request := httptest.NewRequest(tc.method, tc.path, nil)
			request.Header.Set(APIKeyHeader, plain)
			response := httptest.NewRecorder()
			newAuthTestRouter(keys).ServeHTTP(response, request)
			if response.Code != tc.wantStatus {
				t.Fatalf("%s %s = %d, want %d (body %s)", tc.method, tc.path, response.Code, tc.wantStatus, response.Body.String())
			}
		})
	}
}

func TestAuthWithoutAPIKeyStore(t *testing.T) {
	plain := apikey.Prefix + "0123456789abcdef"
	request := httptest.NewRequest(http.MethodGet, "/api/voicebots", nil)
	request.Header.Set(APIKeyHeader, plain)
	response := httptest.NewRecorder()
	newAuthTestRouter(nil).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("api key with no authenticator configured = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	// 没有配置 API Key 认证时，JWT 这条路径不受影响。
	request = httptest.NewRequest(http.MethodGet, "/api/voicebots", nil)
	request.Header.Set("Authorization", "Bearer "+testJWT(t, "user-1", false, time.Now().Add(time.Hour)))
	response = httptest.NewRecorder()
	newAuthTestRouter(nil).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("session with no authenticator configured = %d, want 200", response.Code)
	}
}

func TestRequiredScope(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		want   apikey.Scope
		wantOK bool
	}{
		{name: "voicebots collection", path: "/api/voicebots", want: apikey.ScopeAgent, wantOK: true},
		{name: "nested voicebot route", path: "/api/voicebots/:id/devices/:did/channels/telegram", want: apikey.ScopeAgent, wantOK: true},
		{name: "longest prefix wins", path: "/api/data/knowledge/knowledge_bases/:kb_id", want: apikey.ScopeData, wantOK: true},
		{name: "mcp tools", path: "/api/mcp/call-tool", want: apikey.ScopeMCP, wantOK: true},
		{name: "providers are console-only", path: "/api/providers", wantOK: false},
		{name: "key management is console-only", path: "/api/apikeys", wantOK: false},
		{name: "billing is console-only", path: "/api/billing/summary", wantOK: false},
		{name: "partial segment must not match", path: "/api/mcpfoo", wantOK: false},
		{name: "prefix inside a longer segment must not match", path: "/api/voicebotsx", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := requiredScope(tc.path)
			if ok != tc.wantOK {
				t.Fatalf("requiredScope(%q) ok = %v, want %v", tc.path, ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("requiredScope(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}
