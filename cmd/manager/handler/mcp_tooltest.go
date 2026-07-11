package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/liuscraft/orion-x/internal/tools"
)

type toolTestRequest struct {
	Transport string            `json:"transport"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	CWD       string            `json:"cwd,omitempty"`
	Endpoint  string            `json:"endpoint,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	TimeoutMs int               `json:"timeout_ms,omitempty"`
}

type callToolRequest struct {
	toolTestRequest
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// POST /api/mcp/list-tools
func (h *MCPHandler) ListTools(c *gin.Context) {
	var req toolTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusOK, gin.H{"success": false, "message": fmt.Sprintf("连接失败: %v", err)})
		return
	}
	defer func() { _ = session.Close() }()

	result, err := session.ListTools(c.Request.Context(), &mcp.ListToolsParams{})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": fmt.Sprintf("获取工具列表失败: %v", err)})
		return
	}

	type toolDef struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		InputSchema map[string]any `json:"input_schema,omitempty"`
	}

	toolsList := make([]toolDef, 0, len(result.Tools))
	for _, t := range result.Tools {
		schema, _ := t.InputSchema.(map[string]any)
		toolsList = append(toolsList, toolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "tools": toolsList})
}

// POST /api/mcp/call-tool
func (h *MCPHandler) CallTool(c *gin.Context) {
	var req callToolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ToolName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tool_name is required"})
		return
	}

	timeout := req.TimeoutMs
	if timeout <= 0 {
		timeout = 30000
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
		c.JSON(http.StatusOK, gin.H{"success": false, "message": fmt.Sprintf("连接失败: %v", err)})
		return
	}
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(c.Request.Context(), &mcp.CallToolParams{
		Name:      req.ToolName,
		Arguments: req.Arguments,
	})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": fmt.Sprintf("调用失败: %v", err)})
		return
	}

	var output string
	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok && tc.Text != "" {
			if output != "" {
				output += "\n"
			}
			output += tc.Text
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"is_error": result.IsError,
		"output":   output,
	})
}
