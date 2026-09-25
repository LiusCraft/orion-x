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

	"github.com/liuscraft/orion-x/internal/billing"
)

// requestShape 是假控制面记下来的请求形状。
type requestShape struct {
	Method      string
	Path        string
	Auth        string
	ContentType string
	Body        []byte
	Calls       int
}

// captureServer 起一个把请求记下来的假控制面。respond 收到的是第几次调用（从 1 起），
// 返回状态码与响应体；为 nil 时返回 200 + "{}"。
func captureServer(t *testing.T, respond func(call int) (int, string)) (*httptest.Server, func() requestShape) {
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
		shape.Calls++
		call := shape.Calls
		mu.Unlock()

		status, payload := http.StatusOK, "{}"
		if respond != nil {
			status, payload = respond(call)
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

// TestClientCallsControlPlaneEndpoints 钉住三个接口的路径、头部与请求体编码。
func TestClientCallsControlPlaneEndpoints(t *testing.T) {
	occurredAt := time.Date(2026, 9, 20, 10, 12, 33, 123_000_000, time.UTC)

	tests := []struct {
		name     string
		wantPath string
		wantBody string
		response string
		call     func(ctx context.Context, c *Client) error
	}{
		{
			name:     "authorize",
			wantPath: billing.PathAuthorize,
			wantBody: `{"device_id":"dev-1","session_id":"sess-1","channel":"xiaozhi"}`,
			response: `{"allowed":true,"account_id":"acct-1","reservation_id":"res-1","reserved_micro":500000,` +
				`"max_session_seconds":600,"balance_micro":9000000,` +
				`"price_snapshot":[{"item_code":"tts:characters","billable":true,"unit_price_micro":2,"unit_size":1},` +
				`{"item_code":"asr:audio:seconds","billable":false,"reason":"byok"}]}`,
			call: func(ctx context.Context, c *Client) error {
				resp, err := c.Authorize(ctx, billing.AuthorizeRequest{DeviceID: "dev-1", SessionID: "sess-1", Channel: "xiaozhi"})
				if err != nil {
					return err
				}
				if !resp.Allowed || resp.AccountID != "acct-1" || resp.ReservationID != "res-1" {
					return fmt.Errorf("decision = %+v, want allowed reservation", resp)
				}
				if resp.ReservedMicro != 500000 || resp.MaxSessionSeconds != 600 || resp.BalanceMicro != 9000000 {
					return fmt.Errorf("limits = %+v, want reserved/seconds/balance decoded", resp)
				}
				snap := resp.SnapshotMap()
				if len(snap) != 2 {
					return fmt.Errorf("snapshot = %v, want 2 entries", snap)
				}
				if s := snap[billing.ItemTTSCharacters]; !s.Billable || s.UnitPriceMicro != 2 || s.UnitSize != 1 {
					return fmt.Errorf("tts snapshot = %+v, want billable 2/1", s)
				}
				if s := snap[billing.ItemASRAudioSecond]; s.Billable || s.Reason != "byok" {
					return fmt.Errorf("asr snapshot = %+v, want non-billable byok", s)
				}
				return nil
			},
		},
		{
			name:     "usage events",
			wantPath: billing.PathUsageEvents,
			wantBody: `{"session_id":"sess-1","events":[{"event_id":"ev-1","item_code":"llm:tokens:input",` +
				`"quantity":1234,"turn_index":3,"occurred_at":"2026-09-20T10:12:33.123Z",` +
				`"dimensions":{"source":"main","step":2}}]}`,
			response: `{"accepted":11,"duplicated":1,"rejected":[{"event_id":"ev-2","reason":"unknown_session"}]}`,
			call: func(ctx context.Context, c *Client) error {
				resp, err := c.ReportUsage(ctx, billing.UsageBatchRequest{
					SessionID: "sess-1",
					Events: []billing.UsageEventReport{{
						EventID:    "ev-1",
						ItemCode:   billing.ItemLLMInput,
						Quantity:   1234,
						TurnIndex:  3,
						OccurredAt: occurredAt,
						Dimensions: map[string]any{"step": 2, "source": "main"},
					}},
				})
				if err != nil {
					return err
				}
				if resp.Accepted != 11 || resp.Duplicated != 1 || len(resp.Rejected) != 1 {
					return fmt.Errorf("usage response = %+v, want accepted/duplicated/rejected decoded", resp)
				}
				if resp.Rejected[0].Reason != "unknown_session" {
					return fmt.Errorf("reject reason = %q, want unknown_session", resp.Rejected[0].Reason)
				}
				return nil
			},
		},
		{
			name:     "settle",
			wantPath: billing.PathSettle,
			wantBody: `{"session_id":"sess-1","reason":"client_close","ended_at":"2026-09-20T10:20:11Z"}`,
			response: `{"charged_micro":12345,"released_micro":487655,"balance_micro":7987655,"settled":true}`,
			call: func(ctx context.Context, c *Client) error {
				resp, err := c.Settle(ctx, billing.SettleRequest{
					SessionID: "sess-1",
					Reason:    "client_close",
					EndedAt:   time.Date(2026, 9, 20, 10, 20, 11, 0, time.UTC),
				})
				if err != nil {
					return err
				}
				if !resp.Settled || resp.ChargedMicro != 12345 || resp.ReleasedMicro != 487655 || resp.BalanceMicro != 7987655 {
					return fmt.Errorf("settle response = %+v, want amounts decoded", resp)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, shape := captureServer(t, func(int) (int, string) { return http.StatusOK, tt.response })
			c := New(Config{BaseURL: srv.URL, Token: "secret-token"})

			if err := tt.call(context.Background(), c); err != nil {
				t.Fatalf("call: %v", err)
			}

			got := shape()
			if got.Method != http.MethodPost {
				t.Errorf("method = %q, want POST", got.Method)
			}
			if got.Path != tt.wantPath {
				t.Errorf("path = %q, want %q", got.Path, tt.wantPath)
			}
			if got.Auth != "Bearer secret-token" {
				t.Errorf("authorization = %q, want Bearer secret-token", got.Auth)
			}
			if got.ContentType != "application/json" {
				t.Errorf("content-type = %q, want application/json", got.ContentType)
			}
			assertJSONEqual(t, got.Body, tt.wantBody)
		})
	}
}

// TestClientAuthorizeBusinessRejection 钉住“200 + reject_reason 不是错误”这条约定（§14.3），
// 四个 reject_reason 都走一遍。
func TestClientAuthorizeBusinessRejection(t *testing.T) {
	reasons := []billing.RejectReason{
		billing.RejectInsufficientBalance,
		billing.RejectAccountSuspended,
		billing.RejectNoAccount,
		billing.RejectPriceMissing,
	}
	for _, reason := range reasons {
		t.Run(string(reason), func(t *testing.T) {
			srv, _ := captureServer(t, func(int) (int, string) {
				return http.StatusOK, fmt.Sprintf(
					`{"allowed":false,"account_id":"acct-123","reject_reason":%q,"balance_micro":12000,"required_micro":500000}`, reason)
			})
			c := New(Config{BaseURL: srv.URL, Token: "t"})

			resp, err := c.Authorize(context.Background(), billing.AuthorizeRequest{DeviceID: "d", SessionID: "s"})
			if err != nil {
				t.Fatalf("rejected authorize must not be an error, got: %v", err)
			}
			if resp.Allowed {
				t.Errorf("allowed = true, want false")
			}
			if resp.RejectReason != reason {
				t.Errorf("reject_reason = %q, want %q", resp.RejectReason, reason)
			}
			if resp.BalanceMicro != 12000 || resp.RequiredMicro != 500000 {
				t.Errorf("balance/required = %d/%d, want 12000/500000", resp.BalanceMicro, resp.RequiredMicro)
			}
		})
	}
}

// TestClientUnauthorizedIsDistinguishable 401 要能和别的失败分开:调用方靠它判断 token 配错。
func TestClientUnauthorizedIsDistinguishable(t *testing.T) {
	srv, _ := captureServer(t, func(int) (int, string) {
		return http.StatusUnauthorized, `{"error":"invalid internal token"}`
	})
	c := New(Config{BaseURL: srv.URL, Token: "bad-token"})

	_, err := c.ReportUsage(context.Background(), billing.UsageBatchRequest{SessionID: "s"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want errors.Is(err, ErrUnauthorized)", err)
	}
	for _, want := range []string{"401", billing.PathUsageEvents, "invalid internal token"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it to contain %q", err, want)
		}
	}
}

// TestClientStatusErrors 非 2xx 一律报错，错误里带状态码；只有 401 是 ErrUnauthorized。
func TestClientStatusErrors(t *testing.T) {
	tests := []struct {
		name             string
		status           int
		body             string
		wantUnauthorized bool
	}{
		{name: "bad request", status: http.StatusBadRequest, body: `{"error":"malformed json"}`},
		{name: "forbidden", status: http.StatusForbidden, body: `{"error":"forbidden"}`},
		{name: "payload too large", status: http.StatusRequestEntityTooLarge, body: `{"error":"batch too large"}`},
		{name: "internal", status: http.StatusInternalServerError, body: `{"error":"boom"}`},
		{name: "unavailable", status: http.StatusServiceUnavailable, body: ""},
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"error":"nope"}`, wantUnauthorized: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := captureServer(t, func(int) (int, string) { return tt.status, tt.body })
			c := New(Config{BaseURL: srv.URL, Token: "t"})

			_, err := c.Settle(context.Background(), billing.SettleRequest{SessionID: "s"})
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

// TestClientErrorSnippetIsBounded 错误信息里只保留有界的响应体,别把一整页 HTML 灌进日志。
func TestClientErrorSnippetIsBounded(t *testing.T) {
	srv, _ := captureServer(t, func(int) (int, string) {
		return http.StatusInternalServerError, strings.Repeat("x", errorBodySnippetBytes*8)
	})
	c := New(Config{BaseURL: srv.URL, Token: "t"})

	_, err := c.Settle(context.Background(), billing.SettleRequest{SessionID: "s"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if got := strings.Count(err.Error(), "x"); got != errorBodySnippetBytes {
		t.Fatalf("snippet length = %d, want %d", got, errorBodySnippetBytes)
	}
}

// TestClientDecodeError 2xx 但 body 不是 JSON 时要报错,不能静默返回零值。
func TestClientDecodeError(t *testing.T) {
	srv, _ := captureServer(t, func(int) (int, string) {
		return http.StatusOK, `<html>not json</html>`
	})
	c := New(Config{BaseURL: srv.URL, Token: "t"})

	_, err := c.Authorize(context.Background(), billing.AuthorizeRequest{DeviceID: "d", SessionID: "s"})
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

	_, err := c.ReportUsage(context.Background(), billing.UsageBatchRequest{SessionID: "s"})
	if err == nil {
		t.Fatal("want transport error, got nil")
	}
	if errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want a transport error, not ErrUnauthorized", err)
	}
	if !strings.Contains(err.Error(), billing.PathUsageEvents) {
		t.Errorf("err = %q, want it to name the endpoint", err)
	}
}

// TestClientOmitsAuthorizationWithoutToken token 为空时不带头(控制面会拒,这里只管别发假头)。
func TestClientOmitsAuthorizationWithoutToken(t *testing.T) {
	srv, shape := captureServer(t, nil)
	c := New(Config{BaseURL: srv.URL})

	if _, err := c.Authorize(context.Background(), billing.AuthorizeRequest{DeviceID: "d", SessionID: "s"}); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if got := shape().Auth; got != "" {
		t.Fatalf("authorization = %q, want it omitted", got)
	}
}

// TestClientTimeoutIsEnforced 超时必须真的生效:authorize 卡在设备等 hello 的关键路径上。
func TestClientTimeoutIsEnforced(t *testing.T) {
	tests := []struct {
		name  string
		cfg   func(baseURL string) Config
		call  func(ctx context.Context, c *Client) error
		limit time.Duration
	}{
		{
			name: "authorize",
			cfg: func(baseURL string) Config {
				return Config{BaseURL: baseURL, Token: "t", AuthorizeTimeout: 20 * time.Millisecond}
			},
			call: func(ctx context.Context, c *Client) error {
				_, err := c.Authorize(ctx, billing.AuthorizeRequest{DeviceID: "d", SessionID: "s"})
				return err
			},
			limit: 200 * time.Millisecond,
		},
		{
			name: "usage report",
			cfg: func(baseURL string) Config {
				return Config{BaseURL: baseURL, Token: "t", Timeout: 20 * time.Millisecond}
			},
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ReportUsage(ctx, billing.UsageBatchRequest{SessionID: "s"})
				return err
			},
			limit: 200 * time.Millisecond,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := captureServer(t, func(int) (int, string) {
				time.Sleep(400 * time.Millisecond)
				return http.StatusOK, "{}"
			})
			c := New(tt.cfg(srv.URL))

			start := time.Now()
			err := tt.call(context.Background(), c)
			elapsed := time.Since(start)

			if err == nil {
				t.Fatal("want timeout error, got nil")
			}
			if elapsed > tt.limit {
				t.Fatalf("call took %s, want it bounded by %s", elapsed, tt.limit)
			}
		})
	}
}

// TestClientHonorsContextCancellation 调用方 ctx 取消要能立刻停下,不受单请求超时保护。
func TestClientHonorsContextCancellation(t *testing.T) {
	srv, _ := captureServer(t, func(int) (int, string) {
		time.Sleep(400 * time.Millisecond)
		return http.StatusOK, "{}"
	})
	c := New(Config{BaseURL: srv.URL, Token: "t"})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	defer cancel()

	start := time.Now()
	if _, err := c.ReportUsage(ctx, billing.UsageBatchRequest{SessionID: "s"}); err == nil {
		t.Fatal("want error, got nil")
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("call took %s, want it to stop on ctx cancellation", elapsed)
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
