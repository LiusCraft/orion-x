package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/providertemplate"
)

const providerTemplateAdminPathPrefix = "/api/v1/admin/provider-templates/"

type ProviderTemplateHandler struct {
	service *providertemplate.Service
}

func NewProviderTemplateHandler(service *providertemplate.Service) *ProviderTemplateHandler {
	return &ProviderTemplateHandler{service: service}
}

func (h *ProviderTemplateHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req providerTemplateCreateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "invalid request body",
		})
		return
	}

	template, err := h.service.Create(r.Context(), principal.UserID, providertemplate.CreateInput{
		Category: req.Category,
		Provider: req.Provider,
		Status:   req.Status,
		Version:  req.Version,
		Fields:   req.Fields,
	})
	if err != nil {
		writeProviderTemplateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    "OK",
		"message": "",
		"data":    providerTemplateDTO(template),
	})
}

func (h *ProviderTemplateHandler) List(w http.ResponseWriter, r *http.Request) {
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

	templates, err := h.service.List(r.Context(), providertemplate.ListInput{
		Category: r.URL.Query().Get("category"),
		Provider: r.URL.Query().Get("provider"),
		Status:   r.URL.Query().Get("status"),
	})
	if err != nil {
		writeProviderTemplateError(w, err)
		return
	}

	items := make([]map[string]any, 0, len(templates))
	for _, item := range templates {
		items = append(items, providerTemplateDTO(item))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    "OK",
		"message": "",
		"data": map[string]any{
			"items": items,
		},
	})
}

func (h *ProviderTemplateHandler) ByID(w http.ResponseWriter, r *http.Request) {
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

	templateID, err := extractProviderTemplateID(r.URL.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "invalid template id",
		})
		return
	}

	switch r.Method {
	case http.MethodPatch:
		h.patchByID(w, r, templateID)
	case http.MethodDelete:
		h.deleteByID(w, r, templateID)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *ProviderTemplateHandler) patchByID(w http.ResponseWriter, r *http.Request, templateID uuid.UUID) {
	var req providerTemplateUpdateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": "invalid request body",
		})
		return
	}

	updated, err := h.service.Update(r.Context(), templateID, providertemplate.UpdateInput{
		Category: req.Category,
		Provider: req.Provider,
		Status:   req.Status,
		Version:  req.Version,
		Fields:   req.Fields,
	})
	if err != nil {
		writeProviderTemplateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    "OK",
		"message": "",
		"data":    providerTemplateDTO(updated),
	})
}

func (h *ProviderTemplateHandler) deleteByID(w http.ResponseWriter, r *http.Request, templateID uuid.UUID) {
	if err := h.service.Delete(r.Context(), templateID); err != nil {
		writeProviderTemplateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    "OK",
		"message": "",
		"data": map[string]any{
			"id": templateID.String(),
		},
	})
}

type providerTemplateCreateRequest struct {
	Category string                   `json:"category"`
	Provider string                   `json:"provider"`
	Status   string                   `json:"status"`
	Version  int                      `json:"version"`
	Fields   []providertemplate.Field `json:"fields"`
}

type providerTemplateUpdateRequest struct {
	Category *string                   `json:"category"`
	Provider *string                   `json:"provider"`
	Status   *string                   `json:"status"`
	Version  *int                      `json:"version"`
	Fields   *[]providertemplate.Field `json:"fields"`
}

func providerTemplateDTO(template providertemplate.Template) map[string]any {
	return map[string]any{
		"id":         template.ID.String(),
		"category":   template.Category,
		"provider":   template.Provider,
		"status":     template.Status,
		"version":    template.Version,
		"fields":     template.Fields,
		"created_by": template.CreatedBy.String(),
		"created_at": template.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": template.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func extractProviderTemplateID(path string) (uuid.UUID, error) {
	if !strings.HasPrefix(path, providerTemplateAdminPathPrefix) {
		return uuid.Nil, errors.New("unsupported path")
	}
	rawID := strings.TrimSpace(strings.TrimPrefix(path, providerTemplateAdminPathPrefix))
	if rawID == "" || strings.Contains(rawID, "/") {
		return uuid.Nil, errors.New("template id is required")
	}
	templateID, err := uuid.Parse(rawID)
	if err != nil {
		return uuid.Nil, err
	}
	return templateID, nil
}

func writeProviderTemplateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, providertemplate.ErrInvalidArgument):
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "ERR_INVALID_ARGUMENT",
			"message": err.Error(),
		})
	case errors.Is(err, providertemplate.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]any{
			"code":    "ERR_CONFLICT",
			"message": "resource already exists",
		})
	case errors.Is(err, providertemplate.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{
			"code":    "ERR_NOT_FOUND",
			"message": "provider template not found",
		})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"code":    "ERR_INTERNAL",
			"message": "internal server error",
		})
	}
}
