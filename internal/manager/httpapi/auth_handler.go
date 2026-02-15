package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/liuscraft/orion-x/internal/manager/auth"
)

type AuthHandler struct {
	service *auth.Service
}

func NewAuthHandler(service *auth.Service) *AuthHandler {
	return &AuthHandler{service: service}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"code":    "ERR_INTERNAL",
			"message": "internal server error",
		})
		return
	}

	var req registerRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "invalid request body",
		})
		return
	}

	result, err := h.service.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		writeAuthServiceError(w, err)
		return
	}

	writeAuthSuccess(w, result)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"code":    "ERR_INTERNAL",
			"message": "internal server error",
		})
		return
	}

	var req loginRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "invalid request body",
		})
		return
	}

	result, err := h.service.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeAuthServiceError(w, err)
		return
	}

	writeAuthSuccess(w, result)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"code":    "ERR_INTERNAL",
			"message": "internal server error",
		})
		return
	}

	var req refreshRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "invalid request body",
		})
		return
	}

	result, err := h.service.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		writeAuthServiceError(w, err)
		return
	}

	writeAuthSuccess(w, result)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"code":    "ERR_UNAUTHORIZED",
			"message": "unauthorized",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    "OK",
		"message": "",
		"data": map[string]any{
			"user_id": principal.UserID.String(),
		},
	})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func decodeJSONBody(r *http.Request, dst any) error {
	if r.Body == nil {
		return io.EOF
	}

	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must only contain one json object")
		}
		return err
	}

	return nil
}

func writeAuthSuccess(w http.ResponseWriter, result auth.LoginResult) {
	writeJSON(w, http.StatusOK, map[string]any{
		"code":    "OK",
		"message": "",
		"data": map[string]any{
			"access_token":       result.Tokens.AccessToken,
			"refresh_token":      result.Tokens.RefreshToken,
			"token_type":         result.Tokens.TokenType,
			"expires_in":         result.Tokens.AccessExpiresIn,
			"refresh_expires_in": result.Tokens.RefreshExpiresIn,
			"user": map[string]any{
				"id":    result.User.UserID.String(),
				"email": result.User.Email,
				"role":  result.User.Role,
			},
		},
	})
}

func writeAuthServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidArgument):
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": err.Error(),
		})
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"code":    "ERR_UNAUTHORIZED",
			"message": "invalid email or password",
		})
	case errors.Is(err, auth.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]any{
			"code":    "ERR_CONFLICT",
			"message": "resource already exists",
		})
	case errors.Is(err, auth.ErrUnauthorized):
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"code":    "ERR_UNAUTHORIZED",
			"message": "unauthorized",
		})
	case errors.Is(err, auth.ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]any{
			"code":    "ERR_FORBIDDEN",
			"message": "permission denied",
		})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"code":    "ERR_INTERNAL",
			"message": "internal server error",
		})
	}
}

func extractBearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}

	const prefix = "bearer "
	if !strings.HasPrefix(strings.ToLower(header), prefix) {
		return ""
	}

	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return ""
	}
	return token
}
