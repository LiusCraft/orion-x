package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	res := httptest.NewRecorder()
	healthHandler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want %d", res.Code, http.StatusOK)
	}
}
