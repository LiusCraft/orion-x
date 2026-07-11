package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/cmd/manager/middleware"
	"github.com/liuscraft/orion-x/internal/store"
	"github.com/liuscraft/orion-x/internal/tools"
)

type MCPHandler struct {
	markets   *store.MCPMarketStore
	servers   *store.MCPServerStore
	bindings  *store.VoicebotMCPBindingStore
	voicebots *store.VoicebotStore
}

func NewMCPHandler(markets *store.MCPMarketStore, servers *store.MCPServerStore, bindings *store.VoicebotMCPBindingStore, voicebots *store.VoicebotStore) *MCPHandler {
	return &MCPHandler{markets: markets, servers: servers, bindings: bindings, voicebots: voicebots}
}

// ── Market ──

// GET /api/mcp/market
func (h *MCPHandler) ListMarket(c *gin.Context) {
	list, err := h.markets.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// ── MCP Server CRUD ──

// GET /api/mcp/servers
func (h *MCPHandler) ListServers(c *gin.Context) {
	list, err := h.servers.ListByOwner(middleware.UserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// GET /api/mcp/servers/:serverID
func (h *MCPHandler) GetServer(c *gin.Context) {
	s, err := h.servers.GetAccessibleByID(c.Param("serverID"), middleware.UserID(c))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s)
}

func stringFromConfig(cfg datatypes.JSONMap, key string) string {
	v, _ := cfg[key].(string)
	return v
}

func stringSliceFromConfig(cfg datatypes.JSONMap, key string) pq.StringArray {
	raw, ok := cfg[key]
	if !ok {
		return nil
	}
	switch vals := raw.(type) {
	case []string:
		return pq.StringArray(vals)
	case []any:
		out := make(pq.StringArray, 0, len(vals))
		for _, v := range vals {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func jsonMapFromConfig(cfg datatypes.JSONMap, key string) datatypes.JSONMap {
	raw, ok := cfg[key]
	if !ok {
		return nil
	}
	switch vals := raw.(type) {
	case datatypes.JSONMap:
		return vals
	case map[string]any:
		return datatypes.JSONMap(vals)
	case map[string]string:
		out := datatypes.JSONMap{}
		for k, v := range vals {
			out[k] = v
		}
		return out
	default:
		return nil
	}
}

func intFromConfig(cfg datatypes.JSONMap, key string) int {
	raw, ok := cfg[key]
	if !ok {
		return 0
	}
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	default:
		return 0
	}
}

type createServerRequest struct {
	MarketID    string            `json:"market_id,omitempty"` // install from market
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Icon        string            `json:"icon,omitempty"`
	Tags        pq.StringArray    `json:"tags,omitempty"`
	Transport   string            `json:"transport,omitempty"`
	Command     string            `json:"command,omitempty"`
	Args        pq.StringArray    `json:"args,omitempty"`
	Env         datatypes.JSONMap `json:"env,omitempty"`
	CWD         string            `json:"cwd,omitempty"`
	Endpoint    string            `json:"endpoint,omitempty"`
	Headers     datatypes.JSONMap `json:"headers,omitempty"`
	ToolList    pq.StringArray    `json:"tool_name_list,omitempty"`
	TimeoutMs   int               `json:"timeout_ms,omitempty"`
}

// POST /api/mcp/servers
func (h *MCPHandler) CreateServer(c *gin.Context) {
	var req createServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := middleware.UserID(c)

	// Install from market
	if req.MarketID != "" {
		market, err := h.markets.GetByID(req.MarketID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "market entry not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		cfg := market.Config
		transportRaw := stringFromConfig(cfg, "transport")
		if transportRaw == "" {
			transportRaw = "streamable"
		}

		server, err := h.servers.Create(store.CreateMCPServerParams{
			OwnerID:      userID,
			MarketID:     &market.ID,
			Name:         market.Name,
			Description:  market.Description,
			Icon:         market.Icon,
			Tags:         market.Tags,
			Transport:    store.MCPTransport(transportRaw),
			Command:      stringFromConfig(cfg, "command"),
			Args:         stringSliceFromConfig(cfg, "args"),
			Env:          jsonMapFromConfig(cfg, "env"),
			CWD:          stringFromConfig(cfg, "cwd"),
			Endpoint:     stringFromConfig(cfg, "endpoint"),
			Headers:      jsonMapFromConfig(cfg, "headers"),
			ToolNameList: stringSliceFromConfig(cfg, "tool_name_list"),
			TimeoutMs:    intFromConfig(cfg, "timeout_ms"),
			Creator:      userID,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, server)
		return
	}

	// Create private MCP server
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	transport := store.MCPTransport(req.Transport)
	if transport == "" {
		transport = store.MCPTransportStreamable
	}
	if !validMCPTransport(transport) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid transport"})
		return
	}
	if transport == store.MCPTransportStdio {
		c.JSON(http.StatusBadRequest, gin.H{"error": "stdio 传输协议仅系统管理员可用"})
		return
	}
	if err := validateMCPConnectionFields(transport, req.Command, req.Endpoint); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	server, err := h.servers.Create(store.CreateMCPServerParams{
		OwnerID:      userID,
		Name:         req.Name,
		Description:  req.Description,
		Icon:         req.Icon,
		Tags:         req.Tags,
		Transport:    transport,
		Command:      req.Command,
		Args:         req.Args,
		Env:          req.Env,
		CWD:          req.CWD,
		Endpoint:     req.Endpoint,
		Headers:      req.Headers,
		ToolNameList: req.ToolList,
		TimeoutMs:    req.TimeoutMs,
		Creator:      userID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, server)
}

type updateServerRequest struct {
	Name         *string            `json:"name,omitempty"`
	Description  *string            `json:"description,omitempty"`
	Transport    *string            `json:"transport,omitempty"`
	Command      *string            `json:"command,omitempty"`
	Args         *pq.StringArray    `json:"args,omitempty"`
	Env          *datatypes.JSONMap `json:"env,omitempty"`
	CWD          *string            `json:"cwd,omitempty"`
	Endpoint     *string            `json:"endpoint,omitempty"`
	Headers      *datatypes.JSONMap `json:"headers,omitempty"`
	ToolNameList *pq.StringArray    `json:"tool_name_list,omitempty"`
	TimeoutMs    *int               `json:"timeout_ms,omitempty"`
}

// PUT /api/mcp/servers/:serverID
func (h *MCPHandler) UpdateServer(c *gin.Context) {
	serverID := c.Param("serverID")
	s, err := h.servers.GetAccessibleByID(serverID, middleware.UserID(c))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if s.OwnerID != middleware.UserID(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	var req updateServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	nextTransport := s.Transport
	if req.Transport != nil {
		nextTransport = store.MCPTransport(*req.Transport)
	}
	nextCommand := s.Command
	if req.Command != nil {
		nextCommand = *req.Command
	}
	nextEndpoint := s.Endpoint
	if req.Endpoint != nil {
		nextEndpoint = *req.Endpoint
	}
	if !validMCPTransport(nextTransport) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid transport"})
		return
	}
	if nextTransport == store.MCPTransportStdio {
		c.JSON(http.StatusBadRequest, gin.H{"error": "stdio 传输协议仅系统管理员可用"})
		return
	}
	if err := validateMCPConnectionFields(nextTransport, nextCommand, nextEndpoint); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Transport != nil {
		updates["transport"] = *req.Transport
	}
	if req.Command != nil {
		updates["command"] = *req.Command
	}
	if req.Args != nil {
		updates["args"] = *req.Args
	}
	if req.Env != nil {
		updates["env"] = *req.Env
	}
	if req.CWD != nil {
		updates["cwd"] = *req.CWD
	}
	if req.Endpoint != nil {
		updates["endpoint"] = *req.Endpoint
	}
	if req.Headers != nil {
		updates["headers"] = *req.Headers
	}
	if req.ToolNameList != nil {
		updates["tool_name_list"] = *req.ToolNameList
	}
	if req.TimeoutMs != nil {
		updates["timeout_ms"] = *req.TimeoutMs
	}

	updated, err := h.servers.Update(serverID, updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

// DELETE /api/mcp/servers/:serverID
func (h *MCPHandler) DeleteServer(c *gin.Context) {
	serverID := c.Param("serverID")
	s, err := h.servers.GetAccessibleByID(serverID, middleware.UserID(c))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if s.OwnerID != middleware.UserID(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	if err := h.bindings.DeleteByServerID(serverID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.servers.Delete(serverID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// ── Voicebot MCP Binding ──

// GET /api/voicebots/:id/mcps — 返回 voicebot 绑定的 MCP servers（含启用状态）
func (h *MCPHandler) ListVoicebotMCPServers(c *gin.Context) {
	voicebotID := c.Param("id")
	if err := h.checkVoicebotOwner(c, voicebotID); err != nil {
		return
	}

	// 列出该用户的所有 MCP server
	allServers, err := h.servers.ListByOwner(middleware.UserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 查询绑定状态
	bindings, err := h.bindings.ListByVoicebot(voicebotID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	bindingMap := make(map[string]bool, len(bindings))
	for _, b := range bindings {
		bindingMap[b.MCPServerID] = b.Enabled
	}

	type responseItem struct {
		store.MCPServer
		Bound   bool `json:"bound"`
		Enabled bool `json:"enabled"`
	}
	resp := make([]responseItem, 0, len(allServers))
	for _, s := range allServers {
		enabled, bound := bindingMap[s.ID]
		resp = append(resp, responseItem{
			MCPServer: s,
			Bound:     bound,
			Enabled:   bound && enabled,
		})
	}
	c.JSON(http.StatusOK, resp)
}

type bindMCPRequest struct {
	MCPServerID string `json:"mcp_server_id" binding:"required"`
}

// POST /api/voicebots/:id/mcps — 绑定 MCP server 到 voicebot
func (h *MCPHandler) BindMCP(c *gin.Context) {
	voicebotID := c.Param("id")
	if err := h.checkVoicebotOwner(c, voicebotID); err != nil {
		return
	}

	var req bindMCPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 确认 MCP server 存在
	if _, err := h.servers.GetAccessibleByID(req.MCPServerID, middleware.UserID(c)); errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "MCP server not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 检查是否已绑定
	bound, err := h.bindings.IsBound(voicebotID, req.MCPServerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if bound {
		c.JSON(http.StatusConflict, gin.H{"error": "already bound"})
		return
	}

	if err := h.bindings.Bind(store.CreateBindingParams{
		VoicebotID:  voicebotID,
		MCPServerID: req.MCPServerID,
		Creator:     middleware.UserID(c),
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusCreated)
}

// DELETE /api/voicebots/:id/mcps/:serverID — 解绑
func (h *MCPHandler) UnbindMCP(c *gin.Context) {
	voicebotID := c.Param("id")
	if err := h.checkVoicebotOwner(c, voicebotID); err != nil {
		return
	}
	if err := h.bindings.Unbind(voicebotID, c.Param("serverID")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// PATCH /api/voicebots/:id/mcps/:serverID/toggle — 切换绑定启用状态
func (h *MCPHandler) ToggleBinding(c *gin.Context) {
	voicebotID := c.Param("id")
	if err := h.checkVoicebotOwner(c, voicebotID); err != nil {
		return
	}
	_, err := h.bindings.ToggleEnabled(voicebotID, c.Param("serverID"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "binding not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

// ── helper ──

func (h *MCPHandler) checkVoicebotOwner(c *gin.Context, voicebotID string) error {
	v, err := h.voicebots.GetByID(voicebotID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "voicebot not found"})
		return err
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return err
	}
	if v.OwnerID != middleware.UserID(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return err
	}
	return nil
}

func validMCPTransport(transport store.MCPTransport) bool {
	switch transport {
	case store.MCPTransportStdio, store.MCPTransportSSE, store.MCPTransportStreamable:
		return true
	default:
		return false
	}
}

func validateMCPConnectionFields(transport store.MCPTransport, command, endpoint string) error {
	switch transport {
	case store.MCPTransportStdio:
		if command == "" {
			return fmt.Errorf("command is required")
		}
	default:
		if endpoint == "" {
			return fmt.Errorf("endpoint is required")
		}
	}
	return nil
}

// ── Connection Test ──

type testConnectionRequest struct {
	Transport string            `json:"transport"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	CWD       string            `json:"cwd,omitempty"`
	Endpoint  string            `json:"endpoint,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	TimeoutMs int               `json:"timeout_ms,omitempty"`
}

type testConnectionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// POST /api/mcp/test-connection
func (h *MCPHandler) TestConnection(c *gin.Context) {
	var req testConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, testConnectionResponse{Message: err.Error()})
		return
	}

	timeout := req.TimeoutMs
	if timeout <= 0 {
		timeout = 10000
	}

	cfg := tools.MCPServerConfig{
		Transport: req.Transport,
		Command:   req.Command,
		Args:      req.Args,
		Env:       req.Env,
		CWD:       req.CWD,
		Endpoint:  req.Endpoint,
		Headers:   req.Headers,
		TimeoutMs: timeout,
	}

	session, err := tools.ConnectMCP(c.Request.Context(), cfg)
	if err != nil {
		c.JSON(http.StatusOK, testConnectionResponse{Success: false, Message: fmt.Sprintf("连接失败: %v", err)})
		return
	}
	session.Close()

	c.JSON(http.StatusOK, testConnectionResponse{Success: true, Message: "连接成功：MCP 握手完成"})
}
