package handler

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/store"
)

type DataMemoryHandler struct {
	memStore    *store.MemoryEntryStore
	deviceStore *store.DeviceStore
	botStore    *store.VoicebotStore
}

func NewDataMemoryHandler(memStore *store.MemoryEntryStore, deviceStore *store.DeviceStore, botStore *store.VoicebotStore) *DataMemoryHandler {
	return &DataMemoryHandler{
		memStore:    memStore,
		deviceStore: deviceStore,
		botStore:    botStore,
	}
}

// ── 第三层：条目 ──

type entryItem struct {
	ID        string `json:"id"`
	Target    string `json:"target"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type listEntriesResponse struct {
	Entries  []entryItem `json:"entries"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

// ListEntries  GET /api/data/memory/devices/:device_id/entries
func (h *DataMemoryHandler) ListEntries(c *gin.Context) {
	userID := c.GetString("userID")
	deviceID := c.Param("device_id")

	if !h.checkDeviceOwnership(c, deviceID, userID) {
		return
	}

	page, pageSize := parsePageParams(c)
	target := c.Query("target")
	search := c.Query("q")

	entries, total, err := h.memStore.ListByDevicePaginated(deviceID, target, search, page, pageSize)
	if err != nil {
		logging.Errorf("DataMemory ListEntries device=%s: %v", deviceID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询记忆失败"})
		return
	}

	items := make([]entryItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, entryItem{
			ID:        e.ID,
			Target:    e.Target,
			Content:   e.Content,
			CreatedAt: e.CreatedAt.Format("2006-01-02 15:04"),
			UpdatedAt: e.UpdatedAt.Format("2006-01-02 15:04"),
		})
	}

	c.JSON(http.StatusOK, listEntriesResponse{
		Entries:  items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// ── 第二层：设备 ──

type deviceUsage struct {
	Memory struct{ Used, Limit int } `json:"memory"`
	User   struct{ Used, Limit int } `json:"user"`
}

type deviceItem struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Total int64       `json:"total"`
	Usage deviceUsage `json:"usage"`
}

type listDevicesResponse struct {
	Devices  []deviceItem `json:"devices"`
	Total    int64        `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
}

// ListDevices  GET /api/data/memory/agents/:agent_id/devices?q=xxx
func (h *DataMemoryHandler) ListDevices(c *gin.Context) {
	userID := c.GetString("userID")
	agentID := c.Param("agent_id")
	search := c.Query("q")

	// 校验 owner
	bot, err := h.botStore.GetByID(agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "智能体不存在"})
		return
	}
	if bot.OwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权访问"})
		return
	}

	devs, err := h.deviceStore.ListByVoicebot(agentID)
	if err != nil {
		logging.Errorf("DataMemory ListDevices agent=%s: %v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询设备失败"})
		return
	}

	// 按名称过滤
	if search != "" {
		q := search
		filtered := make([]store.Device, 0, len(devs))
		for _, d := range devs {
			if containsFold(d.ID, q) || containsFold(d.Name, q) {
				filtered = append(filtered, d)
			}
		}
		devs = filtered
	}

	// 排序
	sort.Slice(devs, func(i, j int) bool {
		if devs[i].Name != devs[j].Name {
			return devs[i].Name < devs[j].Name
		}
		return devs[i].ID < devs[j].ID
	})

	// 分页
	page, pageSize := parsePageParams(c)
	total := int64(len(devs))
	start := (page - 1) * pageSize
	if start < 0 {
		start = 0
	}
	if start >= len(devs) {
		c.JSON(http.StatusOK, listDevicesResponse{
			Devices:  []deviceItem{},
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		})
		return
	}
	end := start + pageSize
	if end > len(devs) {
		end = len(devs)
	}
	pagedDevs := devs[start:end]

	// 收集 device ID 查条目数和用量
	deviceIDs := make([]string, len(pagedDevs))
	for i, d := range pagedDevs {
		deviceIDs[i] = d.ID
	}
	targetFilter := c.Query("target")
	// 统计条目数时不使用内容过滤器（搜索设备名时内容查询会误过滤）
	entries, err := h.memStore.ListByDevices(deviceIDs, targetFilter, "")
	if err != nil {
		entries = nil
	}

	entryCount := make(map[string]int64)
	memUsed := make(map[string]int)
	usrUsed := make(map[string]int)
	for _, e := range entries {
		entryCount[e.DeviceID]++
		if e.Target == "user" {
			usrUsed[e.DeviceID] += len([]rune(e.Content))
		} else {
			memUsed[e.DeviceID] += len([]rune(e.Content))
		}
	}

	items := make([]deviceItem, 0, len(pagedDevs))
	for _, d := range pagedDevs {
		cnt := entryCount[d.ID]
		items = append(items, deviceItem{
			ID:    d.ID,
			Name:  d.Name,
			Total: cnt,
			Usage: deviceUsage{
				Memory: struct{ Used, Limit int }{Used: memUsed[d.ID], Limit: defaultMemoryCharLimit},
				User:   struct{ Used, Limit int }{Used: usrUsed[d.ID], Limit: defaultUserCharLimit},
			},
		})
	}

	c.JSON(http.StatusOK, listDevicesResponse{
		Devices:  items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// ── 第一层：智能体 ──

type agentItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DeviceCount int    `json:"device_count"`
	Total       int64  `json:"total"`
}

type listAgentsResponse struct {
	Agents   []agentItem `json:"agents"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

// ListAgents  GET /api/data/memory/agents?q=xxx
func (h *DataMemoryHandler) ListAgents(c *gin.Context) {
	userID := c.GetString("userID")
	search := c.Query("q")

	bots, err := h.botStore.ListByOwner(userID)
	if err != nil {
		logging.Errorf("DataMemory ListAgents owner=%s: %v", userID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询智能体列表失败"})
		return
	}

	// 按名称过滤
	if search != "" {
		q := search
		filtered := make([]store.Voicebot, 0, len(bots))
		for _, b := range bots {
			if containsFold(b.Name, q) {
				filtered = append(filtered, b)
			}
		}
		bots = filtered
	}

	sort.Slice(bots, func(i, j int) bool {
		return bots[i].Name < bots[j].Name
	})

	// 收集每个 voicebot 下的设备数 + 记忆数。无设备的智能体不属于记忆库列表。
	allItems := make([]agentItem, 0, len(bots))
	for _, b := range bots {
		devs, err := h.deviceStore.ListByVoicebot(b.ID)
		if err != nil {
			continue
		}
		if len(devs) == 0 {
			continue
		}
		deviceIDs := make([]string, len(devs))
		for i, d := range devs {
			deviceIDs[i] = d.ID
		}
		cnt, _ := h.memStore.CountByDevices(deviceIDs, "", "")
		allItems = append(allItems, agentItem{
			ID:          b.ID,
			Name:        b.Name,
			DeviceCount: len(devs),
			Total:       cnt,
		})
	}

	page, pageSize := parsePageParams(c)
	total := int64(len(allItems))
	start := (page - 1) * pageSize
	if start < 0 {
		start = 0
	}
	if start >= len(allItems) {
		c.JSON(http.StatusOK, listAgentsResponse{
			Agents:   []agentItem{},
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		})
		return
	}
	end := start + pageSize
	if end > len(allItems) {
		end = len(allItems)
	}

	c.JSON(http.StatusOK, listAgentsResponse{
		Agents:   allItems[start:end],
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// ── 删除条目 ──

// DeleteMemory DELETE /api/data/memory/:id
func (h *DataMemoryHandler) DeleteMemory(c *gin.Context) {
	userID := c.GetString("userID")
	memID := c.Param("id")

	entry, err := h.memStore.GetByID(memID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "记忆条目不存在"})
		return
	}

	if !h.checkDeviceOwnership(c, entry.DeviceID, userID) {
		return
	}

	if err := h.memStore.DeleteByID(memID); err != nil {
		logging.Errorf("DataMemory DeleteMemory id=%s: %v", memID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ── 内部辅助 ──

func (h *DataMemoryHandler) checkDeviceOwnership(c *gin.Context, deviceID, userID string) bool {
	dev, err := h.deviceStore.GetByID(deviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "设备不存在"})
		return false
	}
	bot, err := h.botStore.GetByID(dev.VoicebotID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "关联智能体不存在"})
		return false
	}
	if bot.OwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权访问"})
		return false
	}
	return true
}

func parsePageParams(c *gin.Context) (page, pageSize int) {
	page = 1
	pageSize = 20
	if p := c.Query("page"); p != "" {
		if v, err := parseInt(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if v, err := parseInt(ps); err == nil && v > 0 && v <= 100 {
			pageSize = v
		}
	}
	return
}

func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
