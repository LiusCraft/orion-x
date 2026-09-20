package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/apikey"
)

func testClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return New(Config{BaseURL: server.URL, Token: "internal-token", Timeout: time.Second}), server
}

func TestAuthorizeSendsTheContractAndParsesADecision(t *testing.T) {
	var gotPath, gotToken string
	var gotBody apikey.AuthorizeRequest

	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apikey.AuthorizeResponse{
			Allowed: true, OwnerID: "user-1", KeyID: "key-1", KeyName: "生产环境",
		})
	})

	resp, err := c.Authorize(context.Background(), "ox:sk:abc", "device-1")
	if err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if gotPath != apikey.PathAuthorize {
		t.Errorf("path = %q, want %q", gotPath, apikey.PathAuthorize)
	}
	if gotToken != "Bearer internal-token" {
		t.Errorf("Authorization = %q", gotToken)
	}
	if gotBody.Key != "ox:sk:abc" || gotBody.DeviceID != "device-1" {
		t.Errorf("request body = %+v", gotBody)
	}
	if !resp.Allowed || resp.OwnerID != "user-1" || resp.KeyName != "生产环境" {
		t.Errorf("response = %+v", resp)
	}
}

func TestAuthorizeRejectionsAreNotErrors(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apikey.AuthorizeResponse{Allowed: false, RejectReason: apikey.RejectScopeMissing})
	})

	resp, err := c.Authorize(context.Background(), "ox:sk:abc", "device-1")
	if err != nil {
		t.Fatalf("Authorize() error = %v, want nil for a plain rejection", err)
	}
	if resp.Allowed {
		t.Error("Allowed = true, want a rejection")
	}
	if resp.RejectReason != apikey.RejectScopeMissing {
		t.Errorf("RejectReason = %q", resp.RejectReason)
	}
}

func TestAuthorizeFailures(t *testing.T) {
	cases := []struct {
		name     string
		handler  http.HandlerFunc
		wantErr  error
		contains string
	}{
		{
			name: "unauthorized means a bad internal token",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErr: ErrUnauthorized,
		},
		{
			name: "server error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "boom", http.StatusInternalServerError)
			},
			contains: "unexpected status 500",
		},
		{
			name: "not json",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("nope"))
			},
			contains: "decode response",
		},
		{
			name: "200 without a decision",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{}`))
			},
			contains: "no decision",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := testClient(t, tc.handler)
			_, err := c.Authorize(context.Background(), "ox:sk:abc", "device-1")
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Authorize() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("Authorize() error = %v, want it to contain %q", err, tc.contains)
			}
		})
	}
}

func TestAuthorizeAgainstAnUnreachableManager(t *testing.T) {
	// 端口 1 上不会有服务：调用必须失败，而不是挂在那里。
	c := New(Config{BaseURL: "http://127.0.0.1:1", Token: "t", Timeout: 300 * time.Millisecond})
	if _, err := c.Authorize(context.Background(), "ox:sk:abc", "device-1"); err == nil {
		t.Fatal("Authorize() error = nil, want a transport error")
	}
}

func TestAuthorizeWithoutATokenStillCalls(t *testing.T) {
	// 没配 token 时不该静默跳过调用：请求照发，控制面会用 401 告诉我们配置错了。
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := New(Config{BaseURL: server.URL})
	if _, err := c.Authorize(context.Background(), "ox:sk:abc", "device-1"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Authorize() error = %v, want ErrUnauthorized", err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want no header when the token is empty", gotAuth)
	}
}
