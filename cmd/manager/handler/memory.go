package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

const (
	defaultMemoryCharLimit = 2200
	defaultUserCharLimit   = 1375
)

type MemoryHandler struct {
	store *store.MemoryEntryStore
}

func NewMemoryHandler(s *store.MemoryEntryStore) *MemoryHandler {
	return &MemoryHandler{store: s}
}

// GetMemory GET /internal/devices/:device_id/memory
func (h *MemoryHandler) GetMemory(c *gin.Context) {
	deviceID := c.Param("device_id")
	entries, err := h.store.ListByDevice(deviceID)
	if err != nil {
		logging.Errorf("GetMemory device=%s: %v", deviceID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var memEntries, userEntries []gin.H
	var memUsed, userUsed int
	for _, e := range entries {
		if e.Target == "user" {
			userUsed += len(e.Content)
			userEntries = append(userEntries, gin.H{"content": e.Content, "created_at": e.CreatedAt})
		} else {
			memUsed += len(e.Content)
			memEntries = append(memEntries, gin.H{"content": e.Content, "created_at": e.CreatedAt})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"entries": gin.H{
			"memory": memEntries,
			"user":   userEntries,
		},
		"usage": gin.H{
			"memory": gin.H{"used": memUsed, "limit": defaultMemoryCharLimit},
			"user":   gin.H{"used": userUsed, "limit": defaultUserCharLimit},
		},
	})
}

type memoryPutBody struct {
	Entries []struct {
		Target  string `json:"target" binding:"required"`
		Content string `json:"content" binding:"required"`
	} `json:"entries" binding:"required"`
}

// PutMemory PUT /internal/devices/:device_id/memory
func (h *MemoryHandler) PutMemory(c *gin.Context) {
	deviceID := c.Param("device_id")
	var body memoryPutBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	entries := make([]store.MemoryEntry, 0, len(body.Entries))
	for _, e := range body.Entries {
		entries = append(entries, store.MemoryEntry{
			Target:  e.Target,
			Content: e.Content,
		})
	}

	if err := h.store.ReplaceAll(deviceID, entries); err != nil {
		logging.Errorf("PutMemory device=%s: %v", deviceID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
