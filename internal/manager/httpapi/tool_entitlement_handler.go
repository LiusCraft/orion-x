package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/toolentitlement"
	"github.com/liuscraft/orion-x/internal/manager/toolruntime"
)

const toolRepoPathPrefix = "/api/v1/me/tool-repo/"

type ToolEntitlementHandler struct {
	service        *toolentitlement.Service
	runtimeService toolRuntimeService
}

type toolRuntimeService interface {
	ListTools(ctx context.Context, userID, entitlementID uuid.UUID) ([]toolruntime.ToolDescriptor, error)
	CallTool(ctx context.Context, userID, entitlementID uuid.UUID, toolName string, arguments map[string]any) (toolruntime.ToolCallResult, error)
}

func NewToolEntitlementHandler(service *toolentitlement.Service) *ToolEntitlementHandler {
	return &ToolEntitlementHandler{service: service}
}

func (h *ToolEntitlementHandler) SetRuntimeService(runtimeService toolRuntimeService) {
	h.runtimeService = runtimeService
}

func (h *ToolEntitlementHandler) Grant(w http.ResponseWriter, r *http.Request) {
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

	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"code":    "ERR_UNAUTHORIZED",
			"message": "unauthorized",
		})
		return
	}

	var req toolEntitlementGrantRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "invalid request body",
		})
		return
	}

	userID, err := uuid.Parse(strings.TrimSpace(req.UserID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "invalid user_id",
		})
		return
	}
	itemID, err := uuid.Parse(strings.TrimSpace(req.ItemID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "invalid item_id",
		})
		return
	}

	granted, err := h.service.Grant(r.Context(), principal.UserID, toolentitlement.GrantInput{
		UserID:    userID,
		ItemID:    itemID,
		SourceRef: req.SourceRef,
		StartsAt:  req.StartsAt,
	})
	if err != nil {
		writeToolEntitlementServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    "OK",
		"message": "",
		"data":    toolEntitlementDTO(granted),
	})
}

func (h *ToolEntitlementHandler) ListRepo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
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

	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"code":    "ERR_UNAUTHORIZED",
			"message": "unauthorized",
		})
		return
	}

	items, err := h.service.ListRepo(r.Context(), principal.UserID, toolentitlement.RepoListInput{
		Status: r.URL.Query().Get("status"),
	})
	if err != nil {
		writeToolEntitlementServiceError(w, err)
		return
	}

	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, toolRepoEntryDTO(item))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    "OK",
		"message": "",
		"data": map[string]any{
			"items": payload,
		},
	})
}

func (h *ToolEntitlementHandler) RepoByID(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"code":    "ERR_UNAUTHORIZED",
			"message": "unauthorized",
		})
		return
	}

	entitlementID, action, err := parseToolRepoPath(r.URL.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "invalid entitlement path",
		})
		return
	}

	switch action {
	case "usage":
		if r.Method != http.MethodGet {
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

		summary, usageErr := h.service.GetUsage(r.Context(), principal.UserID, entitlementID)
		if usageErr != nil {
			writeToolEntitlementServiceError(w, usageErr)
			return
		}

		entries := make([]map[string]any, 0, len(summary.Entries))
		for _, entry := range summary.Entries {
			entries = append(entries, toolUsageEntryDTO(entry))
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"code":    "OK",
			"message": "",
			"data": map[string]any{
				"entitlement":     toolEntitlementDTO(summary.Entitlement),
				"quota_total":     summary.Entitlement.QuotaTotal,
				"quota_used":      summary.Entitlement.QuotaUsed,
				"remaining_quota": summary.RemainingQuota,
				"entries":         entries,
			},
		})
	case "tools/list":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if h.runtimeService == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"code":    "ERR_INTERNAL",
				"message": "internal server error",
			})
			return
		}

		toolsList, listErr := h.runtimeService.ListTools(r.Context(), principal.UserID, entitlementID)
		if listErr != nil {
			writeToolRuntimeServiceError(w, listErr)
			return
		}

		items := make([]map[string]any, 0, len(toolsList))
		for _, item := range toolsList {
			items = append(items, map[string]any{
				"name":         item.Name,
				"description":  item.Description,
				"input_schema": item.InputSchema,
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"code":    "OK",
			"message": "",
			"data": map[string]any{
				"items": items,
			},
		})
	case "tools/call":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if h.runtimeService == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"code":    "ERR_INTERNAL",
				"message": "internal server error",
			})
			return
		}

		var req toolRuntimeCallRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"code":    "ERR_INVALID_ARGUMENT",
				"message": "invalid request body",
			})
			return
		}

		result, callErr := h.runtimeService.CallTool(r.Context(), principal.UserID, entitlementID, req.ToolName, req.Arguments)
		if callErr != nil {
			writeToolRuntimeServiceError(w, callErr)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"code":    "OK",
			"message": "",
			"data": map[string]any{
				"tool_name": result.ToolName,
				"result":    result.Result,
			},
		})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "unsupported action",
		})
	}
}

type toolEntitlementGrantRequest struct {
	UserID    string     `json:"user_id"`
	ItemID    string     `json:"item_id"`
	SourceRef string     `json:"source_ref"`
	StartsAt  *time.Time `json:"starts_at"`
}

type toolRuntimeCallRequest struct {
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments"`
}

func parseToolRepoPath(path string) (uuid.UUID, string, error) {
	if !strings.HasPrefix(path, toolRepoPathPrefix) {
		return uuid.Nil, "", errors.New("unsupported path")
	}
	raw := strings.Trim(strings.TrimPrefix(path, toolRepoPathPrefix), "/")
	if raw == "" {
		return uuid.Nil, "", errors.New("entitlement id is required")
	}

	parts := strings.Split(raw, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return uuid.Nil, "", errors.New("unsupported action")
	}

	entitlementID, err := uuid.Parse(strings.TrimSpace(parts[0]))
	if err != nil {
		return uuid.Nil, "", err
	}

	if len(parts) == 2 && parts[1] == "usage" {
		return entitlementID, "usage", nil
	}
	if len(parts) == 3 && parts[1] == "tools" {
		switch parts[2] {
		case "list":
			return entitlementID, "tools/list", nil
		case "call":
			return entitlementID, "tools/call", nil
		}
	}

	return uuid.Nil, "", errors.New("unsupported action")
}

func toolEntitlementDTO(entitlement toolentitlement.Entitlement) map[string]any {
	return map[string]any{
		"id":           entitlement.ID.String(),
		"user_id":      entitlement.UserID.String(),
		"tool_item_id": entitlement.ToolItemID.String(),
		"source_type":  entitlement.SourceType,
		"source_ref":   entitlement.SourceRef,
		"status":       entitlement.Status,
		"starts_at":    entitlement.StartsAt.UTC().Format(time.RFC3339),
		"expires_at":   formatOptionalTime(entitlement.ExpiresAt),
		"quota_total":  entitlement.QuotaTotal,
		"quota_used":   entitlement.QuotaUsed,
		"created_at":   entitlement.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":   entitlement.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toolRepoEntryDTO(entry toolentitlement.RepoEntry) map[string]any {
	return map[string]any{
		"is_usable":   entry.IsUsable,
		"entitlement": toolEntitlementDTO(entry.Entitlement),
		"tool_item": map[string]any{
			"id":       entry.Item.ID.String(),
			"tool_key": entry.Item.ToolKey,
			"name":     entry.Item.Name,
			"provider": entry.Item.Provider,
			"protocol": entry.Item.Protocol,
			"status":   entry.Item.Status,
		},
	}
}

func toolUsageEntryDTO(entry toolentitlement.UsageEntry) map[string]any {
	return map[string]any{
		"id":             entry.ID.String(),
		"entitlement_id": entry.EntitlementID.String(),
		"voicebot_id":    formatOptionalUUID(entry.VoicebotID),
		"device_id":      formatOptionalUUID(entry.DeviceID),
		"consumed_units": entry.ConsumedUnits,
		"created_at":     entry.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func formatOptionalUUID(value *uuid.UUID) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func formatOptionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339)
}

func writeToolEntitlementServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, toolentitlement.ErrInvalidArgument):
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": err.Error(),
		})
	case errors.Is(err, toolentitlement.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]any{
			"code":    "ERR_CONFLICT",
			"message": "resource already exists",
		})
	case errors.Is(err, toolentitlement.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{
			"code":    "ERR_NOT_FOUND",
			"message": "entitlement not found",
		})
	case errors.Is(err, toolentitlement.ErrBusinessRule):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"code":    "ERR_BUSINESS_RULE",
			"message": err.Error(),
		})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"code":    "ERR_INTERNAL",
			"message": "internal server error",
		})
	}
}

func writeToolRuntimeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, toolruntime.ErrInvalidArgument):
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": err.Error(),
		})
	case errors.Is(err, toolruntime.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{
			"code":    "ERR_NOT_FOUND",
			"message": "tool runtime target not found",
		})
	case errors.Is(err, toolruntime.ErrBusinessRule):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"code":    "ERR_BUSINESS_RULE",
			"message": err.Error(),
		})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"code":    "ERR_INTERNAL",
			"message": "internal server error",
		})
	}
}
