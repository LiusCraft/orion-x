package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/billing/service"
	"github.com/liuscraft/orion-x/internal/store"
)

// newBillingTestRouter 组装一个只用来验路由与中间件的 router：所有 store 传 nil，
// 因为这里只走“路由存在 / 中间件拦下”的路径，不会真的读库。
func newBillingTestRouter(t *testing.T, billingSvc *service.Service, internalToken string) *gin.Engine {
	t.Helper()
	return newTestRouter(t, billingSvc, nil, false)
}

// newTestRouter 是 newRouter 的测试入口：store 全为 nil，apikey 按需给。
func newTestRouter(t *testing.T, billingSvc *service.Service, apikeySvc *apikey.Service, apikeyAdminOnly bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	secret := []byte("test-secret")
	sign := func(userID string, isAdmin bool) (string, error) { return signToken(secret, userID, isAdmin) }
	return newRouter(secret,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		sign, nil, nil, nil, nil, nil, nil, nil, nil,
		"internal-token", billingSvc, nil, apikeySvc, apikeyAdminOnly)
}

func hasRoute(r *gin.Engine, method, path string) bool {
	for _, route := range r.Routes() {
		if route.Method == method && route.Path == path {
			return true
		}
	}
	return false
}

func TestBillingRoutesWhenDisabled(t *testing.T) {
	// 计费关闭（billing.enabled: false）：内部接口根本不注册（不存在的端点该是 404，
	// 而不是一个永远回 503 的空壳），/api 上的接口仍然注册着并回 503。
	r := newBillingTestRouter(t, nil, "internal-token")

	for _, path := range []string{"/internal/billing/authorize", "/internal/billing/usage-events", "/internal/billing/settle"} {
		if hasRoute(r, http.MethodPost, path) {
			t.Errorf("POST %s must not be registered when billing is disabled", path)
		}
	}

	token, err := signToken([]byte("test-secret"), "user-1", false)
	if err != nil {
		t.Fatalf("signToken() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/billing/summary", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /api/billing/summary with billing disabled = %d, want %d (body %s)",
			response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
}

func TestBillingRoutesWhenEnabled(t *testing.T) {
	svc := service.New(nil, nil, service.Config{})
	r := newBillingTestRouter(t, svc, "internal-token")

	cases := []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodPost, "/internal/billing/authorize", true},
		{http.MethodPost, "/internal/billing/usage-events", true},
		{http.MethodPost, "/internal/billing/settle", true},
		// 存量 /internal/* 接口（P1 不动它们的鉴权范围，这里只当作“没被误删”的锚点）。
		{http.MethodGet, "/internal/device-config", true},
		{http.MethodGet, "/api/billing/items", true},
		{http.MethodPut, "/api/billing/items/:code", true},
		{http.MethodGet, "/api/billing/prices", true},
		{http.MethodPost, "/api/billing/prices", true},
		{http.MethodPut, "/api/billing/prices/:id", true},
		{http.MethodDelete, "/api/billing/prices/:id", true},
		{http.MethodGet, "/api/billing/accounts", true},
		{http.MethodPost, "/api/billing/accounts/:id/adjust", true},
		{http.MethodGet, "/api/billing/ledger", true},
		{http.MethodGet, "/api/billing/stats", true},
		{http.MethodGet, "/api/billing/summary", true},
		{http.MethodGet, "/api/billing/usage", true},
		// 充值：同一个 /api/billing/recharge 下同时有静态路径（列表）与参数路径
		// （查单），Gin 的路由树能共存；回调两条是匿名路由，绝不能挂 JWT。
		{http.MethodPost, "/api/billing/recharge", true},
		{http.MethodGet, "/api/billing/recharge", true},
		{http.MethodGet, "/api/billing/recharge/config", true},
		{http.MethodGet, "/api/billing/recharge/:out_trade_no", true},
		{http.MethodPost, "/api/billing/recharge/:out_trade_no/refund", true},
		{http.MethodPost, "/pay/epay/notify", true},
		{http.MethodGet, "/pay/epay/notify", true},
		{http.MethodGet, "/pay/epay/return", true},
	}
	for _, tc := range cases {
		if got := hasRoute(r, tc.method, tc.path); got != tc.want {
			t.Errorf("%s %s registered = %v, want %v", tc.method, tc.path, got, tc.want)
		}
	}

	// 内部口径：没带 token 一律 401（细节见 middleware.InternalAuth 的单测）。
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/internal/billing/authorize", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /internal/billing/authorize = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	// 管理端口径：普通用户的 JWT 进不去（中间件先拦，不会碰到 store）。
	userToken, err := signToken([]byte("test-secret"), "user-1", false)
	if err != nil {
		t.Fatalf("signToken() error = %v", err)
	}
	adminOnly := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/billing/items"},
		{http.MethodGet, "/api/billing/accounts"},
		{http.MethodGet, "/api/billing/ledger"},
		{http.MethodGet, "/api/billing/stats"},
	}
	for _, tc := range adminOnly {
		request := httptest.NewRequest(tc.method, tc.path, nil)
		request.Header.Set("Authorization", "Bearer "+userToken)
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Errorf("%s %s as a regular user = %d, want %d", tc.method, tc.path, response.Code, http.StatusForbidden)
		}
	}
}

// fakeKeyStore 是 *store.APIKeyStore 的内联替身（仓库约定：不引 mock 生成器）。
type fakeKeyStore struct {
	rows map[string]*store.APIKey
}

func newFakeKeyStore() *fakeKeyStore { return &fakeKeyStore{rows: map[string]*store.APIKey{}} }

func (s *fakeKeyStore) Create(_ context.Context, k *store.APIKey) error {
	row := *k
	s.rows[k.ID] = &row
	return nil
}

func (s *fakeKeyStore) GetByLookup(_ context.Context, lookup string) (*store.APIKey, error) {
	for _, row := range s.rows {
		if row.Lookup == lookup {
			cp := *row
			return &cp, nil
		}
	}
	return nil, store.ErrNotFound
}

func (s *fakeKeyStore) ListByUser(context.Context, string) ([]store.APIKey, error) { return nil, nil }
func (s *fakeKeyStore) CountByUser(context.Context, string) (int64, error)         { return 0, nil }

func (s *fakeKeyStore) Revoke(_ context.Context, id, userID string, at time.Time) error {
	row, ok := s.rows[id]
	if !ok || row.UserID != userID {
		return store.ErrNotFound
	}
	if row.RevokedAt == nil {
		row.RevokedAt = &at
	}
	return nil
}

func (s *fakeKeyStore) AddUsage(context.Context, string, int64, time.Time) error { return nil }

// routeRequest 发一个带 Bearer 的请求。
func routeRequest(r *gin.Engine, method, path, bearer string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	return response
}

// TestAPIKeyRouteCoverage 是 §7 V3 的双向覆盖性测试：正向看每条 /api/* 有没有归属
// （scope 声明或"key 不可达"白名单），反向看目录里每个 scope 有没有被路由用到。
func TestAPIKeyRouteCoverage(t *testing.T) {
	r := newTestRouter(t, nil, nil, false)
	problems := middleware.ValidateCoverage(r.Routes())
	if len(problems) > 0 {
		t.Fatalf("路由与 scope 表的双向覆盖性校验失败：\n  - %s", strings.Join(problems, "\n  - "))
	}
}

// TestAPIKeyEndToEnd 走完 G1（key 与 JWT 的响应逐字节一致）、G3（撤销后立刻失效）
// 与 FR-5（缺 scope 的 403 可诊断）三条主线。
func TestAPIKeyEndToEnd(t *testing.T) {
	st := newFakeKeyStore()
	// 限流放得很宽：这条测试关心的是授权，不是配额。
	svc := apikey.New(st, apikey.Config{RateLimitRPS: 1000, RateLimitBurst: 1000})
	r := newTestRouter(t, nil, svc, false)

	ctx := context.Background()
	readOnlyKey, _, err := svc.Create(ctx, "user-1", "agent-only", []string{apikey.ScopeAgentRead}, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	dataKey, _, err := svc.Create(ctx, "user-1", "data", []string{apikey.ScopeDataRead}, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	jwtToken, err := signToken([]byte("test-secret"), "user-1", false)
	if err != nil {
		t.Fatalf("signToken() error = %v", err)
	}

	// G1：同一条请求，key 与 JWT 的响应逐字节一致。
	viaJWT := routeRequest(r, http.MethodGet, "/api/sessions", jwtToken)
	viaKey := routeRequest(r, http.MethodGet, "/api/sessions", dataKey)
	if viaJWT.Code != http.StatusOK || viaKey.Code != http.StatusOK {
		t.Fatalf("GET /api/sessions = %d (jwt) / %d (key), want 200 both (body %s)",
			viaJWT.Code, viaKey.Code, viaKey.Body.String())
	}
	if viaJWT.Body.String() != viaKey.Body.String() {
		t.Fatalf("key response %q differs from jwt response %q", viaKey.Body.String(), viaJWT.Body.String())
	}

	// FR-5：缺 scope 的 403 要能自诊断。
	denied := routeRequest(r, http.MethodGet, "/api/sessions", readOnlyKey)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("read-only key = %d, want 403 (body %s)", denied.Code, denied.Body.String())
	}
	if !strings.Contains(denied.Body.String(), apikey.ScopeDataRead) {
		t.Fatalf("403 body must tell the caller which scope is required: %s", denied.Body.String())
	}

	// 撤销后第一个请求即失效，不需要等任何缓存（G3）。
	identity, err := svc.Authenticate(ctx, dataKey)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if err := svc.Revoke(ctx, "user-1", identity.KeyID); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	revoked := routeRequest(r, http.MethodGet, "/api/sessions", dataKey)
	if revoked.Code != http.StatusUnauthorized {
		t.Fatalf("request with a revoked key = %d, want 401 (body %s)", revoked.Code, revoked.Body.String())
	}

	// 机器凭证到不了凭证管理面与账号面：那两组只挂 JWT。
	for _, path := range []string{"/api/api-keys", "/api/auth/profile"} {
		if got := routeRequest(r, http.MethodGet, path, dataKey); got.Code == http.StatusOK {
			t.Errorf("GET %s with an API key = 200, want it to be unreachable", path)
		}
	}
}

// TestAPIKeyRoutesWhenDisabled：apikey.enabled: false —— 管理面回 503，key 一律 401。
func TestAPIKeyRoutesWhenDisabled(t *testing.T) {
	r := newTestRouter(t, nil, nil, false)

	for _, path := range []string{"/api/api-keys", "/api/api-keys/scopes"} {
		if !hasRoute(r, http.MethodGet, path) {
			t.Errorf("GET %s must stay registered (回 503，而不是 404)", path)
		}
	}
	if !hasRoute(r, http.MethodPost, "/api/api-keys") {
		t.Error("POST /api/api-keys must stay registered")
	}
	if !hasRoute(r, http.MethodDelete, "/api/api-keys/:id") {
		t.Error("DELETE /api/api-keys/:id must stay registered")
	}

	token, err := signToken([]byte("test-secret"), "user-1", false)
	if err != nil {
		t.Fatalf("signToken() error = %v", err)
	}
	response := routeRequest(r, http.MethodGet, "/api/api-keys", token)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /api/api-keys with apikey disabled = %d, want 503 (body %s)",
			response.Code, response.Body.String())
	}

	// key 请求与"没这个功能时"一致：401。
	response = routeRequest(r, http.MethodGet, "/api/sessions", "ox_sk_"+strings.Repeat("x", 55))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("API key request with apikey disabled = %d, want 401", response.Code)
	}
}

// TestAPIKeyRoutesWhenEnabled：开关打开时四条管理路由都在，且 key 能拿到目录。
func TestAPIKeyRoutesWhenEnabled(t *testing.T) {
	r := newTestRouter(t, nil, apikey.New(newFakeKeyStore(), apikey.Config{}), false)

	cases := []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodGet, "/api/api-keys", true},
		{http.MethodPost, "/api/api-keys", true},
		{http.MethodGet, "/api/api-keys/scopes", true},
		{http.MethodDelete, "/api/api-keys/:id", true},
	}
	for _, tc := range cases {
		if got := hasRoute(r, tc.method, tc.path); got != tc.want {
			t.Errorf("%s %s registered = %v, want %v", tc.method, tc.path, got, tc.want)
		}
	}

	// 资源路由仍然只认 JWT 或 key，没带凭证一律 401。
	if response := routeRequest(r, http.MethodGet, "/api/sessions", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/sessions = %d, want 401", response.Code)
	}
}
