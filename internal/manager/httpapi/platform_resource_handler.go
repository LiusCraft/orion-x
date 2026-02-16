package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/platformresource"
)

const platformResourceAdminPathPrefix = "/api/v1/admin/platform-resources/"

type PlatformResourceHandler struct {
	service *platformresource.Service
}

func NewPlatformResourceHandler(service *platformresource.Service) *PlatformResourceHandler {
	return &PlatformResourceHandler{service: service}
}

func (h *PlatformResourceHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req platformResourceCreateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "invalid request body",
		})
		return
	}

	resource, err := h.service.Create(r.Context(), principal.UserID, platformresource.CreateInput{
		Category:      req.Category,
		Provider:      req.Provider,
		ResourceKey:   req.ResourceKey,
		Name:          req.Name,
		SchemaVersion: req.SchemaVersion,
		Capabilities:  req.Capabilities,
		Config:        req.Config,
		CredentialRef: req.CredentialRef,
		Status:        req.Status,
	})
	if err != nil {
		writePlatformResourceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    "OK",
		"message": "",
		"data":    platformResourceDTO(resource),
	})
}

func (h *PlatformResourceHandler) List(w http.ResponseWriter, r *http.Request) {
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

	if _, ok := PrincipalFromContext(r.Context()); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"code":    "ERR_UNAUTHORIZED",
			"message": "unauthorized",
		})
		return
	}

	resources, err := h.service.List(r.Context(), platformresource.ListInput{
		Category: r.URL.Query().Get("category"),
		Provider: r.URL.Query().Get("provider"),
		Status:   r.URL.Query().Get("status"),
	})
	if err != nil {
		writePlatformResourceError(w, err)
		return
	}

	items := make([]map[string]any, 0, len(resources))
	for _, item := range resources {
		items = append(items, platformResourceDTO(item))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    "OK",
		"message": "",
		"data": map[string]any{
			"items": items,
		},
	})
}

func (h *PlatformResourceHandler) ByID(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
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

	resourceID, err := extractPlatformResourceID(r.URL.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "invalid resource id",
		})
		return
	}

	switch r.Method {
	case http.MethodPatch:
		h.patchByID(w, r, resourceID)
	case http.MethodDelete:
		h.deleteByID(w, r, resourceID)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *PlatformResourceHandler) patchByID(w http.ResponseWriter, r *http.Request, resourceID uuid.UUID) {
	var req platformResourceUpdateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "invalid request body",
		})
		return
	}

	resource, err := h.service.Update(r.Context(), resourceID, platformresource.UpdateInput{
		Category:      req.Category,
		Provider:      req.Provider,
		ResourceKey:   req.ResourceKey,
		Name:          req.Name,
		SchemaVersion: req.SchemaVersion,
		Capabilities:  req.Capabilities,
		Config:        req.Config,
		CredentialRef: req.CredentialRef,
		Status:        req.Status,
	})
	if err != nil {
		writePlatformResourceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    "OK",
		"message": "",
		"data":    platformResourceDTO(resource),
	})
}

func (h *PlatformResourceHandler) deleteByID(w http.ResponseWriter, r *http.Request, resourceID uuid.UUID) {
	err := h.service.Delete(r.Context(), resourceID)
	if err != nil {
		writePlatformResourceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    "OK",
		"message": "",
		"data": map[string]any{
			"id": resourceID.String(),
		},
	})
}

type platformResourceCreateRequest struct {
	Category      string          `json:"category"`
	Provider      string          `json:"provider"`
	ResourceKey   string          `json:"resource_key"`
	Name          string          `json:"name"`
	SchemaVersion int             `json:"schema_version"`
	Capabilities  json.RawMessage `json:"capabilities"`
	Config        json.RawMessage `json:"config"`
	CredentialRef string          `json:"credential_ref"`
	Status        string          `json:"status"`
}

type platformResourceUpdateRequest struct {
	Category      *string          `json:"category"`
	Provider      *string          `json:"provider"`
	ResourceKey   *string          `json:"resource_key"`
	Name          *string          `json:"name"`
	SchemaVersion *int             `json:"schema_version"`
	Capabilities  *json.RawMessage `json:"capabilities"`
	Config        *json.RawMessage `json:"config"`
	CredentialRef *string          `json:"credential_ref"`
	Status        *string          `json:"status"`
}

func platformResourceDTO(resource platformresource.Resource) map[string]any {
	return map[string]any{
		"id":             resource.ID.String(),
		"category":       resource.Category,
		"provider":       resource.Provider,
		"resource_key":   resource.ResourceKey,
		"name":           resource.Name,
		"schema_version": resource.SchemaVersion,
		"capabilities":   resource.Capabilities,
		"config":         resource.Config,
		"credential_ref": resource.CredentialRef,
		"status":         resource.Status,
		"created_by":     resource.CreatedBy.String(),
		"created_at":     resource.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":     resource.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func extractPlatformResourceID(path string) (uuid.UUID, error) {
	if !strings.HasPrefix(path, platformResourceAdminPathPrefix) {
		return uuid.Nil, errors.New("unsupported path")
	}
	rawID := strings.TrimSpace(strings.TrimPrefix(path, platformResourceAdminPathPrefix))
	if rawID == "" || strings.Contains(rawID, "/") {
		return uuid.Nil, errors.New("resource id is required")
	}

	resourceID, err := uuid.Parse(rawID)
	if err != nil {
		return uuid.Nil, err
	}
	return resourceID, nil
}

func writePlatformResourceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, platformresource.ErrInvalidArgument):
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": err.Error(),
		})
	case errors.Is(err, platformresource.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]any{
			"code":    "ERR_CONFLICT",
			"message": "resource already exists",
		})
	case errors.Is(err, platformresource.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{
			"code":    "ERR_NOT_FOUND",
			"message": "resource not found",
		})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"code":    "ERR_INTERNAL",
			"message": "internal server error",
		})
	}
}
