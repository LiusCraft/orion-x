package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/liuscraft/orion-x/internal/manager/auth"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/platformresource"
)

type handlerPlatformResourceRepo struct {
	resources   map[uuid.UUID]platformresource.Resource
	resourceIDs map[string]uuid.UUID
	versions    map[uuid.UUID]int
}

func newHandlerPlatformResourceRepo() *handlerPlatformResourceRepo {
	return &handlerPlatformResourceRepo{
		resources:   make(map[uuid.UUID]platformresource.Resource),
		resourceIDs: make(map[string]uuid.UUID),
		versions:    make(map[uuid.UUID]int),
	}
}

func (r *handlerPlatformResourceRepo) Create(_ context.Context, resource platformresource.Resource) (platformresource.Resource, error) {
	if _, exists := r.resourceIDs[resource.ResourceKey]; exists {
		return platformresource.Resource{}, platformresource.ErrConflict
	}
	now := time.Now().UTC()
	resource.CreatedAt = now
	resource.UpdatedAt = now
	r.resources[resource.ID] = resource
	r.resourceIDs[resource.ResourceKey] = resource.ID
	r.versions[resource.ID] = 1
	return resource, nil
}

func (r *handlerPlatformResourceRepo) GetByID(_ context.Context, id uuid.UUID) (platformresource.Resource, error) {
	resource, exists := r.resources[id]
	if !exists {
		return platformresource.Resource{}, platformresource.ErrNotFound
	}
	return resource, nil
}

func (r *handlerPlatformResourceRepo) List(_ context.Context, filter platformresource.ListFilter) ([]platformresource.Resource, error) {
	items := make([]platformresource.Resource, 0)
	for _, item := range r.resources {
		if filter.Category != nil && item.Category != *filter.Category {
			continue
		}
		if filter.Provider != "" && item.Provider != filter.Provider {
			continue
		}
		if filter.Status != nil && item.Status != *filter.Status {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *handlerPlatformResourceRepo) Update(_ context.Context, id uuid.UUID, patch platformresource.UpdatePatch) (platformresource.Resource, error) {
	resource, exists := r.resources[id]
	if !exists {
		return platformresource.Resource{}, platformresource.ErrNotFound
	}
	if !patch.HasChanges() {
		return platformresource.Resource{}, platformresource.ErrInvalidArgument
	}

	if patch.ResourceKey != nil {
		if otherID, exists := r.resourceIDs[*patch.ResourceKey]; exists && otherID != id {
			return platformresource.Resource{}, platformresource.ErrConflict
		}
		delete(r.resourceIDs, resource.ResourceKey)
		resource.ResourceKey = *patch.ResourceKey
		r.resourceIDs[resource.ResourceKey] = id
	}
	if patch.Category != nil {
		resource.Category = *patch.Category
	}
	if patch.Provider != nil {
		resource.Provider = *patch.Provider
	}
	if patch.Name != nil {
		resource.Name = *patch.Name
	}
	if patch.SchemaVersion != nil {
		resource.SchemaVersion = *patch.SchemaVersion
	}
	if patch.BaseURL != nil {
		resource.BaseURL = *patch.BaseURL
	}
	if patch.AccessKey != nil {
		resource.AccessKey = *patch.AccessKey
	}
	if patch.Capabilities != nil {
		resource.Capabilities = copyRawMessageForHandler(*patch.Capabilities)
	}
	if patch.Config != nil {
		resource.Config = copyRawMessageForHandler(*patch.Config)
	}
	if patch.Status != nil {
		resource.Status = *patch.Status
	}
	resource.UpdatedAt = time.Now().UTC()
	r.resources[id] = resource
	r.versions[id]++
	return resource, nil
}

func (r *handlerPlatformResourceRepo) Delete(_ context.Context, id uuid.UUID) error {
	resource, exists := r.resources[id]
	if !exists {
		return platformresource.ErrNotFound
	}
	delete(r.resources, id)
	delete(r.resourceIDs, resource.ResourceKey)
	delete(r.versions, id)
	return nil
}

type handlerFakeAccessKeyCipher struct{}

func (handlerFakeAccessKeyCipher) Encrypt(plaintext string) (string, error) {
	return "enc:" + plaintext, nil
}

func (handlerFakeAccessKeyCipher) Decrypt(ciphertext string) (string, error) {
	if !strings.HasPrefix(ciphertext, "enc:") {
		return "", errors.New("invalid ciphertext")
	}
	return strings.TrimPrefix(ciphertext, "enc:"), nil
}

type handlerAuthUserRepo struct {
	byID    map[uuid.UUID]auth.User
	byEmail map[string]auth.User
}

func (r *handlerAuthUserRepo) Create(_ context.Context, user auth.User) error {
	if r.byID == nil {
		r.byID = make(map[uuid.UUID]auth.User)
	}
	if r.byEmail == nil {
		r.byEmail = make(map[string]auth.User)
	}
	r.byID[user.ID] = user
	r.byEmail[user.Email] = user
	return nil
}

func (r *handlerAuthUserRepo) GetByID(_ context.Context, id uuid.UUID) (auth.User, error) {
	user, ok := r.byID[id]
	if !ok {
		return auth.User{}, auth.ErrUserNotFound
	}
	return user, nil
}

func (r *handlerAuthUserRepo) GetByEmail(_ context.Context, email string) (auth.User, error) {
	user, ok := r.byEmail[email]
	if !ok {
		return auth.User{}, auth.ErrUserNotFound
	}
	return user, nil
}

type noopTokenManager struct{}

func (noopTokenManager) IssueTokenPair(user auth.User) (auth.TokenPair, error) {
	return auth.TokenPair{TokenType: "Bearer"}, nil
}

func (noopTokenManager) Parse(token string, expectedType auth.TokenType) (auth.TokenClaims, error) {
	return auth.TokenClaims{}, auth.ErrUnauthorized
}

func TestPlatformResourceHandler_CRUDAndRevealFlow(t *testing.T) {
	repo, handler, principal := newPlatformResourceHandlerTestHarness(t)

	mux := http.NewServeMux()
	mux.Handle("/api/v1/admin/platform-resources", http.HandlerFunc(handler.Create))
	mux.Handle("/api/v1/admin/platform-resources/", http.HandlerFunc(handler.ByID))
	mux.Handle("/api/v1/platform-resources", http.HandlerFunc(handler.List))

	createResp := performJSONRequestWithPrincipal(t, mux, http.MethodPost, "/api/v1/admin/platform-resources", []byte(`{
		"category":"llm",
		"provider":"zhipu",
		"resource_key":"llm-zhipu-prod",
		"name":"LLM Zhipu Prod",
		"schema_version":1,
		"base_url":"https://open.bigmodel.cn/api/v4",
		"access_key":"sk-zhipu-plain",
		"capabilities":{"stream":true},
		"config":{"model":"glm-4-flash"}
	}`), principal)
	if createResp.Code != http.StatusOK {
		t.Fatalf("expected create status 200, got %d", createResp.Code)
	}

	createPayload := decodePayload(t, createResp)
	createData, ok := createPayload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected create data object")
	}
	resourceID, _ := createData["id"].(string)
	if resourceID == "" {
		t.Fatalf("expected non-empty resource id")
	}
	if _, exists := createData["access_key"]; exists {
		t.Fatalf("expected create response not to return access_key")
	}
	if createData["has_access_key"] != true {
		t.Fatalf("expected has_access_key=true, got %#v", createData["has_access_key"])
	}

	listResp := performJSONRequestWithPrincipal(t, mux, http.MethodGet, "/api/v1/platform-resources?category=llm&provider=zhipu&status=active", nil, principal)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d", listResp.Code)
	}
	listPayload := decodePayload(t, listResp)
	listData, ok := listPayload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected list data object")
	}
	items, ok := listData["items"].([]any)
	if !ok {
		t.Fatalf("expected items array")
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 list item, got %d", len(items))
	}

	patchResp := performJSONRequestWithPrincipal(t, mux, http.MethodPatch, "/api/v1/admin/platform-resources/"+resourceID, []byte(`{
		"schema_version":2,
		"base_url":"https://open.bigmodel.cn/api/v5",
		"access_key":"sk-zhipu-rotated",
		"config":{"model":"glm-4-air"},
		"status":"inactive"
	}`), principal)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("expected patch status 200, got %d", patchResp.Code)
	}
	patchPayload := decodePayload(t, patchResp)
	patchData, ok := patchPayload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected patch data object")
	}
	if patchData["schema_version"] != float64(2) {
		t.Fatalf("expected schema_version 2, got %#v", patchData["schema_version"])
	}
	if patchData["status"] != string(contracts.ResourceStatusInactive) {
		t.Fatalf("expected status inactive, got %#v", patchData["status"])
	}
	if _, exists := patchData["access_key"]; exists {
		t.Fatalf("expected patch response not to return access_key")
	}

	revealBadResp := performJSONRequestWithPrincipal(t, mux, http.MethodPost, "/api/v1/admin/platform-resources/"+resourceID+"/access-key/reveal", []byte(`{"password":"wrong"}`), principal)
	if revealBadResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected reveal with wrong password status 401, got %d", revealBadResp.Code)
	}

	revealResp := performJSONRequestWithPrincipal(t, mux, http.MethodPost, "/api/v1/admin/platform-resources/"+resourceID+"/access-key/reveal", []byte(`{"password":"P@ssw0rd"}`), principal)
	if revealResp.Code != http.StatusOK {
		t.Fatalf("expected reveal status 200, got %d", revealResp.Code)
	}
	revealPayload := decodePayload(t, revealResp)
	revealData, ok := revealPayload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected reveal data object")
	}
	if revealData["access_key"] != "sk-zhipu-rotated" {
		t.Fatalf("expected revealed access key sk-zhipu-rotated, got %#v", revealData["access_key"])
	}

	deleteResp := performJSONRequestWithPrincipal(t, mux, http.MethodDelete, "/api/v1/admin/platform-resources/"+resourceID, nil, principal)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected delete status 200, got %d", deleteResp.Code)
	}
	if _, exists := repo.resources[mustParseUUID(t, resourceID)]; exists {
		t.Fatalf("expected resource to be deleted")
	}
}

func TestPlatformResourceHandler_InvalidCategoryOrProviderReturns400(t *testing.T) {
	_, handler, principal := newPlatformResourceHandlerTestHarness(t)

	mux := http.NewServeMux()
	mux.Handle("/api/v1/admin/platform-resources", http.HandlerFunc(handler.Create))

	tests := []struct {
		name string
		body string
	}{
		{
			name: "invalid category",
			body: `{
				"category":"vision",
				"provider":"dashscope",
				"resource_key":"vision-dashscope-prod",
				"name":"invalid",
				"schema_version":1,
				"base_url":"https://dashscope.aliyuncs.com",
				"access_key":"sk-test",
				"capabilities":{"stream":true},
				"config":{"model":"x"}
			}`,
		},
		{
			name: "invalid provider",
			body: `{
				"category":"asr",
				"provider":"zhipu",
				"resource_key":"asr-zhipu-prod",
				"name":"invalid",
				"schema_version":1,
				"base_url":"https://dashscope.aliyuncs.com",
				"access_key":"sk-test",
				"capabilities":{"stream":true},
				"config":{"model":"x"}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := performJSONRequestWithPrincipal(t, mux, http.MethodPost, "/api/v1/admin/platform-resources", []byte(tt.body), principal)
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d", resp.Code)
			}
			payload := decodePayload(t, resp)
			if payload["code"] != "ERR_INVALID_ARGUMENT" {
				t.Fatalf("expected code ERR_INVALID_ARGUMENT, got %#v", payload["code"])
			}
		})
	}
}

func TestPlatformResourceHandler_UnauthorizedWithoutPrincipal(t *testing.T) {
	_, handler, _ := newPlatformResourceHandlerTestHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/platform-resources", nil)
	resp := httptest.NewRecorder()
	handler.List(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", resp.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["code"] != "ERR_UNAUTHORIZED" {
		t.Fatalf("expected code ERR_UNAUTHORIZED, got %#v", payload["code"])
	}
}

func TestPlatformResourceHandler_DeleteNotFound(t *testing.T) {
	_, handler, principal := newPlatformResourceHandlerTestHarness(t)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/admin/platform-resources/", http.HandlerFunc(handler.ByID))

	notFoundID := uuid.New().String()

	resp := performJSONRequestWithPrincipal(t, mux, http.MethodDelete, "/api/v1/admin/platform-resources/"+notFoundID, nil, principal)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.Code)
	}
	payload := decodePayload(t, resp)
	if payload["code"] != "ERR_NOT_FOUND" {
		t.Fatalf("expected code ERR_NOT_FOUND, got %#v", payload["code"])
	}
}

func newPlatformResourceHandlerTestHarness(t *testing.T) (*handlerPlatformResourceRepo, *PlatformResourceHandler, auth.Principal) {
	t.Helper()

	passwordHash, err := auth.HashPassword("P@ssw0rd")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	userID := uuid.New()
	userRepo := &handlerAuthUserRepo{
		byID: map[uuid.UUID]auth.User{
			userID: {
				ID:           userID,
				Email:        "admin@example.com",
				PasswordHash: passwordHash,
				Role:         contracts.RoleAdmin,
				Status:       contracts.UserStatusActive,
			},
		},
		byEmail: map[string]auth.User{},
	}
	userRepo.byEmail["admin@example.com"] = userRepo.byID[userID]

	authService := auth.NewService(userRepo, noopTokenManager{})
	resourceRepo := newHandlerPlatformResourceRepo()
	resourceService := platformresource.NewService(resourceRepo, handlerFakeAccessKeyCipher{})

	return resourceRepo, NewPlatformResourceHandler(resourceService, authService), auth.Principal{
		UserID: userID,
		Email:  "admin@example.com",
		Role:   contracts.RoleAdmin,
	}
}

func performJSONRequestWithPrincipal(t *testing.T, handler http.Handler, method, path string, body []byte, principal auth.Principal) *httptest.ResponseRecorder {
	t.Helper()

	reader := bytes.NewReader(body)
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if principal.UserID != uuid.Nil {
		req = req.WithContext(withPrincipal(req.Context(), principal))
	}

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

func mustParseUUID(t *testing.T, value string) uuid.UUID {
	t.Helper()
	parsed, err := uuid.Parse(value)
	if err != nil {
		t.Fatalf("uuid.Parse() error = %v", err)
	}
	return parsed
}

func copyRawMessageForHandler(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	cloned := make([]byte, len(raw))
	copy(cloned, raw)
	return json.RawMessage(cloned)
}
