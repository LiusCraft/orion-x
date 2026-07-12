package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

type TurnHandler struct {
	store *store.TurnStore
}

func NewTurnHandler(s *store.TurnStore) *TurnHandler {
	return &TurnHandler{store: s}
}

// CreateTurn POST /internal/devices/:device_id/turns
func (h *TurnHandler) CreateTurn(c *gin.Context) {
	deviceID := c.Param("device_id")
	var turn store.SessionTurn
	if err := c.ShouldBindJSON(&turn); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	turn.DeviceID = deviceID
	if err := h.store.Create(&turn); err != nil {
		logging.Errorf("CreateTurn device=%s: %v", deviceID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": turn.ID})
}

// SearchTurns GET /internal/devices/:device_id/turns?q=xxx&limit=3
func (h *TurnHandler) SearchTurns(c *gin.Context) {
	deviceID := c.Param("device_id")
	query := c.Query("q")
	if query == "" {
		h.listSessions(c, deviceID)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "3"))
	if limit <= 0 || limit > 10 {
		limit = 3
	}
	turns, err := h.store.Search(deviceID, query, limit)
	if err != nil {
		logging.Errorf("SearchTurns device=%s q=%q: %v", deviceID, query, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	seen := make(map[string]bool)
	results := make([]gin.H, 0, limit)
	for _, t := range turns {
		if seen[t.SessionID] {
			continue
		}
		seen[t.SessionID] = true
		results = append(results, gin.H{
			"session_id":   t.SessionID,
			"snippet":      truncate(t.UserText+" "+t.AssistantText, 200),
			"matched_role": "user",
			"messages": []gin.H{
				{"id": t.ID * 10, "role": "user", "content": t.UserText, "timestamp": t.CreatedAt},
				{"id": t.ID*10 + 1, "role": "assistant", "content": t.AssistantText, "timestamp": t.CreatedAt},
			},
		})
		if len(results) >= limit {
			break
		}
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// GetSessionMessages GET /internal/devices/:device_id/sessions/:session_id
func (h *TurnHandler) GetSessionMessages(c *gin.Context) {
	deviceID := c.Param("device_id")
	sessionID := c.Param("session_id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if offset < 0 {
		offset = 0
	}
	turns, err := h.store.ListBySession(deviceID, sessionID, limit, offset)
	if err != nil {
		logging.Errorf("GetSessionMessages device=%s session=%s: %v", deviceID, sessionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	messages := make([]gin.H, 0, len(turns))
	for _, t := range turns {
		messages = append(messages, gin.H{
			"id": t.ID, "role": "user", "content": t.UserText, "timestamp": t.CreatedAt,
		})
		if t.AssistantText != "" {
			messages = append(messages, gin.H{
				"id": t.ID*10 + 1, "role": "assistant", "content": t.AssistantText, "timestamp": t.EndedAt,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"messages": messages, "count": len(messages)})
}

func (h *TurnHandler) listSessions(c *gin.Context, deviceID string) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	sessions, err := h.store.ListSessions(deviceID, limit)
	if err != nil {
		logging.Errorf("ListSessions device=%s: %v", deviceID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}
