package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/language"
)

type LanguageHandler struct{}

func NewLanguageHandler() *LanguageHandler {
	return &LanguageHandler{}
}

// GET /api/languages
func (h *LanguageHandler) List(c *gin.Context) {
	c.JSON(http.StatusOK, language.All())
}

// GET /api/languages/:code
func (h *LanguageHandler) Get(c *gin.Context) {
	info := language.Get(language.Code(c.Param("code")))
	if info == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, info)
}
