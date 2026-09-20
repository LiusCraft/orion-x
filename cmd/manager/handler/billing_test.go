package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/billing/service"
	"github.com/liuscraft/orion-x/internal/store"
)

func TestParsePageParams(t *testing.T) {
	cases := []struct {
		name         string
		query        string
		wantPage     int
		wantPageSize int
	}{
		{name: "no params", query: "", wantPage: 1, wantPageSize: 20},
		{name: "explicit values", query: "?page=3&page_size=50", wantPage: 3, wantPageSize: 50},
		{name: "page zero falls back", query: "?page=0", wantPage: 1, wantPageSize: 20},
		{name: "negative page falls back", query: "?page=-2", wantPage: 1, wantPageSize: 20},
		{name: "non numeric page falls back", query: "?page=abc", wantPage: 1, wantPageSize: 20},
		{name: "page size above the cap falls back", query: "?page_size=101", wantPage: 1, wantPageSize: 20},
		{name: "page size at the cap", query: "?page_size=100", wantPage: 1, wantPageSize: 100},
		{name: "page size zero falls back", query: "?page_size=0", wantPage: 1, wantPageSize: 20},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestContext(http.MethodGet, "/api/billing/ledger"+tc.query)
			page, pageSize := parsePageParams(c)
			if page != tc.wantPage || pageSize != tc.wantPageSize {
				t.Fatalf("parsePageParams() = (%d, %d), want (%d, %d)", page, pageSize, tc.wantPage, tc.wantPageSize)
			}
		})
	}
}

func TestWriteBillingError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{
			name:       "batch too large is 413",
			err:        fmt.Errorf("wrapped: %w", service.ErrBatchTooLarge),
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "invalid request is 400",
			err:        service.ErrInvalidRequest,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing item is 404",
			err:        fmt.Errorf("%w: no price for item voice:clone", service.ErrItemNotFound),
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "missing store row is 404",
			err:        fmt.Errorf("billing store: get account: %w", store.ErrNotFound),
			wantStatus: http.StatusNotFound,
		},
		{
			// 409 而不是 402：余额不足是“当前状态与请求冲突”（见 writeBillingError 注释）。
			name:       "insufficient balance is 409",
			err:        fmt.Errorf("%w: event left unpaid", service.ErrInsufficientBalance),
			wantStatus: http.StatusConflict,
		},
		{
			name:       "anything else is 500",
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			writeBillingError(c, tc.err)
			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, tc.wantStatus)
			}
		})
	}
}

// 计费关闭（billing.enabled: false）时 service 是 nil：每个 routes 都该回 503，而不是
// 500 或者 panic。
func TestBillingHandlersAreUnavailableWhenDisabled(t *testing.T) {
	internal := NewInternalBillingHandler(nil)
	admin := NewBillingAdminHandler(nil)
	user := NewBillingUserHandler(nil)

	cases := []struct {
		name    string
		handler gin.HandlerFunc
		request string
	}{
		{name: "internal authorize", handler: internal.Authorize, request: "/internal/billing/authorize"},
		{name: "internal usage-events", handler: internal.UsageEvents, request: "/internal/billing/usage-events"},
		{name: "internal settle", handler: internal.Settle, request: "/internal/billing/settle"},
		{name: "admin items", handler: admin.Items, request: "/api/billing/items"},
		{name: "admin accounts", handler: admin.Accounts, request: "/api/billing/accounts"},
		{name: "admin ledger", handler: admin.Ledger, request: "/api/billing/ledger"},
		{name: "admin stats", handler: admin.Stats, request: "/api/billing/stats"},
		{name: "user summary", handler: user.Summary, request: "/api/billing/summary"},
		{name: "user usage", handler: user.Usage, request: "/api/billing/usage"},
		{name: "user usage by model", handler: user.ModelUsage, request: "/api/billing/usage-by-model"},
		{name: "user prices", handler: user.Prices, request: "/api/billing/prices"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = httptest.NewRequest(http.MethodPost, tc.request, nil)
			tc.handler(context)
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
			}
		})
	}
}

// 复刻音色的预冻结失败要映射成 402，价格没配要能看懂是什么没配。
func TestWriteVoiceCloneErrorMapsBillingErrors(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{
			name:       "insufficient balance is 402",
			err:        fmt.Errorf("voice clone reserve: %w", service.ErrInsufficientBalance),
			wantStatus: http.StatusPaymentRequired,
		},
		{
			name:       "missing price is 503",
			err:        fmt.Errorf("voice clone reserve: %w: no price for item voice:clone", service.ErrItemNotFound),
			wantStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			writeVoiceCloneError(c, tc.err)
			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", response.Code, tc.wantStatus, response.Body.String())
			}
		})
	}
}

func newTestContext(method, target string) *gin.Context {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(method, target, nil)
	return c
}
