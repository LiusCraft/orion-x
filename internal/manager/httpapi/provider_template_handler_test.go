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
	"github.com/liuscraft/orion-x/internal/manager/providertemplate"
)

type fakeProviderTemplateRepo struct {
	items map[uuid.UUID]providertemplate.Template
}

func newFakeProviderTemplateRepo() *fakeProviderTemplateRepo {
	return &fakeProviderTemplateRepo{items: make(map[uuid.UUID]providertemplate.Template)}
}

func (r *fakeProviderTemplateRepo) Create(_ context.Context, template providertemplate.Template) (providertemplate.Template, error) {
	now := time.Now().UTC()
	template.CreatedAt = now
	template.UpdatedAt = now
	r.items[template.ID] = template
	return template, nil
}

func (r *fakeProviderTemplateRepo) GetByID(_ context.Context, id uuid.UUID) (providertemplate.Template, error) {
	t, ok := r.items[id]
	if !ok {
		return providertemplate.Template{}, providertemplate.ErrNotFound
	}
	return t, nil
}

func (r *fakeProviderTemplateRepo) List(_ context.Context, filter providertemplate.ListFilter) ([]providertemplate.Template, error) {
	items := make([]providertemplate.Template, 0)
	for _, item := range r.items {
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

func (r *fakeProviderTemplateRepo) Update(_ context.Context, id uuid.UUID, patch providertemplate.UpdatePatch) (providertemplate.Template, error) {
	t, ok := r.items[id]
	if !ok {
		return providertemplate.Template{}, providertemplate.ErrNotFound
	}
	if patch.Category != nil {
		t.Category = *patch.Category
	}
	if patch.Provider != nil {
		t.Provider = *patch.Provider
	}
	if patch.Status != nil {
		t.Status = *patch.Status
	}
	if patch.Version != nil {
		t.Version = *patch.Version
	}
	if patch.Fields != nil {
		t.Fields = append(json.RawMessage(nil), *patch.Fields...)
	}
	t.UpdatedAt = time.Now().UTC()
	r.items[id] = t
	return t, nil
}

func (r *fakeProviderTemplateRepo) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := r.items[id]; !ok {
		return providertemplate.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func TestProviderTemplateHandler_CRUDFlow(t *testing.T) {
	repo := newFakeProviderTemplateRepo()
	service := providertemplate.NewService(repo)
	handler := NewProviderTemplateHandler(service)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/admin/provider-templates", http.HandlerFunc(handler.Create))
	mux.Handle("/api/v1/admin/provider-templates/", http.HandlerFunc(handler.ByID))
	mux.Handle("/api/v1/provider-templates", http.HandlerFunc(handler.List))

	principal := auth.Principal{UserID: uuid.New(), Email: "admin@example.com", Role: contracts.RoleAdmin}

	createResp := performProviderTemplateRequest(t, mux, http.MethodPost, "/api/v1/admin/provider-templates", []byte(`{
		"category":"llm",
		"provider":"zhipu",
		"status":"active",
		"version":1,
		"fields":[
			{"key":"model","label":"Model","type":"select","required":true,"options":[{"label":"GLM-4","value":"glm-4"}]},
			{"key":"temperature","label":"Temperature","type":"number","min":0,"max":2,"step":0.1}
		]
	}`), principal)
	if createResp.Code != http.StatusOK {
		t.Fatalf("expected create status 200, got %d", createResp.Code)
	}
	createPayload := decodePayload(t, createResp)
	createData, ok := createPayload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected create data object")
	}
	templateID, _ := createData["id"].(string)
	if templateID == "" {
		t.Fatalf("expected non-empty template id")
	}

	listResp := performProviderTemplateRequest(t, mux, http.MethodGet, "/api/v1/provider-templates?category=llm&provider=zhipu&status=active", nil, principal)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d", listResp.Code)
	}
	listPayload := decodePayload(t, listResp)
	listData, ok := listPayload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected list data object")
	}
	items, ok := listData["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one template item, got %#v", listData["items"])
	}

	patchResp := performProviderTemplateRequest(t, mux, http.MethodPatch, "/api/v1/admin/provider-templates/"+templateID, []byte(`{
		"status":"inactive",
		"version":2
	}`), principal)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("expected patch status 200, got %d", patchResp.Code)
	}
	patchPayload := decodePayload(t, patchResp)
	patchData, ok := patchPayload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected patch data object")
	}
	if patchData["version"] != float64(2) {
		t.Fatalf("expected version 2, got %#v", patchData["version"])
	}
	if patchData["status"] != string(contracts.ResourceStatusInactive) {
		t.Fatalf("expected inactive status, got %#v", patchData["status"])
	}

	deleteResp := performProviderTemplateRequest(t, mux, http.MethodDelete, "/api/v1/admin/provider-templates/"+templateID, nil, principal)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("expected delete status 200, got %d", deleteResp.Code)
	}
}

func TestProviderTemplateHandler_InvalidBodyReturns400(t *testing.T) {
	service := providertemplate.NewService(newFakeProviderTemplateRepo())
	handler := NewProviderTemplateHandler(service)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/admin/provider-templates", http.HandlerFunc(handler.Create))

	principal := auth.Principal{UserID: uuid.New(), Email: "admin@example.com", Role: contracts.RoleAdmin}
	resp := performProviderTemplateRequest(t, mux, http.MethodPost, "/api/v1/admin/provider-templates", []byte(`{
		"category":"llm",
		"provider":"zhipu",
		"version":1,
		"fields":[{"key":"base_url","label":"Base URL","type":"text"}]
	}`), principal)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.Code)
	}
	payload := decodePayload(t, resp)
	if payload["code"] != "ERR_INVALID_ARGUMENT" {
		t.Fatalf("expected ERR_INVALID_ARGUMENT, got %#v", payload["code"])
	}
}

func TestProviderTemplateHandler_UnauthorizedWithoutPrincipal(t *testing.T) {
	service := providertemplate.NewService(newFakeProviderTemplateRepo())
	handler := NewProviderTemplateHandler(service)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider-templates", nil)
	resp := httptest.NewRecorder()
	handler.List(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", resp.Code)
	}
}

func performProviderTemplateRequest(t *testing.T, handler http.Handler, method, path string, body []byte, principal auth.Principal) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req = req.WithContext(withPrincipal(req.Context(), principal))

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}
