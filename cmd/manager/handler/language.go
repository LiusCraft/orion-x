package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/internal/store"
)

type LanguageHandler struct {
	languages *store.LanguageStore
}

func NewLanguageHandler(languages *store.LanguageStore) *LanguageHandler {
	return &LanguageHandler{languages: languages}
}

// GET /api/languages [?parent_code=zh]
func (h *LanguageHandler) List(c *gin.Context) {
	list, err := h.languages.List(c.Query("parent_code"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// GET /api/languages/:code
func (h *LanguageHandler) Get(c *gin.Context) {
	lang, err := h.languages.GetByCode(c.Param("code"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, lang)
}

// --- Internal admin handlers ---

type createLanguageRequest struct {
	Code       string  `json:"code" binding:"required"`
	Name       string  `json:"name" binding:"required"`
	ParentCode *string `json:"parent_code"`
}

// POST /internal/languages
func (h *LanguageHandler) AdminCreate(c *gin.Context) {
	var req createLanguageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	lang, err := h.languages.Create(store.CreateLanguageParams{
		Code:       req.Code,
		Name:       req.Name,
		ParentCode: req.ParentCode,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, lang)
}

// PUT /internal/languages/:code
func (h *LanguageHandler) AdminUpdate(c *gin.Context) {
	var updates map[string]any
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	lang, err := h.languages.Update(c.Param("code"), updates)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, lang)
}

// DELETE /internal/languages/:code
func (h *LanguageHandler) AdminDelete(c *gin.Context) {
	if err := h.languages.Delete(c.Param("code")); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
