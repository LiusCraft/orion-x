package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	if patch.Capabilities != nil {
		resource.Capabilities = copyRawMessageForHandler(*patch.Capabilities)
	}
	if patch.Config != nil {
		resource.Config = copyRawMessageForHandler(*patch.Config)
	}
	if patch.CredentialRef != nil {
		resource.CredentialRef = *patch.CredentialRef
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

func TestPlatformResourceHandler_CRUDFlow(t *testing.T) {
	repo := newHandlerPlatformResourceRepo()
	handler := NewPlatformResourceHandler(platformresource.NewService(repo))
	mux := http.NewServeMux()
	mux.Handle("/api/v1/admin/platform-resources", http.HandlerFunc(handler.Create))
	mux.Handle("/api/v1/admin/platform-resources/", http.HandlerFunc(handler.ByID))
	mux.Handle("/api/v1/platform-resources", http.HandlerFunc(handler.List))

	principal := auth.Principal{
		UserID: uuid.New(),
		Email:  "admin@example.com",
		Role:   contracts.RoleAdmin,
	}

	createResp := performJSONRequestWithPrincipal(t, mux, http.MethodPost, "/api/v1/admin/platform-resources", []byte(`{
		"category":"llm",
		"provider":"zhipu",
		"resource_key":"llm-zhipu-prod",
		"name":"LLM Zhipu Prod",
		"schema_version":1,
		"capabilities":{"stream":true},
		"config":{"model":"glm-4-flash"},
		"credential_ref":"secret://manager/llm/zhipu/prod"
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

	deleteResp := performJSONRequestWithPrincipal(t, mux, http.MethodDelete, "/api/v1/admin/platform-resources/"+resourceID, nil, principal)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected delete status 200, got %d", deleteResp.Code)
	}
	if _, exists := repo.resources[mustParseUUID(t, resourceID)]; exists {
		t.Fatalf("expected resource to be deleted")
	}
}

func TestPlatformResourceHandler_InvalidCategoryOrProviderReturns400(t *testing.T) {
	repo := newHandlerPlatformResourceRepo()
	handler := NewPlatformResourceHandler(platformresource.NewService(repo))
	mux := http.NewServeMux()
	mux.Handle("/api/v1/admin/platform-resources", http.HandlerFunc(handler.Create))

	principal := auth.Principal{UserID: uuid.New(), Email: "admin@example.com", Role: contracts.RoleAdmin}

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
				"capabilities":{"stream":true},
				"config":{"model":"x"},
				"credential_ref":"secret://x"
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
				"capabilities":{"stream":true},
				"config":{"model":"x"},
				"credential_ref":"secret://x"
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
	repo := newHandlerPlatformResourceRepo()
	handler := NewPlatformResourceHandler(platformresource.NewService(repo))

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
	repo := newHandlerPlatformResourceRepo()
	handler := NewPlatformResourceHandler(platformresource.NewService(repo))
	mux := http.NewServeMux()
	mux.Handle("/api/v1/admin/platform-resources/", http.HandlerFunc(handler.ByID))

	principal := auth.Principal{UserID: uuid.New(), Email: "admin@example.com", Role: contracts.RoleAdmin}
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
