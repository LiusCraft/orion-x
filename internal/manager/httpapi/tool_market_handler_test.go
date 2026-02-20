package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/auth"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/toolentitlement"
	"github.com/liuscraft/orion-x/internal/manager/toolmarket"
	"github.com/liuscraft/orion-x/internal/manager/toolruntime"
)

type handlerToolConfigValidator struct{}

func (handlerToolConfigValidator) Validate(ctx context.Context, protocol contracts.ToolProtocol, raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	cloned := make([]byte, len(raw))
	copy(cloned, raw)
	return json.RawMessage(cloned), nil
}

type handlerToolMarketRepo struct {
	items map[uuid.UUID]toolmarket.Item
	keys  map[string]uuid.UUID
}

func newHandlerToolMarketRepo() *handlerToolMarketRepo {
	return &handlerToolMarketRepo{
		items: make(map[uuid.UUID]toolmarket.Item),
		keys:  make(map[string]uuid.UUID),
	}
}

func (r *handlerToolMarketRepo) Create(_ context.Context, item toolmarket.Item) (toolmarket.Item, error) {
	if _, exists := r.keys[item.ToolKey]; exists {
		return toolmarket.Item{}, toolmarket.ErrConflict
	}
	now := time.Now().UTC()
	item.CreatedAt = now
	item.UpdatedAt = now
	r.items[item.ID] = item
	r.keys[item.ToolKey] = item.ID
	return item, nil
}

func (r *handlerToolMarketRepo) GetByID(_ context.Context, id uuid.UUID) (toolmarket.Item, error) {
	item, exists := r.items[id]
	if !exists {
		return toolmarket.Item{}, toolmarket.ErrNotFound
	}
	return item, nil
}

func (r *handlerToolMarketRepo) List(_ context.Context, filter toolmarket.ListFilter) ([]toolmarket.Item, error) {
	items := make([]toolmarket.Item, 0)
	for _, item := range r.items {
		if filter.Provider != "" && filter.Provider != item.Provider {
			continue
		}
		if filter.Status != nil && item.Status != *filter.Status {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *handlerToolMarketRepo) Update(_ context.Context, id uuid.UUID, patch toolmarket.UpdatePatch) (toolmarket.Item, error) {
	item, exists := r.items[id]
	if !exists {
		return toolmarket.Item{}, toolmarket.ErrNotFound
	}
	if patch.ToolKey != nil {
		if otherID, used := r.keys[*patch.ToolKey]; used && otherID != id {
			return toolmarket.Item{}, toolmarket.ErrConflict
		}
		delete(r.keys, item.ToolKey)
		item.ToolKey = *patch.ToolKey
		r.keys[item.ToolKey] = id
	}
	if patch.Name != nil {
		item.Name = *patch.Name
	}
	if patch.Provider != nil {
		item.Provider = *patch.Provider
	}
	if patch.Protocol != nil {
		item.Protocol = *patch.Protocol
	}
	if patch.Config != nil {
		item.Config = copyRawMessageForHandler(*patch.Config)
	}
	if patch.Status != nil {
		item.Status = *patch.Status
	}
	item.UpdatedAt = time.Now().UTC()
	r.items[id] = item
	return item, nil
}

func (r *handlerToolMarketRepo) Delete(_ context.Context, id uuid.UUID) error {
	item, exists := r.items[id]
	if !exists {
		return toolmarket.ErrNotFound
	}
	delete(r.keys, item.ToolKey)
	delete(r.items, id)
	return nil
}

type handlerToolEntitlementRepo struct {
	entitlements map[uuid.UUID]toolentitlement.Entitlement
	usage        map[uuid.UUID][]toolentitlement.UsageEntry
}

func newHandlerToolEntitlementRepo() *handlerToolEntitlementRepo {
	return &handlerToolEntitlementRepo{
		entitlements: make(map[uuid.UUID]toolentitlement.Entitlement),
		usage:        make(map[uuid.UUID][]toolentitlement.UsageEntry),
	}
}

func (r *handlerToolEntitlementRepo) Create(_ context.Context, entitlement toolentitlement.Entitlement) (toolentitlement.Entitlement, error) {
	now := time.Now().UTC()
	entitlement.CreatedAt = now
	entitlement.UpdatedAt = now
	r.entitlements[entitlement.ID] = entitlement
	return entitlement, nil
}

func (r *handlerToolEntitlementRepo) ListByUser(_ context.Context, userID uuid.UUID, status *contracts.EntitlementStatus) ([]toolentitlement.Entitlement, error) {
	items := make([]toolentitlement.Entitlement, 0)
	for _, item := range r.entitlements {
		if item.UserID != userID {
			continue
		}
		if status != nil && item.Status != *status {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *handlerToolEntitlementRepo) GetByIDForUser(_ context.Context, id, userID uuid.UUID) (toolentitlement.Entitlement, error) {
	item, exists := r.entitlements[id]
	if !exists || item.UserID != userID {
		return toolentitlement.Entitlement{}, toolentitlement.ErrNotFound
	}
	return item, nil
}

func (r *handlerToolEntitlementRepo) ListUsageByEntitlement(_ context.Context, entitlementID uuid.UUID) ([]toolentitlement.UsageEntry, error) {
	entries := r.usage[entitlementID]
	cloned := make([]toolentitlement.UsageEntry, len(entries))
	copy(cloned, entries)
	return cloned, nil
}

type handlerToolUserReader struct {
	users map[uuid.UUID]auth.User
}

func (r *handlerToolUserReader) GetByID(_ context.Context, id uuid.UUID) (auth.User, error) {
	user, exists := r.users[id]
	if !exists {
		return auth.User{}, auth.ErrUserNotFound
	}
	return user, nil
}

type fakeToolRuntimeService struct {
	listResult []toolruntime.ToolDescriptor
	callResult toolruntime.ToolCallResult
	err        error
}

func (s *fakeToolRuntimeService) ListTools(_ context.Context, _ uuid.UUID, _ uuid.UUID) ([]toolruntime.ToolDescriptor, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.listResult, nil
}

func (s *fakeToolRuntimeService) CallTool(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ string, _ map[string]any) (toolruntime.ToolCallResult, error) {
	if s.err != nil {
		return toolruntime.ToolCallResult{}, s.err
	}
	return s.callResult, nil
}

func TestToolMarketHandler_MainFlow(t *testing.T) {
	adminID := uuid.New()
	userID := uuid.New()

	marketRepo := newHandlerToolMarketRepo()
	entitlementRepo := newHandlerToolEntitlementRepo()
	users := &handlerToolUserReader{users: map[uuid.UUID]auth.User{
		adminID: {ID: adminID, Status: contracts.UserStatusActive},
		userID:  {ID: userID, Status: contracts.UserStatusActive},
	}}

	marketService := toolmarket.NewService(marketRepo, handlerToolConfigValidator{})
	entitlementService := toolentitlement.NewService(entitlementRepo, marketRepo, users)

	marketHandler := NewToolMarketHandler(marketService, entitlementService)
	entitlementHandler := NewToolEntitlementHandler(entitlementService)

	mux := http.NewServeMux()
	mux.Handle("/api/v1/admin/tool-market/items", http.HandlerFunc(marketHandler.AdminItems))
	mux.Handle("/api/v1/admin/tool-market/items/", http.HandlerFunc(marketHandler.AdminByItem))
	mux.Handle("/api/v1/admin/tool-entitlements/grant", http.HandlerFunc(entitlementHandler.Grant))
	mux.Handle("/api/v1/tool-market/items", http.HandlerFunc(marketHandler.PublicItems))
	mux.Handle("/api/v1/tool-market/items/", http.HandlerFunc(marketHandler.PublicByItem))
	mux.Handle("/api/v1/me/tool-repo", http.HandlerFunc(entitlementHandler.ListRepo))
	mux.Handle("/api/v1/me/tool-repo/", http.HandlerFunc(entitlementHandler.RepoByID))

	adminPrincipal := auth.Principal{UserID: adminID, Role: contracts.RoleAdmin}
	userPrincipal := auth.Principal{UserID: userID, Role: contracts.RoleNormalUser}

	createItemResp := performJSONRequestWithPrincipal(t, mux, http.MethodPost, "/api/v1/admin/tool-market/items", []byte(`{
		"tool_key":"mcp-device-helper",
		"name":"Device Helper",
		"provider":"acme",
		"protocol":"mcp",
		"config":{
			"transport":"stream_http",
			"stream_http":{"endpoint":"https://example.com/mcp"}
		}
	}`), adminPrincipal)
	if createItemResp.Code != http.StatusOK {
		t.Fatalf("expected create item 200, got %d", createItemResp.Code)
	}
	createItemPayload := decodePayload(t, createItemResp)
	createItemData := createItemPayload["data"].(map[string]any)
	itemID := createItemData["id"].(string)

	listMarketResp := performJSONRequestWithPrincipal(t, mux, http.MethodGet, "/api/v1/tool-market/items", nil, userPrincipal)
	if listMarketResp.Code != http.StatusOK {
		t.Fatalf("expected market list 200, got %d", listMarketResp.Code)
	}

	activateResp := performJSONRequestWithPrincipal(t, mux, http.MethodPost, "/api/v1/tool-market/items/"+itemID+"/activate", nil, userPrincipal)
	if activateResp.Code != http.StatusOK {
		t.Fatalf("expected activate 200, got %d", activateResp.Code)
	}
	activateData := decodePayload(t, activateResp)["data"].(map[string]any)
	entitlementID := activateData["id"].(string)

	reactivateResp := performJSONRequestWithPrincipal(t, mux, http.MethodPost, "/api/v1/tool-market/items/"+itemID+"/activate", nil, userPrincipal)
	if reactivateResp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected duplicate activate 422, got %d", reactivateResp.Code)
	}

	listRepoResp := performJSONRequestWithPrincipal(t, mux, http.MethodGet, "/api/v1/me/tool-repo", nil, userPrincipal)
	if listRepoResp.Code != http.StatusOK {
		t.Fatalf("expected list repo 200, got %d", listRepoResp.Code)
	}
	repoItems := decodePayload(t, listRepoResp)["data"].(map[string]any)["items"].([]any)
	if len(repoItems) != 1 {
		t.Fatalf("expected 1 entitlement in repo, got %d", len(repoItems))
	}

	usageResp := performJSONRequestWithPrincipal(t, mux, http.MethodGet, "/api/v1/me/tool-repo/"+entitlementID+"/usage", nil, userPrincipal)
	if usageResp.Code != http.StatusOK {
		t.Fatalf("expected usage 200, got %d", usageResp.Code)
	}
}

func TestToolMarketHandler_RejectsInactiveStatusFilterForPublicAPI(t *testing.T) {
	marketService := toolmarket.NewService(newHandlerToolMarketRepo(), handlerToolConfigValidator{})
	handler := NewToolMarketHandler(marketService, nil)

	principal := auth.Principal{UserID: uuid.New(), Role: contracts.RoleNormalUser}
	resp := performJSONRequestWithPrincipal(t, http.HandlerFunc(handler.PublicItems), http.MethodGet, "/api/v1/tool-market/items?status=inactive", nil, principal)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	payload := decodePayload(t, resp)
	if payload["code"] != "ERR_INVALID_ARGUMENT" {
		t.Fatalf("expected ERR_INVALID_ARGUMENT, got %#v", payload["code"])
	}
}

func TestToolMarketHandler_UnauthorizedWithoutPrincipal(t *testing.T) {
	marketService := toolmarket.NewService(newHandlerToolMarketRepo(), handlerToolConfigValidator{})
	handler := NewToolMarketHandler(marketService, nil)

	resp := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tool-market/items", nil)
	handler.PublicItems(resp, request)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", resp.Code)
	}
	payload := decodePayload(t, resp)
	if payload["code"] != "ERR_UNAUTHORIZED" {
		t.Fatalf("expected ERR_UNAUTHORIZED, got %#v", payload["code"])
	}
}

func TestToolEntitlementHandler_RuntimeToolsEndpoints(t *testing.T) {
	entitlementHandler := NewToolEntitlementHandler(nil)
	entitlementHandler.runtimeService = &fakeToolRuntimeService{
		listResult: []toolruntime.ToolDescriptor{{
			Name:        "ping",
			Description: "ping tool",
			InputSchema: map[string]any{"type": "object"},
		}},
		callResult: toolruntime.ToolCallResult{
			ToolName: "ping",
			Result:   map[string]any{"ok": true},
		},
	}

	princ := auth.Principal{UserID: uuid.New(), Role: contracts.RoleNormalUser}
	entitlementID := uuid.New().String()

	listResp := performJSONRequestWithPrincipal(t, http.HandlerFunc(entitlementHandler.RepoByID), http.MethodGet, "/api/v1/me/tool-repo/"+entitlementID+"/tools/list", nil, princ)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected tools/list 200, got %d", listResp.Code)
	}
	listPayload := decodePayload(t, listResp)
	items := listPayload["data"].(map[string]any)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 tool item, got %d", len(items))
	}

	callResp := performJSONRequestWithPrincipal(t, http.HandlerFunc(entitlementHandler.RepoByID), http.MethodPost, "/api/v1/me/tool-repo/"+entitlementID+"/tools/call", []byte(`{"tool_name":"ping","arguments":{}}`), princ)
	if callResp.Code != http.StatusOK {
		t.Fatalf("expected tools/call 200, got %d", callResp.Code)
	}
	callPayload := decodePayload(t, callResp)
	if callPayload["data"].(map[string]any)["tool_name"] != "ping" {
		t.Fatalf("expected tool_name ping, got %#v", callPayload["data"].(map[string]any)["tool_name"])
	}
}

func TestToolEntitlementHandler_RuntimeToolsCallRejectsInvalidBody(t *testing.T) {
	entitlementHandler := NewToolEntitlementHandler(nil)
	entitlementHandler.runtimeService = &fakeToolRuntimeService{}
	princ := auth.Principal{UserID: uuid.New(), Role: contracts.RoleNormalUser}
	entitlementID := uuid.New().String()

	resp := performJSONRequestWithPrincipal(t, http.HandlerFunc(entitlementHandler.RepoByID), http.MethodPost, "/api/v1/me/tool-repo/"+entitlementID+"/tools/call", []byte(`{"tool_name":123}`), princ)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	payload := decodePayload(t, resp)
	if payload["code"] != "ERR_INVALID_ARGUMENT" {
		t.Fatalf("expected ERR_INVALID_ARGUMENT, got %#v", payload["code"])
	}
}

func TestParseToolRepoPath_RejectsInvalidPath(t *testing.T) {
	_, _, err := parseToolRepoPath("/api/v1/me/tool-repo/not-uuid")
	if err == nil {
		t.Fatalf("expected parse error")
	}
}
