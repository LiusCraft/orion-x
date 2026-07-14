package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/store"
)

type ModelHandler struct {
	models *store.AIModelStore
}

func NewModelHandler(models *store.AIModelStore) *ModelHandler {
	return &ModelHandler{models: models}
}

// GET /api/models/types
func (h *ModelHandler) Types(c *gin.Context) {
	c.JSON(http.StatusOK, store.AllModelTypes())
}

// GET /api/models?type=text
func (h *ModelHandler) List(c *gin.Context) {
	modelType := store.ModelType(c.Query("type"))
	list, err := h.models.List(middleware.UserID(c), modelType, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// GET /api/models/:id
func (h *ModelHandler) Get(c *gin.Context) {
	m, err := h.models.GetByID(c.Param("id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, m)
}

type createModelRequest struct {
	ProviderID string            `json:"provider_id" binding:"required"`
	Name       string            `json:"name" binding:"required"`
	Type       store.ModelType   `json:"type" binding:"required"`
	BaseURL    string            `json:"base_url"`
	ModelID    string            `json:"model_id" binding:"required"`
	Extra      datatypes.JSONMap `json:"extra"`
}

// POST /api/models
func (h *ModelHandler) Create(c *gin.Context) {
	var req createModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID := middleware.UserID(c)
	m, err := h.models.Create(req.ProviderID, req.Name, req.Type, req.BaseURL, req.ModelID, userID, req.Extra)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, m)
}

type updateModelRequest struct {
	Name    string            `json:"name"`
	BaseURL string            `json:"base_url"`
	ModelID string            `json:"model_id"`
	Extra   datatypes.JSONMap `json:"extra"`
}

// PUT /api/models/:id
func (h *ModelHandler) Update(c *gin.Context) {
	m, err := h.models.GetByID(c.Param("id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if m.IsSystem || m.Creator != middleware.UserID(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	var req updateModelRequest
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
	if req.ModelID != "" {
		updates["model_id"] = req.ModelID
	}
	if req.Extra != nil {
		updates["extra"] = req.Extra
	}

	updated, err := h.models.Update(m.ID, updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

// DELETE /api/models/:id
func (h *ModelHandler) Delete(c *gin.Context) {
	m, err := h.models.GetByID(c.Param("id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if m.Creator != middleware.UserID(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	if err := h.models.Delete(m.ID); err != nil {
		if errors.Is(err, store.ErrSystemRecord) {
			c.JSON(http.StatusForbidden, gin.H{"error": "cannot delete system model"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
