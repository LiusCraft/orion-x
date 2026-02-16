package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/manager/auth"
	"github.com/liuscraft/orion-x/internal/manager/platformresource"
)

const (
	platformResourceAdminPathPrefix = "/api/v1/admin/platform-resources/"
	revealAccessKeyAction           = "reveal_access_key"
)

type PlatformResourceHandler struct {
	resourceService *platformresource.Service
	authService     *auth.Service
}

func NewPlatformResourceHandler(resourceService *platformresource.Service, authService *auth.Service) *PlatformResourceHandler {
	return &PlatformResourceHandler{
		resourceService: resourceService,
		authService:     authService,
	}
}

func (h *PlatformResourceHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.resourceService == nil {
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

	resource, err := h.resourceService.Create(r.Context(), principal.UserID, platformresource.CreateInput{
		Category:      req.Category,
		Provider:      req.Provider,
		ResourceKey:   req.ResourceKey,
		Name:          req.Name,
		SchemaVersion: req.SchemaVersion,
		BaseURL:       req.BaseURL,
		AccessKey:     req.AccessKey,
		Capabilities:  req.Capabilities,
		Config:        req.Config,
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
	if h.resourceService == nil {
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

	resources, err := h.resourceService.List(r.Context(), platformresource.ListInput{
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
	if h.resourceService == nil {
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

	resourceID, action, err := parsePlatformResourceSubPath(r.URL.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "invalid resource path",
		})
		return
	}

	if action == revealAccessKeyAction {
		h.revealAccessKey(w, r, principal, resourceID)
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

	resource, err := h.resourceService.Update(r.Context(), resourceID, platformresource.UpdateInput{
		Category:      req.Category,
		Provider:      req.Provider,
		ResourceKey:   req.ResourceKey,
		Name:          req.Name,
		SchemaVersion: req.SchemaVersion,
		BaseURL:       req.BaseURL,
		AccessKey:     req.AccessKey,
		Capabilities:  req.Capabilities,
		Config:        req.Config,
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
	err := h.resourceService.Delete(r.Context(), resourceID)
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

func (h *PlatformResourceHandler) revealAccessKey(w http.ResponseWriter, r *http.Request, principal auth.Principal, resourceID uuid.UUID) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.authService == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"code":    "ERR_INTERNAL",
			"message": "internal server error",
		})
		return
	}

	var req revealAccessKeyRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "invalid request body",
		})
		return
	}

	if err := h.authService.Reauthenticate(r.Context(), principal.UserID, req.Password); err != nil {
		writeAuthServiceError(w, err)
		return
	}

	accessKey, err := h.resourceService.RevealAccessKey(r.Context(), resourceID)
	if err != nil {
		writePlatformResourceError(w, err)
		return
	}

	logging.Infof("manager access key revealed user_id=%s resource_id=%s", principal.UserID.String(), resourceID.String())

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    "OK",
		"message": "",
		"data": map[string]any{
			"id":         resourceID.String(),
			"access_key": accessKey,
		},
	})
}

type platformResourceCreateRequest struct {
	Category      string          `json:"category"`
	Provider      string          `json:"provider"`
	ResourceKey   string          `json:"resource_key"`
	Name          string          `json:"name"`
	SchemaVersion int             `json:"schema_version"`
	BaseURL       string          `json:"base_url"`
	AccessKey     string          `json:"access_key"`
	Capabilities  json.RawMessage `json:"capabilities"`
	Config        json.RawMessage `json:"config"`
	Status        string          `json:"status"`
}

type platformResourceUpdateRequest struct {
	Category      *string          `json:"category"`
	Provider      *string          `json:"provider"`
	ResourceKey   *string          `json:"resource_key"`
	Name          *string          `json:"name"`
	SchemaVersion *int             `json:"schema_version"`
	BaseURL       *string          `json:"base_url"`
	AccessKey     *string          `json:"access_key"`
	Capabilities  *json.RawMessage `json:"capabilities"`
	Config        *json.RawMessage `json:"config"`
	Status        *string          `json:"status"`
}

type revealAccessKeyRequest struct {
	Password string `json:"password"`
}

func platformResourceDTO(resource platformresource.Resource) map[string]any {
	return map[string]any{
		"id":             resource.ID.String(),
		"category":       resource.Category,
		"provider":       resource.Provider,
		"resource_key":   resource.ResourceKey,
		"name":           resource.Name,
		"schema_version": resource.SchemaVersion,
		"base_url":       resource.BaseURL,
		"has_access_key": resource.HasAccessKey,
		"capabilities":   resource.Capabilities,
		"config":         resource.Config,
		"status":         resource.Status,
		"created_by":     resource.CreatedBy.String(),
		"created_at":     resource.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":     resource.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func parsePlatformResourceSubPath(path string) (uuid.UUID, string, error) {
	if !strings.HasPrefix(path, platformResourceAdminPathPrefix) {
		return uuid.Nil, "", errors.New("unsupported path")
	}

	raw := strings.Trim(strings.TrimPrefix(path, platformResourceAdminPathPrefix), "/")
	if raw == "" {
		return uuid.Nil, "", errors.New("resource id is required")
	}

	parts := strings.Split(raw, "/")
	if len(parts) != 1 && len(parts) != 3 {
		return uuid.Nil, "", errors.New("unsupported resource action")
	}

	resourceID, err := uuid.Parse(strings.TrimSpace(parts[0]))
	if err != nil {
		return uuid.Nil, "", err
	}

	if len(parts) == 1 {
		return resourceID, "", nil
	}
	if parts[1] == "access-key" && parts[2] == "reveal" {
		return resourceID, revealAccessKeyAction, nil
	}

	return uuid.Nil, "", errors.New("unsupported resource action")
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
