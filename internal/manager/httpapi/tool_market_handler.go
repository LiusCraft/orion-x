package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/toolentitlement"
	"github.com/liuscraft/orion-x/internal/manager/toolmarket"
)

const (
	toolMarketAdminPathPrefix  = "/api/v1/admin/tool-market/items/"
	toolMarketPublicPathPrefix = "/api/v1/tool-market/items/"
)

type ToolMarketHandler struct {
	marketService      *toolmarket.Service
	entitlementService *toolentitlement.Service
}

func NewToolMarketHandler(
	marketService *toolmarket.Service,
	entitlementService *toolentitlement.Service,
) *ToolMarketHandler {
	return &ToolMarketHandler{
		marketService:      marketService,
		entitlementService: entitlementService,
	}
}

func (h *ToolMarketHandler) AdminItems(w http.ResponseWriter, r *http.Request) {
	if h.marketService == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"code":    "ERR_INTERNAL",
			"message": "internal server error",
		})
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

	switch r.Method {
	case http.MethodPost:
		var req toolMarketCreateRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"code":    "ERR_INVALID_ARGUMENT",
				"message": "invalid request body",
			})
			return
		}

		created, err := h.marketService.Create(r.Context(), principal.UserID, toolmarket.CreateInput{
			ToolKey:  req.ToolKey,
			Name:     req.Name,
			Provider: req.Provider,
			Protocol: req.Protocol,
			Config:   req.Config,
			Status:   req.Status,
		})
		if err != nil {
			writeToolMarketServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"code":    "OK",
			"message": "",
			"data":    toolMarketItemDTO(created),
		})
	case http.MethodGet:
		items, err := h.marketService.List(r.Context(), toolmarket.ListInput{
			Provider: r.URL.Query().Get("provider"),
			Status:   r.URL.Query().Get("status"),
		})
		if err != nil {
			writeToolMarketServiceError(w, err)
			return
		}

		payload := make([]map[string]any, 0, len(items))
		for _, item := range items {
			payload = append(payload, toolMarketItemDTO(item))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"code":    "OK",
			"message": "",
			"data": map[string]any{
				"items": payload,
			},
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *ToolMarketHandler) AdminByItem(w http.ResponseWriter, r *http.Request) {
	if h.marketService == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"code":    "ERR_INTERNAL",
			"message": "internal server error",
		})
		return
	}
	if _, ok := PrincipalFromContext(r.Context()); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"code":    "ERR_UNAUTHORIZED",
			"message": "unauthorized",
		})
		return
	}

	itemID, err := parseAdminToolMarketPath(r.URL.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "invalid tool market path",
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		item, err := h.marketService.GetByID(r.Context(), itemID)
		if err != nil {
			writeToolMarketServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"code":    "OK",
			"message": "",
			"data":    toolMarketItemDTO(item),
		})
	case http.MethodPatch:
		var req toolMarketUpdateRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"code":    "ERR_INVALID_ARGUMENT",
				"message": "invalid request body",
			})
			return
		}

		updated, err := h.marketService.Update(r.Context(), itemID, toolmarket.UpdateInput{
			ToolKey:  req.ToolKey,
			Name:     req.Name,
			Provider: req.Provider,
			Protocol: req.Protocol,
			Config:   req.Config,
			Status:   req.Status,
		})
		if err != nil {
			writeToolMarketServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"code":    "OK",
			"message": "",
			"data":    toolMarketItemDTO(updated),
		})
	case http.MethodDelete:
		if err := h.marketService.Delete(r.Context(), itemID); err != nil {
			writeToolMarketServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"code":    "OK",
			"message": "",
			"data": map[string]any{
				"id": itemID.String(),
			},
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *ToolMarketHandler) PublicItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.marketService == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"code":    "ERR_INTERNAL",
			"message": "internal server error",
		})
		return
	}
	if _, ok := PrincipalFromContext(r.Context()); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"code":    "ERR_UNAUTHORIZED",
			"message": "unauthorized",
		})
		return
	}

	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	if status == "" {
		status = string(contracts.ToolStatusActive)
	}
	if status != string(contracts.ToolStatusActive) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "status filter only supports active",
		})
		return
	}

	items, err := h.marketService.List(r.Context(), toolmarket.ListInput{
		Provider: r.URL.Query().Get("provider"),
		Status:   status,
	})
	if err != nil {
		writeToolMarketServiceError(w, err)
		return
	}

	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, toolMarketItemDTO(item))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    "OK",
		"message": "",
		"data": map[string]any{
			"items": payload,
		},
	})
}

func (h *ToolMarketHandler) PublicByItem(w http.ResponseWriter, r *http.Request) {
	if h.entitlementService == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"code":    "ERR_INTERNAL",
			"message": "internal server error",
		})
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

	itemID, action, err := parsePublicToolMarketPath(r.URL.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "invalid tool market path",
		})
		return
	}

	if action != "activate" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "unsupported action",
		})
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	entitlement, activateErr := h.entitlementService.Activate(r.Context(), principal.UserID, toolentitlement.ActivateInput{
		ItemID: itemID,
	})
	if activateErr != nil {
		writeToolEntitlementServiceError(w, activateErr)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    "OK",
		"message": "",
		"data":    toolEntitlementDTO(entitlement),
	})
}

type toolMarketCreateRequest struct {
	ToolKey  string          `json:"tool_key"`
	Name     string          `json:"name"`
	Provider string          `json:"provider"`
	Protocol string          `json:"protocol"`
	Config   json.RawMessage `json:"config"`
	Status   string          `json:"status"`
}

type toolMarketUpdateRequest struct {
	ToolKey  *string          `json:"tool_key"`
	Name     *string          `json:"name"`
	Provider *string          `json:"provider"`
	Protocol *string          `json:"protocol"`
	Config   *json.RawMessage `json:"config"`
	Status   *string          `json:"status"`
}

func parseAdminToolMarketPath(path string) (uuid.UUID, error) {
	if !strings.HasPrefix(path, toolMarketAdminPathPrefix) {
		return uuid.Nil, errors.New("unsupported path")
	}
	raw := strings.Trim(strings.TrimPrefix(path, toolMarketAdminPathPrefix), "/")
	if raw == "" {
		return uuid.Nil, errors.New("item id is required")
	}

	parts := strings.Split(raw, "/")
	if len(parts) != 1 {
		return uuid.Nil, errors.New("unsupported path")
	}

	itemID, err := uuid.Parse(strings.TrimSpace(parts[0]))
	if err != nil {
		return uuid.Nil, err
	}

	return itemID, nil
}

func parsePublicToolMarketPath(path string) (uuid.UUID, string, error) {
	if !strings.HasPrefix(path, toolMarketPublicPathPrefix) {
		return uuid.Nil, "", errors.New("unsupported path")
	}
	raw := strings.Trim(strings.TrimPrefix(path, toolMarketPublicPathPrefix), "/")
	if raw == "" {
		return uuid.Nil, "", errors.New("item id is required")
	}

	parts := strings.Split(raw, "/")
	if len(parts) != 2 {
		return uuid.Nil, "", errors.New("unsupported path")
	}

	itemID, err := uuid.Parse(strings.TrimSpace(parts[0]))
	if err != nil {
		return uuid.Nil, "", err
	}

	if parts[1] != "activate" {
		return uuid.Nil, "", errors.New("unsupported action")
	}
	return itemID, "activate", nil
}

func toolMarketItemDTO(item toolmarket.Item) map[string]any {
	return map[string]any{
		"id":         item.ID.String(),
		"tool_key":   item.ToolKey,
		"name":       item.Name,
		"provider":   item.Provider,
		"protocol":   item.Protocol,
		"config":     item.Config,
		"status":     item.Status,
		"created_by": item.CreatedBy.String(),
		"created_at": item.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": item.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func writeToolMarketServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, toolmarket.ErrInvalidArgument):
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": err.Error(),
		})
	case errors.Is(err, toolmarket.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]any{
			"code":    "ERR_CONFLICT",
			"message": "resource already exists",
		})
	case errors.Is(err, toolmarket.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{
			"code":    "ERR_NOT_FOUND",
			"message": "tool market item not found",
		})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"code":    "ERR_INTERNAL",
			"message": "internal server error",
		})
	}
}
