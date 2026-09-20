package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/store"
)

// newDryRunAPIKeyHandler 造一个只生成 SQL、不连库的 handler：这里验的是入参校验与
// 响应形状，落库语句的形状由 internal/store 自己的测试盯。
func newDryRunAPIKeyHandler(t *testing.T) *APIKeyHandler {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DriverName: "pgx",
		DSN:        "postgres://apikey:apikey@127.0.0.1:5432/apikey_test?sslmode=disable",
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Discard,
	})
	if err != nil {
		t.Fatalf("open dry-run db: %v", err)
	}
	boxKey, err := apikey.DeriveSecretKey([]byte("test-master-secret"))
	if err != nil {
		t.Fatalf("derive secret box key: %v", err)
	}
	return NewAPIKeyHandler(store.NewAPIKeyStore(db, boxKey), store.NewUserStore(db))
}

// callAPIKeyHandler 把 handler 挂到真 router 上跑一遍请求，返回状态码与响应体。
// userID 由调用方直接注入，等价于 middleware.Auth 认证成功后写进 context 的那个值。
func callAPIKeyHandler(t *testing.T, h *APIKeyHandler, method, route, target, body string) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Handle(method, route, func(c *gin.Context) {
		c.Set("userID", "user-1")
		switch {
		case method == http.MethodPost && strings.HasSuffix(route, "/reveal"):
			h.Reveal(c)
		case method == http.MethodGet:
			h.List(c)
		case method == http.MethodPost:
			h.Create(c)
		default:
			h.Delete(c)
		}
	})

	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	return response.Code, response.Body.String()
}

func TestNewAPIKeyResponseHidesTheSecret(t *testing.T) {
	plain := apikey.Prefix + strings.Repeat("ab", 24)
	prefix, last4, err := apikey.Display(plain)
	if err != nil {
		t.Fatalf("Display() error = %v", err)
	}

	response := newAPIKeyResponse(&store.APIKey{
		ID:        "key-1",
		Name:      "生产环境",
		KeyHash:   apikey.Hash(plain),
		KeyPrefix: prefix,
		KeyLast4:  last4,
		Scopes:    []string{"apikey:scope:agent"},
	})
	if response.Key != "" {
		t.Errorf("plaintext key must only appear in the create response, got %q", response.Key)
	}
	if response.MaskedKey != prefix+"..."+last4 {
		t.Errorf("MaskedKey = %q", response.MaskedKey)
	}
	secret := strings.TrimPrefix(plain, apikey.Prefix)
	middle := secret[len(prefix)-len(apikey.Prefix) : len(secret)-4]
	if strings.Contains(response.MaskedKey, middle) || strings.Contains(response.MaskedKey, apikey.Hash(plain)) {
		t.Errorf("masked key leaks the secret: %q", response.MaskedKey)
	}
}

func TestAPIKeyCreateValidation(t *testing.T) {
	h := NewAPIKeyHandler(nil, nil) // 校验失败必须先于落库返回，所以 store 可以是 nil

	cases := []struct {
		name string
		body string
	}{
		{name: "not json", body: "{"},
		{name: "missing name", body: `{"name":"  ","scopes":["apikey:scope:all"]}`},
		{name: "name too long", body: `{"name":"` + strings.Repeat("x", maxAPIKeyNameLen+1) + `","scopes":["apikey:scope:all"]}`},
		{name: "missing scopes", body: `{"name":"生产环境"}`},
		{name: "empty scopes", body: `{"name":"生产环境","scopes":[]}`},
		{name: "unknown scope", body: `{"name":"生产环境","scopes":["apikey:scope:admin"]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := callAPIKeyHandler(t, h, http.MethodPost, "/api/apikeys", "/api/apikeys", tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("POST /api/apikeys %s = %d (body %s), want %d", tc.body, status, body, http.StatusBadRequest)
			}
		})
	}
}

func TestAPIKeyCreateReturnsThePlaintextOnce(t *testing.T) {
	h := newDryRunAPIKeyHandler(t)
	status, body := callAPIKeyHandler(t, h, http.MethodPost, "/api/apikeys", "/api/apikeys",
		`{"name":"生产环境","scopes":["apikey:scope:agent","apikey:scope:agent","apikey:scope:data"]}`)

	if status != http.StatusCreated {
		t.Fatalf("status = %d (body %s), want %d", status, body, http.StatusCreated)
	}
	var resp apiKeyResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !apikey.LooksLike(resp.Key) {
		t.Fatalf("Key = %q, want a generated plaintext", resp.Key)
	}
	if _, _, err := apikey.Display(resp.Key); err != nil {
		t.Errorf("returned plaintext is not a valid key: %v", err)
	}
	if resp.Name != "生产环境" || resp.ID == "" {
		t.Errorf("response = %+v", resp)
	}
	if len(resp.Scopes) != 2 || resp.Scopes[0] != "apikey:scope:agent" || resp.Scopes[1] != "apikey:scope:data" {
		t.Errorf("scopes = %v, want the deduped list", resp.Scopes)
	}
	if resp.CallCount != 0 || resp.LastUsedAt != nil {
		t.Errorf("a fresh key must not report usage: %+v", resp)
	}
	if !strings.Contains(resp.MaskedKey, "...") {
		t.Errorf("MaskedKey = %q, want a masked form", resp.MaskedKey)
	}
}

func TestAPIKeyListReturnsAnArray(t *testing.T) {
	h := newDryRunAPIKeyHandler(t)
	status, body := callAPIKeyHandler(t, h, http.MethodGet, "/api/apikeys", "/api/apikeys", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if got := strings.TrimSpace(body); got != "[]" {
		t.Errorf("empty list body = %q, want []", got)
	}
}

func TestAPIKeyDeleteOfSomeoneElsesKeyIsNotFound(t *testing.T) {
	h := newDryRunAPIKeyHandler(t)
	// DryRun 下 RowsAffected 恒为 0：等价于“这个 id 不属于当前用户”，走 404 分支。
	status, body := callAPIKeyHandler(t, h, http.MethodDelete, "/api/apikeys/:id", "/api/apikeys/key-1", "")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d (body %s), want %d", status, body, http.StatusNotFound)
	}
}

func TestAPIKeyRevealValidation(t *testing.T) {
	h := NewAPIKeyHandler(nil, nil) // 缺密码要先于任何落库/解密返回

	cases := []struct {
		name   string
		body   string
		status int
	}{
		{name: "missing body", body: "", status: http.StatusBadRequest},
		{name: "empty password", body: `{"password":""}`, status: http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := callAPIKeyHandler(t, h, http.MethodPost, "/api/apikeys/:id/reveal", "/api/apikeys/key-1/reveal", tc.body)
			if status != tc.status {
				t.Fatalf("status = %d (body %s), want %d", status, body, tc.status)
			}
			if strings.Contains(body, apikey.Prefix) {
				t.Errorf("error responses must not carry a key: %s", body)
			}
		})
	}
}

// TestAPIKeyRevealWithoutAPasswordOnFile 走 OAuth 账号的处境：库里没存密码哈希，
// 验不了身份就得明确告知，而不是拿空哈希去比对。
// DryRun 下取不到 users 行，零值 user 正好等价于“没设密码的账号”。
func TestAPIKeyRevealWithoutAPasswordOnFile(t *testing.T) {
	h := newDryRunAPIKeyHandler(t)
	status, body := callAPIKeyHandler(t, h, http.MethodPost, "/api/apikeys/:id/reveal", "/api/apikeys/key-1/reveal", `{"password":"whatever"}`)
	if status != http.StatusConflict {
		t.Fatalf("status = %d (body %s), want %d", status, body, http.StatusConflict)
	}
}

// TestAPIKeyAuthorizeHandler 只跑得到“没密钥行”这一步（DryRun 造不出行），
// 断言的是 HTTP 通道：入参校验 + 拒绝信封。判定矩阵由 internal/apikey 的
// Decide 单测盯，真实链路由 bin/smoke_apikey.py 的端到端冒烟盯。
func TestAPIKeyAuthorizeHandler(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DriverName: "pgx",
		DSN:        "postgres://apikey:apikey@127.0.0.1:5432/apikey_test?sslmode=disable",
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Discard,
	})
	if err != nil {
		t.Fatalf("open dry-run db: %v", err)
	}
	boxKey, err := apikey.DeriveSecretKey([]byte("test-master-secret"))
	if err != nil {
		t.Fatalf("derive secret box key: %v", err)
	}
	h := NewAPIKeyAuthorizeHandler(store.NewAPIKeyStore(db, boxKey), store.NewDeviceStore(db), store.NewVoicebotStore(db))

	cases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{name: "not json", body: "{", wantStatus: http.StatusBadRequest},
		{name: "missing key", body: `{"device_id":"device-1"}`, wantStatus: http.StatusBadRequest},
		{name: "missing device", body: `{"key":"ox:sk:abc"}`, wantStatus: http.StatusBadRequest},
		{
			name:       "unknown key becomes a rejection envelope",
			body:       `{"key":"ox:sk:` + strings.Repeat("ab", 24) + `","device_id":"device-1"}`,
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.POST("/internal/apikey/authorize", h.Authorize)
			request := httptest.NewRequest(http.MethodPost, "/internal/apikey/authorize", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)

			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d (body %s), want %d", response.Code, response.Body.String(), tc.wantStatus)
			}
			if tc.wantStatus != http.StatusOK {
				return
			}
			var resp apikey.AuthorizeResponse
			if err := json.Unmarshal(response.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if resp.Allowed {
				t.Errorf("response = %+v, want a rejection", resp)
			}
			if resp.RejectReason == apikey.RejectNone {
				t.Errorf("response = %+v, want a machine-readable reject reason", resp)
			}
		})
	}
}
