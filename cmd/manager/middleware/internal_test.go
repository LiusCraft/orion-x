package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestInternalAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name       string
		token      string
		header     string
		wantStatus int
		wantNext   bool
	}{
		{
			name:       "valid bearer token passes",
			token:      "s3cret",
			header:     "Bearer s3cret",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "unconfigured token rejects a request carrying one",
			token:      "",
			header:     "Bearer s3cret",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "unconfigured token rejects an anonymous request",
			token:      "",
			header:     "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "blank configured token rejects",
			token:      "   ",
			header:     "Bearer    ",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong token",
			token:      "s3cret",
			header:     "Bearer nope",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing header",
			token:      "s3cret",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong scheme",
			token:      "s3cret",
			header:     "Token s3cret",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "prefix of the token is not enough",
			token:      "s3cret",
			header:     "Bearer s3cre",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nextCalled := false
			router := gin.New()
			router.POST("/internal/billing/authorize", InternalAuth(tc.token), func(c *gin.Context) {
				nextCalled = true
				c.Status(http.StatusOK)
			})

			request := httptest.NewRequest(http.MethodPost, "/internal/billing/authorize", nil)
			if tc.header != "" {
				request.Header.Set("Authorization", tc.header)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", response.Code, tc.wantStatus, response.Body.String())
			}
			if nextCalled != tc.wantNext {
				t.Fatalf("handler reached = %v, want %v", nextCalled, tc.wantNext)
			}
		})
	}
}
