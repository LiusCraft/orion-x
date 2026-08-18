package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/store"
)

type AgentTemplateHandler struct {
	templates *store.AgentTemplateStore
}

func NewAgentTemplateHandler(templates *store.AgentTemplateStore) *AgentTemplateHandler {
	return &AgentTemplateHandler{templates: templates}
}

// GET /api/agent-templates/system?category=&q=
func (h *AgentTemplateHandler) ListSystem(c *gin.Context) {
	list, err := h.templates.ListSystem(c.Query("category"), c.Query("q"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 确保返回空数组而不是 null
	if list == nil {
		list = []store.AgentTemplate{}
	}
	c.JSON(http.StatusOK, list)
}

// GET /api/agent-templates/:id
func (h *AgentTemplateHandler) Get(c *gin.Context) {
	t, err := h.templates.GetByID(c.Param("id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

// POST /api/agent-templates/:id/use — 使用模板（递增计数并返回 config_json）
func (h *AgentTemplateHandler) Use(c *gin.Context) {
	t, err := h.templates.IncrementUse(c.Param("id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 解析 config_json 返回给前端用于创建 voicebot
	var cfg map[string]any
	if err := json.Unmarshal([]byte(t.ConfigJSON), &cfg); err != nil {
		cfg = map[string]any{"config_json": t.ConfigJSON}
	}
	c.JSON(http.StatusOK, gin.H{
		"template_id": t.ID,
		"name":        t.Name,
		"config":      cfg,
	})
}

// ── Internal admin routes ──

type adminCreateTemplateRequest struct {
	Name        string         `json:"name" binding:"required"`
	Description string         `json:"description"`
	Icon        string         `json:"icon"`
	Color       string         `json:"color"`
	Category    string         `json:"category" binding:"required"`
	Tags        pq.StringArray `json:"tags"`
	ConfigJSON  string         `json:"config_json"`
	IsSystem    bool           `json:"is_system"`
}

// POST /internal/agent-templates — 管理员创建系统/用户模板
func (h *AgentTemplateHandler) AdminCreate(c *gin.Context) {
	var req adminCreateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	creator := "system"
	if !req.IsSystem {
		if u := middleware.UserID(c); u != "" {
			creator = u
		}
	}

	t, err := h.templates.Create(store.CreateTemplateParams{
		Name:        req.Name,
		Description: req.Description,
		Icon:        req.Icon,
		Color:       req.Color,
		Category:    req.Category,
		Tags:        req.Tags,
		ConfigJSON:  req.ConfigJSON,
		IsSystem:    req.IsSystem,
		Creator:     creator,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, t)
}

// PUT /internal/agent-templates/:id — 管理员更新模板字段
func (h *AgentTemplateHandler) AdminUpdate(c *gin.Context) {
	var updates map[string]any
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.templates.Update(c.Param("id"), updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

// DELETE /internal/agent-templates/:id — 管理员删除模板
func (h *AgentTemplateHandler) AdminDelete(c *gin.Context) {
	if err := h.templates.Delete(c.Param("id")); err != nil {
		if errors.Is(err, store.ErrSystemRecord) {
			c.JSON(http.StatusForbidden, gin.H{"error": "cannot delete system template"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
