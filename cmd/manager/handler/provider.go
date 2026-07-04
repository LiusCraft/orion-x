package handler

import (
	"errors"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	llmprovider "github.com/liuscraft/orion-x/internal/llm/provider"
	asrprovider "github.com/liuscraft/orion-x/internal/provider/asr"
	ttsprovider "github.com/liuscraft/orion-x/internal/provider/tts"
	"github.com/liuscraft/orion-x/internal/store"
)

type ProviderHandler struct {
	providers *store.ProviderStore
}

func NewProviderHandler(providers *store.ProviderStore) *ProviderHandler {
	return &ProviderHandler{providers: providers}
}

// GET /api/providers
func (h *ProviderHandler) List(c *gin.Context) {
	list, err := h.providers.List(middleware.UserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// GET /api/providers/:id
func (h *ProviderHandler) Get(c *gin.Context) {
	p, err := h.providers.GetByID(c.Param("id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, p)
}

type createProviderRequest struct {
	Name    string            `json:"name" binding:"required"`
	Slug    string            `json:"slug" binding:"required"`
	BaseURL string            `json:"base_url" binding:"required"`
	APIKey  string            `json:"api_key"`
	Extra   datatypes.JSONMap `json:"extra"`
}

// POST /api/providers
func (h *ProviderHandler) Create(c *gin.Context) {
	var req createProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID := middleware.UserID(c)
	p, err := h.providers.Create(req.Name, req.Slug, req.BaseURL, req.APIKey, userID, req.Extra)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, p)
}

type updateProviderRequest struct {
	Name    string            `json:"name"`
	BaseURL string            `json:"base_url"`
	APIKey  string            `json:"api_key"`
	Extra   datatypes.JSONMap `json:"extra"`
}

// PUT /api/providers/:id
func (h *ProviderHandler) Update(c *gin.Context) {
	p, err := h.providers.GetByID(c.Param("id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !p.IsSystem && p.Creator != middleware.UserID(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	var req updateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]any{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.BaseURL != "" {
		updates["base_url"] = req.BaseURL
	}
	if req.APIKey != "" {
		updates["api_key_enc"] = req.APIKey
	}
	if req.Extra != nil {
		updates["extra"] = req.Extra
	}

	updated, err := h.providers.Update(p.ID, updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

// ProviderSlug 系统支持的 provider slug（从各工厂注册表动态收集）
type ProviderSlug struct {
	Slug     string `json:"slug"`
	Category string `json:"category"`
	Name     string `json:"name"`
	BaseURL  string `json:"base_url"`
}

// GET /api/providers/slugs
func (h *ProviderHandler) Slugs(c *gin.Context) {
	var slugs []ProviderSlug
	for key, meta := range asrprovider.ListRegistered() {
		slugs = append(slugs, ProviderSlug{
			Slug:     "asr/" + key,
			Category: "asr",
			Name:     meta.Name,
			BaseURL:  meta.DefaultBaseURL,
		})
	}
	for key, meta := range ttsprovider.ListRegistered() {
		slugs = append(slugs, ProviderSlug{
			Slug:     "tts/" + key,
			Category: "tts",
			Name:     meta.Name,
			BaseURL:  meta.DefaultBaseURL,
		})
	}
	for key, meta := range llmprovider.DefaultRegistry().ListRegistered() {
		slugs = append(slugs, ProviderSlug{
			Slug:     "llm/" + key,
			Category: "llm",
			Name:     meta.Name,
			BaseURL:  meta.DefaultBaseURL,
		})
	}
	sort.Slice(slugs, func(i, j int) bool { return slugs[i].Slug < slugs[j].Slug })
	c.JSON(http.StatusOK, slugs)
}

// DELETE /api/providers/:id
func (h *ProviderHandler) Delete(c *gin.Context) {
	p, err := h.providers.GetByID(c.Param("id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if p.Creator != middleware.UserID(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	if err := h.providers.Delete(p.ID); err != nil {
		if errors.Is(err, store.ErrSystemRecord) {
			c.JSON(http.StatusForbidden, gin.H{"error": "cannot delete system provider"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
