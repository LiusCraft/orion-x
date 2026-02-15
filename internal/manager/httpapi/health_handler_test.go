package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakePinger struct {
	err error
}

func (f fakePinger) Ping(context.Context) error {
	return f.err
}

func TestHealthHandler_OK(t *testing.T) {
	h := NewHealthHandler(fakePinger{}, 100*time.Millisecond)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["code"] != "OK" {
		t.Fatalf("expected code OK, got %#v", payload["code"])
	}
}

func TestHealthHandler_DBError(t *testing.T) {
	h := NewHealthHandler(fakePinger{err: errors.New("db down")}, 100*time.Millisecond)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Code)
	}
}

func TestHealthHandler_MethodNotAllowed(t *testing.T) {
	h := NewHealthHandler(fakePinger{}, 100*time.Millisecond)
	req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.Code)
	}
}
