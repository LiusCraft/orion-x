package handler

import (
	"net/http"
	"testing"

	"github.com/liuscraft/orion-x/internal/apikey"
)

// 400 的响应体是 §5.3 里写明的字面契约：把"改哪个参数"说清楚。
func TestAPIKeyCreateErrorBodies(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "missing name", body: `{"scopes":["agent:read"]}`, want: "name is required"},
		{name: "missing scopes", body: `{"name":"ci"}`, want: "at least one scope is required"},
		{
			name: "expired timestamp",
			body: `{"name":"ci","scopes":["agent:read"],"expires_at":"2020-01-01T00:00:00Z"}`,
			want: "expires_at must be in the future",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewAPIKeyHandler(apikey.New(newFakeAPIKeyStore(), apikey.Config{}), false)
			c, w := apiKeyRequest(http.MethodPost, "/api/api-keys", tc.body, "user-1", false, nil)
			h.Create(c)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("Create = %d, want 400 (body %s)", w.Code, w.Body.String())
			}
			if got := decodeBody(t, w)["error"]; got != tc.want {
				t.Fatalf("error = %v, want %q", got, tc.want)
			}
		})
	}
}

// 未知 scope 不静默丢弃：把被拒的值列出来，否则用户会以为自己授了权。
func TestAPIKeyCreateUnknownScopeListsValues(t *testing.T) {
	h := NewAPIKeyHandler(apikey.New(newFakeAPIKeyStore(), apikey.Config{}), false)

	c, w := apiKeyRequest(http.MethodPost, "/api/api-keys",
		`{"name":"ci","scopes":["agent:read","agent:blah","nope"]}`, "user-1", false, nil)
	h.Create(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("Create = %d, want 400 (body %s)", w.Code, w.Body.String())
	}

	body := decodeBody(t, w)
	if body["error"] != "unknown scope" {
		t.Fatalf("error = %v, want \"unknown scope\"", body["error"])
	}
	unknown, _ := body["unknown"].([]any)
	if len(unknown) != 2 || unknown[0] != "agent:blah" || unknown[1] != "nope" {
		t.Fatalf("unknown = %v, want [agent:blah nope]", body["unknown"])
	}
}

// 废弃的 scope 与未知的分开列：前者是"曾经存在、不再可授"。
func TestAPIKeyCreateDeprecatedScopeIsListed(t *testing.T) {
	h := NewAPIKeyHandler(apikey.New(newFakeAPIKeyStore(), apikey.Config{}), false)

	c, w := apiKeyRequest(http.MethodPost, "/api/api-keys",
		`{"name":"ci","scopes":["agent:read"],"preset":"readonly"}`, "user-1", false, nil)
	h.Create(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("preset expansion must stay accepted: %d (body %s)", w.Code, w.Body.String())
	}

	c, w = apiKeyRequest(http.MethodPost, "/api/api-keys",
		`{"name":"ci","preset":"everything"}`, "user-1", false, nil)
	h.Create(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown preset = %d, want 400", w.Code)
	}
}
