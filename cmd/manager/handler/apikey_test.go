package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/apikey"
	"github.com/liuscraft/orion-x/internal/store"
)

// fakeAPIKeyStore 是 *store.APIKeyStore 的内联替身（仓库约定：不引 mock 生成器）。
type fakeAPIKeyStore struct {
	rows        map[string]*store.APIKey
	createCalls int
}

func newFakeAPIKeyStore() *fakeAPIKeyStore {
	return &fakeAPIKeyStore{rows: make(map[string]*store.APIKey)}
}

func (s *fakeAPIKeyStore) Create(_ context.Context, k *store.APIKey) error {
	s.createCalls++
	row := *k
	row.CreatedAt = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	s.rows[k.ID] = &row
	return nil
}

func (s *fakeAPIKeyStore) GetByLookup(_ context.Context, lookup string) (*store.APIKey, error) {
	for _, row := range s.rows {
		if row.Lookup == lookup {
			cp := *row
			return &cp, nil
		}
	}
	return nil, store.ErrNotFound
}

func (s *fakeAPIKeyStore) ListByUser(_ context.Context, userID string) ([]store.APIKey, error) {
	var out []store.APIKey
	for _, row := range s.rows {
		if row.UserID == userID {
			out = append(out, *row)
		}
	}
	return out, nil
}

func (s *fakeAPIKeyStore) CountByUser(_ context.Context, userID string) (int64, error) {
	var n int64
	for _, row := range s.rows {
		if row.UserID == userID {
			n++
		}
	}
	return n, nil
}

func (s *fakeAPIKeyStore) Revoke(_ context.Context, id, userID string, at time.Time) error {
	row, ok := s.rows[id]
	if !ok || row.UserID != userID {
		return store.ErrNotFound
	}
	if row.RevokedAt == nil {
		row.RevokedAt = &at
	}
	return nil
}

func (s *fakeAPIKeyStore) AddUsage(context.Context, string, int64, time.Time) error { return nil }

// apiKeyRequest 直接驱动 handler：上下文里的 userID 与 JWT 中间件写的是同一个键。
func apiKeyRequest(method, path, body, userID string, isAdmin bool, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	if userID != "" {
		c.Set("userID", userID)
	}
	if isAdmin {
		c.Set("isAdmin", true)
	}
	return c, w
}

// invoke 调一次 handler。gin 的 responseWriter 是延迟写状态的（由引擎在 handler 链
// 结束时 flush），直接调 handler 时得自己补上这一步，否则 204 这类没有 body 的响应
// 在 recorder 上只能看到默认的 200。
func invoke(c *gin.Context, fn func(*gin.Context)) {
	fn(c)
	c.Writer.WriteHeaderNow()
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	return out
}

func TestAPIKeyCreateReturnsPlaintextOnce(t *testing.T) {
	st := newFakeAPIKeyStore()
	h := NewAPIKeyHandler(apikey.New(st, apikey.Config{}), false)

	c, w := apiKeyRequest(http.MethodPost, "/api/api-keys",
		`{"name":"生产环境","scopes":["agent:read","data:read"]}`, "user-1", false, nil)
	h.Create(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("Create = %d, want 201 (body %s)", w.Code, w.Body.String())
	}

	created := decodeBody(t, w)
	plaintext, _ := created["key"].(string)
	if plaintext == "" {
		t.Fatalf("create response has no plaintext key: %s", w.Body.String())
	}
	if _, _, ok := apikey.Parse(plaintext); !ok {
		t.Fatalf("create response key %q is not a valid key", plaintext)
	}
	view, _ := created["api_key"].(map[string]any)
	if view == nil {
		t.Fatalf("create response must carry an api_key object: %s", w.Body.String())
	}
	if view["masked_key"] == plaintext {
		t.Fatal("masked_key must not be the plaintext")
	}
	if view["masked_key"] == "" || view["id"] == nil {
		t.Fatalf("api_key = %v", view)
	}
	for _, leaked := range []string{"hash", "lookup", "user_id", "creator"} {
		if _, ok := view[leaked]; ok {
			t.Errorf("api_key view leaks %q", leaked)
		}
	}

	// 列表：掩码、无明文（FR-2）。
	c, w = apiKeyRequest(http.MethodGet, "/api/api-keys", "", "user-1", false, nil)
	h.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("List = %d, want 200", w.Code)
	}
	if strings.Contains(w.Body.String(), plaintext) {
		t.Fatalf("list response leaks the plaintext: %s", w.Body.String())
	}
	listed := decodeBody(t, w)
	if _, ok := listed["key"]; ok {
		t.Fatal("list response must not carry a key field")
	}
	if listed["total"] != float64(1) || listed["page"] != float64(1) {
		t.Fatalf("page/total = (%v, %v), want (1, 1)", listed["page"], listed["total"])
	}
	data, _ := listed["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("data = %v, want one entry", listed["data"])
	}
	item, _ := data[0].(map[string]any)
	if item["masked_key"] == "" || item["masked_key"] == plaintext {
		t.Fatalf("masked_key = %v", item["masked_key"])
	}
	if item["call_count"] != float64(0) {
		t.Fatalf("call_count = %v, want 0", item["call_count"])
	}
}

// 列表分页按仓库惯例：page 从 1 起，超出范围就是空页。
func TestAPIKeyListPagination(t *testing.T) {
	st := newFakeAPIKeyStore()
	h := NewAPIKeyHandler(apikey.New(st, apikey.Config{}), false)

	for i := 0; i < 3; i++ {
		c, w := apiKeyRequest(http.MethodPost, "/api/api-keys", `{"name":"k","scopes":["agent:read"]}`, "user-1", false, nil)
		h.Create(c)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %d = %d", i, w.Code)
		}
	}

	c, w := apiKeyRequest(http.MethodGet, "/api/api-keys?page=2&page_size=2", "", "user-1", false, nil)
	h.List(c)
	body := decodeBody(t, w)
	if body["total"] != float64(3) || body["page"] != float64(2) {
		t.Fatalf("page/total = (%v, %v), want (2, 3)", body["page"], body["total"])
	}
	if data, _ := body["data"].([]any); len(data) != 1 {
		t.Fatalf("second page = %v, want one entry", body["data"])
	}

	// 越界页返回空数组（不是 null）：客户端不用为空值写分支。
	c, w = apiKeyRequest(http.MethodGet, "/api/api-keys?page=9", "", "user-1", false, nil)
	h.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("out-of-range page = %d, want 200", w.Code)
	}
	body = decodeBody(t, w)
	if data, ok := body["data"].([]any); !ok || len(data) != 0 {
		t.Fatalf("out-of-range page data = %v, want []", body["data"])
	}
}

func TestAPIKeyCreateExpandsPreset(t *testing.T) {
	st := newFakeAPIKeyStore()
	h := NewAPIKeyHandler(apikey.New(st, apikey.Config{}), false)

	c, w := apiKeyRequest(http.MethodPost, "/api/api-keys", `{"name":"ci","preset":"readonly"}`, "user-1", false, nil)
	h.Create(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("Create = %d, want 201 (body %s)", w.Code, w.Body.String())
	}

	scopes, _ := decodeBody(t, w)["api_key"].(map[string]any)["scopes"].([]any)
	want, _ := apikey.ExpandPreset("readonly")
	var got []string
	for _, s := range scopes {
		got = append(got, fmt.Sprint(s))
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("created scopes = %v, want the expanded preset %v", got, want)
	}
	// 预设只在创建时展开：库里存的是显式列表，不是预设名。
	for _, row := range st.rows {
		if strings.Join(row.Scopes, ",") != strings.Join(want, ",") {
			t.Fatalf("stored scopes = %v, want %v", row.Scopes, want)
		}
	}
}

func TestAPIKeyCreateErrorMapping(t *testing.T) {
	longName := strings.Repeat("x", 65)
	cases := []struct {
		name       string
		body       string
		maxPerAcct int
		wantStatus int
	}{
		{name: "invalid json", body: `{`, wantStatus: http.StatusBadRequest},
		{name: "missing name", body: `{"scopes":["agent:read"]}`, wantStatus: http.StatusBadRequest},
		{name: "blank name", body: `{"name":"  ","scopes":["agent:read"]}`, wantStatus: http.StatusBadRequest},
		{name: "name too long", body: fmt.Sprintf(`{"name":%q,"scopes":["agent:read"]}`, longName), wantStatus: http.StatusBadRequest},
		{name: "missing scopes", body: `{"name":"ci"}`, wantStatus: http.StatusBadRequest},
		{name: "unknown scope", body: `{"name":"ci","scopes":["agent:admin"]}`, wantStatus: http.StatusBadRequest},
		{name: "unknown preset", body: `{"name":"ci","preset":"everything"}`, wantStatus: http.StatusBadRequest},
		{name: "expired timestamp", body: `{"name":"ci","scopes":["agent:read"],"expires_at":"2020-01-01T00:00:00Z"}`, wantStatus: http.StatusBadRequest},
		{
			name: "account quota", body: `{"name":"ci","scopes":["agent:read"]}`,
			maxPerAcct: 1, wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := newFakeAPIKeyStore()
			cfg := apikey.Config{}
			if tc.maxPerAcct > 0 {
				cfg.MaxKeysPerAccount = tc.maxPerAcct
			}
			h := NewAPIKeyHandler(apikey.New(st, cfg), false)

			if tc.maxPerAcct > 0 {
				c, w := apiKeyRequest(http.MethodPost, "/api/api-keys", `{"name":"first","scopes":["agent:read"]}`, "user-1", false, nil)
				h.Create(c)
				if w.Code != http.StatusCreated {
					t.Fatalf("fixture create = %d", w.Code)
				}
			}

			c, w := apiKeyRequest(http.MethodPost, "/api/api-keys", tc.body, "user-1", false, nil)
			h.Create(c)
			if w.Code != tc.wantStatus {
				t.Fatalf("Create = %d, want %d (body %s)", w.Code, tc.wantStatus, w.Body.String())
			}
			if body := decodeBody(t, w); body["error"] == "" {
				t.Fatalf("error response has no message: %s", w.Body.String())
			}
		})
	}
}

func TestAPIKeyRevokeIsIdempotentAndScoped(t *testing.T) {
	st := newFakeAPIKeyStore()
	h := NewAPIKeyHandler(apikey.New(st, apikey.Config{}), false)

	c, w := apiKeyRequest(http.MethodPost, "/api/api-keys", `{"name":"ci","scopes":["agent:read"]}`, "user-1", false, nil)
	h.Create(c)
	id, _ := decodeBody(t, w)["api_key"].(map[string]any)["id"].(string)
	if id == "" {
		t.Fatal("create did not return an id")
	}

	params := gin.Params{{Key: "id", Value: id}}

	// 别人的 key：404（不区分"不存在"与"不是你的"，免得变成探测工具）。
	c, w = apiKeyRequest(http.MethodDelete, "/api/api-keys/"+id, "", "user-2", false, params)
	invoke(c, h.Revoke)
	if w.Code != http.StatusNotFound {
		t.Fatalf("revoke as another user = %d, want 404", w.Code)
	}

	c, w = apiKeyRequest(http.MethodDelete, "/api/api-keys/"+id, "", "user-1", false, params)
	invoke(c, h.Revoke)
	if w.Code != http.StatusNoContent {
		t.Fatalf("revoke = %d, want 204", w.Code)
	}
	if st.rows[id].RevokedAt == nil {
		t.Fatal("revoke did not persist revoked_at")
	}

	// 幂等：第二次也是 204（FR-3 锁死这一种行为）。
	c, w = apiKeyRequest(http.MethodDelete, "/api/api-keys/"+id, "", "user-1", false, params)
	invoke(c, h.Revoke)
	if w.Code != http.StatusNoContent {
		t.Fatalf("second revoke = %d, want 204", w.Code)
	}

	c, w = apiKeyRequest(http.MethodDelete, "/api/api-keys/nope", "", "user-1", false, gin.Params{{Key: "id", Value: "nope"}})
	invoke(c, h.Revoke)
	if w.Code != http.StatusNotFound {
		t.Fatalf("revoke unknown id = %d, want 404", w.Code)
	}

	// 列表里仍然看得见，并且带着撤销时间。
	c, w = apiKeyRequest(http.MethodGet, "/api/api-keys", "", "user-1", false, nil)
	h.List(c)
	data, _ := decodeBody(t, w)["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("data = %v", data)
	}
	if item, _ := data[0].(map[string]any); item["revoked_at"] == nil {
		t.Fatalf("revoked key is not marked in the list: %v", item)
	}
}

func TestAPIKeyListIsScopedToOwner(t *testing.T) {
	st := newFakeAPIKeyStore()
	h := NewAPIKeyHandler(apikey.New(st, apikey.Config{}), false)

	for _, owner := range []string{"user-1", "user-2"} {
		c, w := apiKeyRequest(http.MethodPost, "/api/api-keys", `{"name":"k","scopes":["agent:read"]}`, owner, false, nil)
		h.Create(c)
		if w.Code != http.StatusCreated {
			t.Fatalf("create for %s = %d", owner, w.Code)
		}
	}

	c, w := apiKeyRequest(http.MethodGet, "/api/api-keys", "", "user-1", false, nil)
	h.List(c)
	data, _ := decodeBody(t, w)["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("data = %v, want only the caller's key", data)
	}
}

func TestAPIKeyScopesEndpoint(t *testing.T) {
	h := NewAPIKeyHandler(apikey.New(newFakeAPIKeyStore(), apikey.Config{}), false)

	c, w := apiKeyRequest(http.MethodGet, "/api/api-keys/scopes", "", "user-1", false, nil)
	h.Scopes(c)
	if w.Code != http.StatusOK {
		t.Fatalf("Scopes = %d, want 200", w.Code)
	}

	body := decodeBody(t, w)
	scopes, _ := body["scopes"].([]any)
	if len(scopes) != len(apikey.Catalog()) || len(scopes) == 0 {
		t.Fatalf("scopes = %d entries, want %d", len(scopes), len(apikey.Catalog()))
	}
	groups, _ := body["groups"].([]any)
	if len(groups) != len(apikey.Groups()) {
		t.Fatalf("groups = %d entries, want %d", len(groups), len(apikey.Groups()))
	}
	presets, _ := body["presets"].([]any)
	if len(presets) != len(apikey.Presets()) {
		t.Fatalf("presets = %d entries, want %d", len(presets), len(apikey.Presets()))
	}
	first, _ := scopes[0].(map[string]any)
	for _, field := range []string{"value", "group", "title", "desc", "default", "deprecated"} {
		if _, ok := first[field]; !ok {
			t.Errorf("scope payload is missing %q: %v", field, first)
		}
	}
	firstPreset, _ := presets[0].(map[string]any)
	for _, field := range []string{"name", "title", "scopes"} {
		if _, ok := firstPreset[field]; !ok {
			t.Errorf("preset payload is missing %q: %v", field, firstPreset)
		}
	}
	if body["can_create"] != true {
		t.Fatalf("can_create = %v, want true", body["can_create"])
	}
}

func TestAPIKeyAdminOnlyGate(t *testing.T) {
	st := newFakeAPIKeyStore()
	h := NewAPIKeyHandler(apikey.New(st, apikey.Config{}), true)

	// 灰度期：普通账号连"能不能创建"都要看得出来，别等提交后才 403。
	c, w := apiKeyRequest(http.MethodGet, "/api/api-keys/scopes", "", "user-1", false, nil)
	h.Scopes(c)
	if got := decodeBody(t, w)["can_create"]; got != false {
		t.Fatalf("can_create for a regular user = %v, want false", got)
	}
	c, w = apiKeyRequest(http.MethodGet, "/api/api-keys/scopes", "", "admin", true, nil)
	h.Scopes(c)
	if got := decodeBody(t, w)["can_create"]; got != true {
		t.Fatalf("can_create for an admin = %v, want true", got)
	}

	c, w = apiKeyRequest(http.MethodPost, "/api/api-keys", `{"name":"ci","scopes":["agent:read"]}`, "user-1", false, nil)
	h.Create(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("create as a regular user = %d, want 403", w.Code)
	}
	if st.createCalls != 0 {
		t.Fatal("the gate must reject before touching the store")
	}

	c, w = apiKeyRequest(http.MethodPost, "/api/api-keys", `{"name":"ci","scopes":["agent:read"]}`, "admin", true, nil)
	h.Create(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("create as an admin = %d, want 201 (body %s)", w.Code, w.Body.String())
	}
}

func TestAPIKeyDisabledReturns503(t *testing.T) {
	h := NewAPIKeyHandler(nil, false)
	cases := []struct {
		name   string
		invoke func(*gin.Context)
	}{
		{name: "list", invoke: h.List},
		{name: "create", invoke: h.Create},
		{name: "revoke", invoke: h.Revoke},
		{name: "scopes", invoke: h.Scopes},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, w := apiKeyRequest(http.MethodGet, "/api/api-keys", "", "user-1", false, nil)
			tc.invoke(c)
			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503 (FR-9)", w.Code)
			}
		})
	}
}
