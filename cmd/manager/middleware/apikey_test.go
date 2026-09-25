package middleware

import (
	"context"
	"encoding/json"
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

const testSecret = "test-secret"

// stubStore 是 *store.APIKeyStore 的内联替身（仓库约定：不引 mock 生成器）。
type stubStore struct {
	row   *store.APIKey
	usage []usageCall
}

type usageCall struct {
	ID    string
	Calls int64
}

func (s *stubStore) Create(context.Context, *store.APIKey) error { return nil }

func (s *stubStore) GetByLookup(_ context.Context, lookup string) (*store.APIKey, error) {
	if s.row != nil && s.row.Lookup == lookup {
		cp := *s.row
		return &cp, nil
	}
	return nil, store.ErrNotFound
}

func (s *stubStore) ListByUser(context.Context, string) ([]store.APIKey, error) { return nil, nil }
func (s *stubStore) CountByUser(context.Context, string) (int64, error)         { return 0, nil }
func (s *stubStore) Revoke(context.Context, string, string, time.Time) error    { return nil }

func (s *stubStore) AddUsage(_ context.Context, id string, calls int64, _ time.Time) error {
	s.usage = append(s.usage, usageCall{ID: id, Calls: calls})
	return nil
}

// keyFixture 造一把可用的 key 与它的 service。
func keyFixture(t *testing.T, scopes []string, cfg apikey.Config) (*apikey.Service, *stubStore, string, *store.APIKey) {
	t.Helper()
	plaintext, record, err := apikey.Generate("user-1", "ci", scopes, nil)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	st := &stubStore{row: &record}
	return apikey.New(st, cfg), st, plaintext, &record
}

func signTestJWT(t *testing.T, userID string, isAdmin bool) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":      userID,
		"is_admin": isAdmin,
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("signedString() error = %v", err)
	}
	return signed
}

// identityRoute 是一个最小资源路由：把中间件产出的事实回显出来，方便断言。
func identityRoute(svc *apikey.Service, table ScopeTable) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/voicebots", Auth([]byte(testSecret), svc), RequireScopes(table), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id":   UserID(c),
			"principal": Principal(c),
			"key_id":    KeyID(c),
			"scopes":    Scopes(c),
			"is_admin":  IsAdmin(c),
		})
	})
	return r
}

func do(r *gin.Engine, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/voicebots", nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

var agentReadTable = ScopeTable{"GET /api/voicebots": {apikey.ScopeAgentRead}}

func TestAuthKeepsJWTSemantics(t *testing.T) {
	svc, _, _, _ := keyFixture(t, []string{apikey.ScopeAgentRead}, apikey.Config{})
	r := identityRoute(svc, agentReadTable)

	// 老客户端不带 ox_sk_ 前缀：走的还是原来的 JWT 分支。
	w := do(r, signTestJWT(t, "user-9", false))
	if w.Code != http.StatusOK {
		t.Fatalf("JWT request = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["user_id"] != "user-9" || body["principal"] != PrincipalJWT {
		t.Fatalf("body = %v, want the JWT identity", body)
	}
	if body["key_id"] != "" {
		t.Fatalf("JWT request carried a key id: %v", body)
	}

	// JWT 是人：scope 表不约束它（权限由角色与资源归属决定）。
	if w := do(r, signTestJWT(t, "user-9", true)); w.Code != http.StatusOK {
		t.Fatalf("admin JWT request = %d, want 200", w.Code)
	}

	if w := do(r, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("missing token = %d, want 401", w.Code)
	}
	if w := do(r, "not-a-jwt"); w.Code != http.StatusUnauthorized {
		t.Fatalf("garbage bearer = %d, want 401", w.Code)
	}
}

func TestAuthAPIKeyFailures(t *testing.T) {
	svc, st, plaintext, record := keyFixture(t, []string{apikey.ScopeAgentRead}, apikey.Config{})
	r := identityRoute(svc, agentReadTable)

	cases := []struct {
		name      string
		bearer    string
		mutate    func()
		wantCode  int
		wantError string
	}{
		{
			name:      "malformed key is rejected before the store",
			bearer:    plaintext[:len(plaintext)-1] + "0",
			wantCode:  http.StatusUnauthorized,
			wantError: "invalid api key",
		},
		{
			name:      "revoked",
			bearer:    plaintext,
			mutate:    func() { at := time.Now(); st.row.RevokedAt = &at },
			wantCode:  http.StatusUnauthorized,
			wantError: "api key revoked",
		},
		{
			name:      "expired",
			bearer:    plaintext,
			mutate:    func() { at := time.Now().Add(-time.Second); st.row.ExpiresAt = &at },
			wantCode:  http.StatusUnauthorized,
			wantError: "api key expired",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mutate != nil {
				tc.mutate()
				t.Cleanup(func() { st.row.RevokedAt, st.row.ExpiresAt = nil, nil })
			}
			w := do(r, tc.bearer)
			if w.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tc.wantCode, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.wantError) {
				t.Fatalf("body = %s, want it to mention %q", w.Body.String(), tc.wantError)
			}
			if strings.Contains(w.Body.String(), plaintext) {
				t.Fatal("error response leaks the plaintext key")
			}
		})
	}

	if st.row.ID != record.ID {
		t.Fatal("fixture mutated unexpectedly")
	}
}

func TestAuthWithAPIDisabled(t *testing.T) {
	// apikey.enabled: false：key 中间件拿不到 service，key 请求一律 401，
	// 与"还没实现这个功能"时的行为一致（FR-9）。
	_, _, plaintext, _ := keyFixture(t, []string{apikey.ScopeAgentRead}, apikey.Config{})
	r := identityRoute(nil, agentReadTable)

	w := do(r, plaintext)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid api key") {
		t.Fatalf("body = %s", w.Body.String())
	}
	// JWT 那条路不受影响。
	if w := do(r, signTestJWT(t, "user-9", false)); w.Code != http.StatusOK {
		t.Fatalf("JWT request with apikey disabled = %d, want 200", w.Code)
	}
}

func TestRequireScopes(t *testing.T) {
	svc, _, plaintext, _ := keyFixture(t, []string{apikey.ScopeAgentRead}, apikey.Config{})
	r := identityRoute(svc, ScopeTable{"GET /api/voicebots": {apikey.ScopeAgentWrite}})

	w := do(r, plaintext)
	if w.Code != http.StatusForbidden {
		t.Fatalf("missing scope = %d, want 403 (body %s)", w.Code, w.Body.String())
	}
	var body struct {
		Error    string   `json:"error"`
		Required []string `json:"required"`
		Granted  []string `json:"granted"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error != "insufficient scope" {
		t.Errorf("error = %q", body.Error)
	}
	// 403 要能自诊断：required / granted 都得给（G4）。
	if strings.Join(body.Required, ",") != apikey.ScopeAgentWrite {
		t.Errorf("required = %v, want [%s]", body.Required, apikey.ScopeAgentWrite)
	}
	if strings.Join(body.Granted, ",") != apikey.ScopeAgentRead {
		t.Errorf("granted = %v, want [%s]", body.Granted, apikey.ScopeAgentRead)
	}
}

func TestRequireScopesDeniesUndeclaredRoute(t *testing.T) {
	svc, _, plaintext, _ := keyFixture(t, []string{apikey.ScopeAgentRead}, apikey.Config{})
	r := identityRoute(svc, ScopeTable{}) // 表里没有这条路由

	w := do(r, plaintext)
	if w.Code != http.StatusForbidden {
		t.Fatalf("undeclared route = %d, want 403 (deny-by-default)", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"required":[]`) {
		t.Fatalf("body = %s, want an empty required list", w.Body.String())
	}
}

func TestAuthRateLimit(t *testing.T) {
	svc, st, plaintext, record := keyFixture(t, []string{apikey.ScopeAgentRead}, apikey.Config{RateLimitRPS: 1, RateLimitBurst: 1})
	r := identityRoute(svc, agentReadTable)

	if w := do(r, plaintext); w.Code != http.StatusOK {
		t.Fatalf("first request = %d, want 200", w.Code)
	}

	w := do(r, plaintext)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("second request = %d, want 429 (body %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "rate limit exceeded") {
		t.Fatalf("body = %s", w.Body.String())
	}
	if got := w.Header().Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After = %q, want 1", got)
	}
	if got := w.Header().Get("X-RateLimit-Limit"); got != "1" {
		t.Errorf("X-RateLimit-Limit = %q, want 1", got)
	}
	if got := w.Header().Get("X-RateLimit-Remaining"); got != "0" {
		t.Errorf("X-RateLimit-Remaining = %q, want 0", got)
	}
	if got := w.Header().Get("X-RateLimit-Reset"); got == "" {
		t.Error("X-RateLimit-Reset must be set on a 429")
	}

	// 被限流的那次不算用量：它没走到业务层（§5.2 路径 A）。
	svc.Flush(context.Background())
	if len(st.usage) != 1 || st.usage[0].Calls != 1 {
		t.Fatalf("usage = %+v, want exactly one counted call for %s", st.usage, record.ID)
	}
}

func TestAPIKeyNeverBecomesAdmin(t *testing.T) {
	svc, _, plaintext, _ := keyFixture(t, []string{apikey.ScopeAgentRead}, apikey.Config{RateLimitRPS: 1, RateLimitBurst: 5})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/voicebots", Auth([]byte(testSecret), svc), RequireAdmin(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	if w := do(r, plaintext); w.Code != http.StatusForbidden {
		t.Fatalf("key request to an admin route = %d, want 403", w.Code)
	}
	if w := do(r, signTestJWT(t, "user-9", true)); w.Code != http.StatusOK {
		t.Fatalf("admin JWT request = %d, want 200", w.Code)
	}
}
