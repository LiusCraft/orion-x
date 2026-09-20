package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/billing/service"
)

// newBillingTestRouter 组装一个只用来验路由与中间件的 router：所有 store 传 nil，
// 因为这里只走“路由存在 / 中间件拦下”的路径，不会真的读库。
func newBillingTestRouter(t *testing.T, billingSvc *service.Service, internalToken string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	secret := []byte("test-secret")
	sign := func(userID string, isAdmin bool) (string, error) { return signToken(secret, userID, isAdmin) }
	return newRouter(secret,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		sign, nil, nil, nil, nil, nil, nil, nil, nil,
		internalToken, billingSvc, nil)
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
