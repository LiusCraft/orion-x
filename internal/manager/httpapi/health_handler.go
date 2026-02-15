package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	checker Pinger
	timeout time.Duration
}

func NewHealthHandler(checker Pinger, timeout time.Duration) *HealthHandler {
	if timeout <= 0 {
		timeout = time.Second
	}
	return &HealthHandler{checker: checker, timeout: timeout}
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	if err := h.checker.Ping(ctx); err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		writeJSON(w, status, map[string]any{
			"code":    "ERR_DB_UNAVAILABLE",
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    "OK",
		"message": "",
		"data": map[string]string{
			"status": "up",
		},
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
