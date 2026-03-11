package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liuscraft/orion-x/internal/manager/auth"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
)

type authTestRepo struct {
	usersByID    map[uuid.UUID]auth.User
	usersByEmail map[string]auth.User
}

func (r *authTestRepo) Create(_ context.Context, user auth.User) error {
	normalizedEmail := strings.ToLower(strings.TrimSpace(user.Email))
	if normalizedEmail == "" {
		return auth.ErrInvalidArgument
	}
	if _, ok := r.usersByEmail[normalizedEmail]; ok {
		return auth.ErrConflict
	}
	if r.usersByID == nil {
		r.usersByID = make(map[uuid.UUID]auth.User)
	}
	if r.usersByEmail == nil {
		r.usersByEmail = make(map[string]auth.User)
	}

	user.Email = normalizedEmail
	r.usersByID[user.ID] = user
	r.usersByEmail[normalizedEmail] = user
	return nil
}

func (r *authTestRepo) Count(_ context.Context) (int64, error) {
	return int64(len(r.usersByID)), nil
}

func (r *authTestRepo) GetByID(_ context.Context, id uuid.UUID) (auth.User, error) {
	user, ok := r.usersByID[id]
	if !ok {
		return auth.User{}, auth.ErrUserNotFound
	}
	return user, nil
}

func (r *authTestRepo) GetByEmail(_ context.Context, email string) (auth.User, error) {
	user, ok := r.usersByEmail[strings.ToLower(strings.TrimSpace(email))]
	if !ok {
		return auth.User{}, auth.ErrUserNotFound
	}
	return user, nil
}

func TestAuthHandler_LoginRefreshLogoutFlow(t *testing.T) {
	passwordHash, err := auth.HashPassword("P@ssw0rd")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	userID := uuid.New()
	user := auth.User{
		ID:           userID,
		Email:        "user@example.com",
		PasswordHash: passwordHash,
		Role:         contracts.RoleNormalUser,
		Status:       contracts.UserStatusActive,
	}

	repo := &authTestRepo{
		usersByID:    map[uuid.UUID]auth.User{userID: user},
		usersByEmail: map[string]auth.User{"user@example.com": user},
	}
	tokenManager, err := auth.NewJWTManager(auth.JWTManagerConfig{
		Secret:     "httpapi-test-secret",
		Issuer:     "httpapi-test",
		AccessTTL:  5 * time.Minute,
		RefreshTTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	service := auth.NewService(repo, tokenManager)
	handler := NewAuthHandler(service)
	middleware := NewAuthMiddleware(service)

	mux := http.NewServeMux()
	mux.Handle("/api/v1/auth/login", http.HandlerFunc(handler.Login))
	mux.Handle("/api/v1/auth/refresh", http.HandlerFunc(handler.Refresh))
	mux.Handle("/api/v1/auth/logout", middleware.RequireAuth(http.HandlerFunc(handler.Logout)))

	loginBody := []byte(`{"email":"user@example.com","password":"P@ssw0rd"}`)
	loginResp := performJSONRequest(t, mux, http.MethodPost, "/api/v1/auth/login", loginBody, "")
	if loginResp.Code != http.StatusOK {
		t.Fatalf("expected login status 200, got %d", loginResp.Code)
	}

	loginPayload := decodePayload(t, loginResp)
	if loginPayload["code"] != "OK" {
		t.Fatalf("expected login code OK, got %#v", loginPayload["code"])
	}
	data, ok := loginPayload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected login data object")
	}
	refreshToken, _ := data["refresh_token"].(string)
	if refreshToken == "" {
		t.Fatalf("expected non-empty refresh token")
	}

	refreshBody := []byte(fmt.Sprintf(`{"refresh_token":%q}`, refreshToken))
	refreshResp := performJSONRequest(t, mux, http.MethodPost, "/api/v1/auth/refresh", refreshBody, "")
	if refreshResp.Code != http.StatusOK {
		t.Fatalf("expected refresh status 200, got %d", refreshResp.Code)
	}

	refreshPayload := decodePayload(t, refreshResp)
	refreshData, ok := refreshPayload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected refresh data object")
	}
	accessToken, _ := refreshData["access_token"].(string)
	if accessToken == "" {
		t.Fatalf("expected non-empty access token")
	}

	logoutResp := performJSONRequest(t, mux, http.MethodPost, "/api/v1/auth/logout", []byte(`{}`), "Bearer "+accessToken)
	if logoutResp.Code != http.StatusOK {
		t.Fatalf("expected logout status 200, got %d", logoutResp.Code)
	}
	logoutPayload := decodePayload(t, logoutResp)
	if logoutPayload["code"] != "OK" {
		t.Fatalf("expected logout code OK, got %#v", logoutPayload["code"])
	}
}

func TestAuthHandler_LoginInvalidCredentials(t *testing.T) {
	passwordHash, err := auth.HashPassword("P@ssw0rd")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	userID := uuid.New()
	repo := &authTestRepo{
		usersByID: map[uuid.UUID]auth.User{
			userID: {
				ID:           userID,
				Email:        "user@example.com",
				PasswordHash: passwordHash,
				Role:         contracts.RoleNormalUser,
				Status:       contracts.UserStatusActive,
			},
		},
		usersByEmail: map[string]auth.User{},
	}
	repo.usersByEmail["user@example.com"] = repo.usersByID[userID]

	tokenManager, err := auth.NewJWTManager(auth.JWTManagerConfig{
		Secret:     "httpapi-test-secret",
		Issuer:     "httpapi-test",
		AccessTTL:  5 * time.Minute,
		RefreshTTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	handler := NewAuthHandler(auth.NewService(repo, tokenManager))

	mux := http.NewServeMux()
	mux.Handle("/api/v1/auth/login", http.HandlerFunc(handler.Login))

	resp := performJSONRequest(t, mux, http.MethodPost, "/api/v1/auth/login", []byte(`{"email":"user@example.com","password":"wrong"}`), "")
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", resp.Code)
	}
	payload := decodePayload(t, resp)
	if payload["code"] != "ERR_UNAUTHORIZED" {
		t.Fatalf("expected code ERR_UNAUTHORIZED, got %#v", payload["code"])
	}
}

func TestAuthHandler_RegisterSuccess(t *testing.T) {
	repo := &authTestRepo{
		usersByID:    make(map[uuid.UUID]auth.User),
		usersByEmail: make(map[string]auth.User),
	}
	tokenManager, err := auth.NewJWTManager(auth.JWTManagerConfig{
		Secret:     "httpapi-test-secret",
		Issuer:     "httpapi-test",
		AccessTTL:  5 * time.Minute,
		RefreshTTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	handler := NewAuthHandler(auth.NewService(repo, tokenManager))
	mux := http.NewServeMux()
	mux.Handle("/api/v1/auth/register", http.HandlerFunc(handler.Register))
	mux.Handle("/api/v1/auth/login", http.HandlerFunc(handler.Login))

	registerBody := []byte(`{"email":"NewUser@example.com","password":"P@ssw0rd"}`)
	registerResp := performJSONRequest(t, mux, http.MethodPost, "/api/v1/auth/register", registerBody, "")
	if registerResp.Code != http.StatusOK {
		t.Fatalf("expected register status 200, got %d", registerResp.Code)
	}

	registerPayload := decodePayload(t, registerResp)
	if registerPayload["code"] != "OK" {
		t.Fatalf("expected register code OK, got %#v", registerPayload["code"])
	}
	data, ok := registerPayload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected register data object")
	}
	userData, ok := data["user"].(map[string]any)
	if !ok {
		t.Fatalf("expected register user object")
	}
	if userData["email"] != "newuser@example.com" {
		t.Fatalf("expected normalized email, got %#v", userData["email"])
	}
	if userData["role"] != string(contracts.RoleAdmin) {
		t.Fatalf("expected role admin for first user, got %#v", userData["role"])
	}

	loginResp := performJSONRequest(t, mux, http.MethodPost, "/api/v1/auth/login", []byte(`{"email":"newuser@example.com","password":"P@ssw0rd"}`), "")
	if loginResp.Code != http.StatusOK {
		t.Fatalf("expected login status 200 after register, got %d", loginResp.Code)
	}
}

func TestAuthHandler_RegisterConflict(t *testing.T) {
	passwordHash, err := auth.HashPassword("P@ssw0rd")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	userID := uuid.New()
	user := auth.User{
		ID:           userID,
		Email:        "exists@example.com",
		PasswordHash: passwordHash,
		Role:         contracts.RoleNormalUser,
		Status:       contracts.UserStatusActive,
	}
	repo := &authTestRepo{
		usersByID:    map[uuid.UUID]auth.User{userID: user},
		usersByEmail: map[string]auth.User{"exists@example.com": user},
	}
	tokenManager, err := auth.NewJWTManager(auth.JWTManagerConfig{
		Secret:     "httpapi-test-secret",
		Issuer:     "httpapi-test",
		AccessTTL:  5 * time.Minute,
		RefreshTTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	handler := NewAuthHandler(auth.NewService(repo, tokenManager))
	mux := http.NewServeMux()
	mux.Handle("/api/v1/auth/register", http.HandlerFunc(handler.Register))

	resp := performJSONRequest(t, mux, http.MethodPost, "/api/v1/auth/register", []byte(`{"email":"EXISTS@example.com","password":"new-pass"}`), "")
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", resp.Code)
	}
	payload := decodePayload(t, resp)
	if payload["code"] != "ERR_CONFLICT" {
		t.Fatalf("expected code ERR_CONFLICT, got %#v", payload["code"])
	}
}

func TestAuthMiddleware_RequireRoleForbidden(t *testing.T) {
	passwordHash, err := auth.HashPassword("P@ssw0rd")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	userID := uuid.New()
	user := auth.User{
		ID:           userID,
		Email:        "normal@example.com",
		PasswordHash: passwordHash,
		Role:         contracts.RoleNormalUser,
		Status:       contracts.UserStatusActive,
	}
	repo := &authTestRepo{
		usersByID:    map[uuid.UUID]auth.User{userID: user},
		usersByEmail: map[string]auth.User{"normal@example.com": user},
	}
	tokenManager, err := auth.NewJWTManager(auth.JWTManagerConfig{
		Secret:     "httpapi-test-secret",
		Issuer:     "httpapi-test",
		AccessTTL:  5 * time.Minute,
		RefreshTTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	service := auth.NewService(repo, tokenManager)
	middleware := NewAuthMiddleware(service)

	loginResult, err := service.Login(context.Background(), "normal@example.com", "P@ssw0rd")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	mux := http.NewServeMux()
	protected := middleware.RequireAuth(middleware.RequireRole(contracts.RoleAdmin)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"code": "OK", "message": "", "data": map[string]any{"status": "ok"}})
	})))
	mux.Handle("/api/v1/admin/protected", protected)

	resp := performJSONRequest(t, mux, http.MethodGet, "/api/v1/admin/protected", nil, "Bearer "+loginResult.Tokens.AccessToken)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", resp.Code)
	}
	payload := decodePayload(t, resp)
	if payload["code"] != "ERR_FORBIDDEN" {
		t.Fatalf("expected code ERR_FORBIDDEN, got %#v", payload["code"])
	}
}

func TestAuthMiddleware_DisabledUserForbidden(t *testing.T) {
	userID := uuid.New()
	disabled := auth.User{
		ID:     userID,
		Email:  "disabled@example.com",
		Role:   contracts.RoleNormalUser,
		Status: contracts.UserStatusDisabled,
	}

	repo := &authTestRepo{
		usersByID:    map[uuid.UUID]auth.User{userID: disabled},
		usersByEmail: map[string]auth.User{"disabled@example.com": disabled},
	}
	tokenManager, err := auth.NewJWTManager(auth.JWTManagerConfig{
		Secret:     "httpapi-test-secret",
		Issuer:     "httpapi-test",
		AccessTTL:  5 * time.Minute,
		RefreshTTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	tokens, err := tokenManager.IssueTokenPair(disabled)
	if err != nil {
		t.Fatalf("IssueTokenPair() error = %v", err)
	}

	service := auth.NewService(repo, tokenManager)
	middleware := NewAuthMiddleware(service)

	mux := http.NewServeMux()
	mux.Handle("/api/v1/protected", middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"code": "OK", "message": "", "data": map[string]any{"status": "ok"}})
	})))

	resp := performJSONRequest(t, mux, http.MethodGet, "/api/v1/protected", nil, "Bearer "+tokens.AccessToken)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", resp.Code)
	}
	payload := decodePayload(t, resp)
	if payload["code"] != "ERR_FORBIDDEN" {
		t.Fatalf("expected code ERR_FORBIDDEN, got %#v", payload["code"])
	}
}

func TestAuthHandler_MethodNotAllowed(t *testing.T) {
	repo := &authTestRepo{
		usersByID:    map[uuid.UUID]auth.User{},
		usersByEmail: map[string]auth.User{},
	}
	tokenManager, err := auth.NewJWTManager(auth.JWTManagerConfig{
		Secret:     "httpapi-test-secret",
		Issuer:     "httpapi-test",
		AccessTTL:  5 * time.Minute,
		RefreshTTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTManager() error = %v", err)
	}

	handler := NewAuthHandler(auth.NewService(repo, tokenManager))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
	resp := httptest.NewRecorder()
	handler.Login(resp, req)
	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", resp.Code)
	}
}

func performJSONRequest(t *testing.T, handler http.Handler, method, path string, body []byte, authorization string) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

func decodePayload(t *testing.T, resp *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return payload
}
