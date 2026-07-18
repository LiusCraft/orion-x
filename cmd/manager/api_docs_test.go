package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAPIDocs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api-docs", apiDocs)

	request := httptest.NewRequest(http.MethodGet, "/api-docs", nil)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Fatalf("Content-Type = %q, want HTML", contentType)
	}
	if !strings.Contains(response.Body.String(), "id=\"api-reference\"") {
		t.Fatal("API docs page does not include Scalar's API reference script")
	}
	if !strings.Contains(response.Body.String(), "data-url=\"/swagger/doc.json\"") {
		t.Fatal("API docs page does not use the generated OpenAPI document")
	}
}
