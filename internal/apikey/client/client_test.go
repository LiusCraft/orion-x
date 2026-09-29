package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/apikey"
)

// requestShape 是假控制面记下来的请求形状。
type requestShape struct {
	Method      string
	Path        string
	Auth        string
	ContentType string
	Body        []byte
}

// captureServer 起一个把请求记下来的假控制面。respond 返回状态码与响应体；
// 为 nil 时返回 200 + "{}"。
func captureServer(t *testing.T, respond func() (int, string)) (*httptest.Server, func() requestShape) {
	t.Helper()

	var mu sync.Mutex
	var shape requestShape
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		mu.Lock()
		shape.Method = r.Method
		shape.Path = r.URL.Path
		shape.Auth = r.Header.Get("Authorization")
		shape.ContentType = r.Header.Get("Content-Type")
		shape.Body = body
		mu.Unlock()

		status, payload := http.StatusOK, "{}"
		if respond != nil {
			status, payload = respond()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, payload)
	}))
	t.Cleanup(srv.Close)

	return srv, func() requestShape {
		mu.Lock()
		defer mu.Unlock()
		return shape
	}
}

// TestClientVerifyCallsControlPlane 钉住路径、头部与请求体编码，以及 allowed 响应的解码。
func TestClientVerifyCallsControlPlane(t *testing.T) {
	srv, shape := captureServer(t, func() (int, string) {
		return http.StatusOK, `{"allowed":true,"key_id":"key-1","user_id":"user-1","scopes":["device:connect","agent:read"]}`
	})
	c := New(Config{BaseURL: srv.URL, Token: "secret-token"})

	resp, err := c.Verify(context.Background(), "ox_sk_key", "dev-1")
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !resp.Allowed || resp.KeyID != "key-1" || resp.UserID != "user-1" {
		t.Fatalf("resp = %+v, want allowed identity", resp)
	}
	if strings.Join(resp.Scopes, ",") != "device:connect,agent:read" {
		t.Fatalf("scopes = %v", resp.Scopes)
	}

	got := shape()
	if got.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.Method)
	}
	if got.Path != apikey.PathVerify {
		t.Errorf("path = %q, want %q", got.Path, apikey.PathVerify)
	}
	if got.Auth != "Bearer secret-token" {
		t.Errorf("authorization = %q, want Bearer secret-token", got.Auth)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", got.ContentType)
	}
	assertJSONEqual(t, got.Body, `{"key":"ox_sk_key","device_id":"dev-1"}`)
}

// TestClientVerifyBusinessRejection 钉住“200 + allowed=false 不是错误”这条约定：
// 拒绝原因是业务结论，调用方按它决定关帧 reason；未知 reason 原样透传。
func TestClientVerifyBusinessRejection(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		reason apikey.RejectReason
	}{
		{
			name:   "insufficient scope",
			body:   `{"allowed":false,"reject_reason":"insufficient_scope","required":["device:connect"],"granted":["agent:read"]}`,
			reason: apikey.RejectInsufficientScope,
		},
		{name: "invalid key", body: `{"allowed":false,"reject_reason":"invalid_key"}`, reason: apikey.RejectInvalidKey},
		// 字段只增：将来 manager 加了新 reason，旧数据面不认识也要原样透传。
		{name: "unknown reason", body: `{"allowed":false,"reject_reason":"brand_new_reason"}`, reason: "brand_new_reason"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := captureServer(t, func() (int, string) { return http.StatusOK, tt.body })
			c := New(Config{BaseURL: srv.URL, Token: "t"})

			resp, err := c.Verify(context.Background(), "k", "d")
			if err != nil {
				t.Fatalf("business rejection must not be an error, got: %v", err)
			}
			if resp.Allowed {
				t.Fatalf("allowed = true, want false")
			}
			if resp.RejectReason != tt.reason {
				t.Fatalf("reject_reason = %q, want %q", resp.RejectReason, tt.reason)
			}
			if tt.reason == apikey.RejectInsufficientScope {
				if strings.Join(resp.Required, ",") != apikey.ScopeDeviceConnect || strings.Join(resp.Granted, ",") != apikey.ScopeAgentRead {
					t.Fatalf("required/granted = %v/%v, want scope detail decoded", resp.Required, resp.Granted)
				}
			}
		})
	}
}

// TestClientUnauthorizedIsDistinguishable 401 要能和别的失败分开:调用方靠它判断
// manager.token 配错（重试没有意义）。
func TestClientUnauthorizedIsDistinguishable(t *testing.T) {
	srv, _ := captureServer(t, func() (int, string) {
		return http.StatusUnauthorized, `{"error":"invalid internal token"}`
	})
	c := New(Config{BaseURL: srv.URL, Token: "bad-token"})

	_, err := c.Verify(context.Background(), "k", "d")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want errors.Is(err, ErrUnauthorized)", err)
	}
	for _, want := range []string{"401", apikey.PathVerify, "invalid internal token"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it to contain %q", err, want)
		}
	}
}

// TestClientStatusErrors 非 2xx 一律报错，错误里带状态码；只有 401 是 ErrUnauthorized。
// 404 是“manager 没开 apikey”的信号，数据面据此记 auth_unavailable（FR-7）。
func TestClientStatusErrors(t *testing.T) {
	tests := []struct {
		name             string
		status           int
		body             string
		wantUnauthorized bool
	}{
		// 404 是“manager 没开 apikey”的信号（FR-7），401 是内部 token 配错的信号。
		{name: "not found", status: http.StatusNotFound, body: `{"error":"api keys are disabled"}`},
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"error":"nope"}`, wantUnauthorized: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := captureServer(t, func() (int, string) { return tt.status, tt.body })
			c := New(Config{BaseURL: srv.URL, Token: "t"})

			_, err := c.Verify(context.Background(), "k", "d")
			if err == nil {
				t.Fatalf("status %d: want error, got nil", tt.status)
			}
			if errors.Is(err, ErrUnauthorized) != tt.wantUnauthorized {
				t.Errorf("errors.Is(err, ErrUnauthorized) = %v, want %v", !tt.wantUnauthorized, tt.wantUnauthorized)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("status %d", tt.status)) {
				t.Errorf("err = %q, want it to contain status %d", err, tt.status)
			}
		})
	}
}

// TestClientDecodeError 2xx 但 body 不是 JSON 时要报错,不能静默返回零值（零值意味着
// allowed=false，会被当成业务拒绝——那是把故障伪装成结论）。
func TestClientDecodeError(t *testing.T) {
	srv, _ := captureServer(t, func() (int, string) {
		return http.StatusOK, `<html>not json</html>`
	})
	c := New(Config{BaseURL: srv.URL, Token: "t"})

	_, err := c.Verify(context.Background(), "k", "d")
	if err == nil {
		t.Fatal("want decode error, got nil")
	}
	if errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want a decode error, not ErrUnauthorized", err)
	}
	if !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err = %q, want it to mention decode response", err)
	}
}

// TestClientTransportError 网络不通是普通错误,不是 ErrUnauthorized。
func TestClientTransportError(t *testing.T) {
	srv, _ := captureServer(t, nil)
	c := New(Config{BaseURL: srv.URL, Token: "t"})
	srv.Close()

	_, err := c.Verify(context.Background(), "k", "d")
	if err == nil {
		t.Fatal("want transport error, got nil")
	}
	if errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want a transport error, not ErrUnauthorized", err)
	}
	if !strings.Contains(err.Error(), apikey.PathVerify) {
		t.Errorf("err = %q, want it to name the endpoint", err)
	}
}

// TestClientOmitsAuthorizationWithoutToken token 为空时不带头（控制面会拒,这里只管别发假头）。
func TestClientOmitsAuthorizationWithoutToken(t *testing.T) {
	srv, shape := captureServer(t, nil)
	c := New(Config{BaseURL: srv.URL})

	if _, err := c.Verify(context.Background(), "k", "d"); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if got := shape().Auth; got != "" {
		t.Fatalf("authorization = %q, want it omitted", got)
	}
}

// TestClientDefaultTimeout verify 的默认超时是 800ms：它卡在设备等 hello 的关键路径上。
func TestClientDefaultTimeout(t *testing.T) {
	if c := New(Config{BaseURL: "http://127.0.0.1:1"}); c.timeout != 800*time.Millisecond {
		t.Fatalf("default timeout = %s, want 800ms", c.timeout)
	}
	if c := New(Config{BaseURL: "http://127.0.0.1:1", Timeout: 250 * time.Millisecond}); c.timeout != 250*time.Millisecond {
		t.Fatalf("configured timeout = %s, want 250ms", c.timeout)
	}
}

// TestClientTimeoutIsEnforced 超时必须真的生效：卡住的 manager 不能让握手永远等下去。
func TestClientTimeoutIsEnforced(t *testing.T) {
	srv, _ := captureServer(t, func() (int, string) {
		time.Sleep(400 * time.Millisecond)
		return http.StatusOK, "{}"
	})
	c := New(Config{BaseURL: srv.URL, Token: "t", Timeout: 20 * time.Millisecond})

	start := time.Now()
	_, err := c.Verify(context.Background(), "k", "d")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("want timeout error, got nil")
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("call took %s, want it bounded by the 20ms timeout", elapsed)
	}
}

// assertJSONEqual 比较两段 JSON 的语义,不看字段顺序。
func assertJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()
	var gotAny, wantAny any
	if err := json.Unmarshal(got, &gotAny); err != nil {
		t.Fatalf("request body is not JSON (%v): %s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &wantAny); err != nil {
		t.Fatalf("want body is not JSON: %v", err)
	}
	if !reflect.DeepEqual(gotAny, wantAny) {
		t.Fatalf("request body = %s, want %s", got, want)
	}
}
