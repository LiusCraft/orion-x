package xiaozhi

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/tools"
	"github.com/liuscraft/orion-x/internal/channels/xiaozhi/wsproto"
)

const (
	mcpInitializeID  = 1
	mcpToolsListID   = 2
	mcpFirstCallID   = 10 // IDs below this are reserved for handshake
	mcpCallTimeout   = 30 * time.Second
)

// deviceMCPClient manages the server-side of the device-MCP handshake:
// the server acts as an MCP client, the ESP32/client acts as an MCP server.
type deviceMCPClient struct {
	conn      safeWriter
	sessionID string
	registry  *tools.Registry

	mu      sync.RWMutex
	ready   bool
	nextID  int
	pending map[int]chan json.RawMessage
}

func newDeviceMCPClient(conn safeWriter, sessionID string, registry *tools.Registry) *deviceMCPClient {
	return &deviceMCPClient{
		conn:      conn,
		sessionID: sessionID,
		registry:  registry,
		nextID:    mcpFirstCallID,
		pending:   make(map[int]chan json.RawMessage),
	}
}

// Initialize sends the MCP initialize handshake to the client device.
func (c *deviceMCPClient) Initialize(ctx context.Context) error {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      mcpInitializeID,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "orion-x", "version": "1.0.0"},
		},
	}
	return c.conn.WriteJSON(wsproto.NewMCPMessage(c.sessionID, payload))
}

// HandleMessage processes an incoming MCP payload from the client device.
func (c *deviceMCPClient) HandleMessage(ctx context.Context, payload json.RawMessage) {
	var env struct {
		ID     *int            `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
		Method string `json:"method"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		logging.Warnf("xiaozhi/device_mcp[%s]: invalid payload: %v", c.sessionID, err)
		return
	}

	if env.ID == nil {
		return
	}
	id := *env.ID

	if env.Error != nil {
		c.resolvePending(id, nil)
		logging.Warnf("xiaozhi/device_mcp[%s]: error id=%d: %s", c.sessionID, id, env.Error.Message)
		return
	}

	switch id {
	case mcpInitializeID:
		logging.Infof("xiaozhi/device_mcp[%s]: initialize ok, requesting tools/list", c.sessionID)
		_ = c.sendToolsList(ctx)

	case mcpToolsListID:
		c.handleToolsList(ctx, env.Result)

	default:
		c.resolvePending(id, env.Result)
	}
}

func (c *deviceMCPClient) sendToolsList(ctx context.Context) error {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      mcpToolsListID,
		"method":  "tools/list",
	}
	return c.conn.WriteJSON(wsproto.NewMCPMessage(c.sessionID, payload))
}

func (c *deviceMCPClient) handleToolsList(ctx context.Context, result json.RawMessage) {
	var body struct {
		Tools      []mcpToolDef `json:"tools"`
		NextCursor string       `json:"nextCursor"`
	}
	if err := json.Unmarshal(result, &body); err != nil {
		logging.Warnf("xiaozhi/device_mcp[%s]: bad tools/list result: %v", c.sessionID, err)
		return
	}

	logging.Infof("xiaozhi/device_mcp[%s]: discovered %d device tools", c.sessionID, len(body.Tools))

	for _, t := range body.Tools {
		spec := c.specFor(t)
		c.registry.Add(spec)
	}

	if body.NextCursor != "" {
		_ = c.sendToolsListContinue(ctx, body.NextCursor)
	} else {
		c.mu.Lock()
		c.ready = true
		c.mu.Unlock()
	}
}

func (c *deviceMCPClient) sendToolsListContinue(ctx context.Context, cursor string) error {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      mcpToolsListID,
		"method":  "tools/list",
		"params":  map[string]any{"cursor": cursor},
	}
	return c.conn.WriteJSON(wsproto.NewMCPMessage(c.sessionID, payload))
}

type mcpToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func (c *deviceMCPClient) specFor(t mcpToolDef) tools.Spec {
	schema := t.InputSchema
	if schema == nil {
		schema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	toolName := "device_mcp__" + t.Name
	originalName := t.Name
	client := c

	return tools.Spec{
		Name:        toolName,
		Description: t.Description,
		Parameters:  schema,
		Execute: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return client.callTool(ctx, originalName, args)
		},
	}
}

func (c *deviceMCPClient) callTool(ctx context.Context, name string, args json.RawMessage) (tools.Result, error) {
	c.mu.Lock()
	if !c.ready {
		c.mu.Unlock()
		return tools.Result{Error: fmt.Errorf("device MCP not ready")}, nil
	}
	id := c.nextID
	c.nextID++
	ch := make(chan json.RawMessage, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	var argMap any
	if len(args) > 0 {
		_ = json.Unmarshal(args, &argMap)
	}
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params":  map[string]any{"name": name, "arguments": argMap},
	}
	if err := c.conn.WriteJSON(wsproto.NewMCPMessage(c.sessionID, payload)); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return tools.Result{Error: err}, nil
	}

	timer := time.NewTimer(mcpCallTimeout)
	defer timer.Stop()
	select {
	case raw, ok := <-ch:
		if !ok || raw == nil {
			return tools.Result{Error: fmt.Errorf("device MCP call failed (id=%d)", id)}, nil
		}
		var content struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(raw, &content); err == nil && len(content.Content) > 0 {
			return tools.Result{Output: content.Content[0].Text}, nil
		}
		return tools.Result{Output: string(raw)}, nil
	case <-timer.C:
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return tools.Result{Error: fmt.Errorf("device MCP call timed out (id=%d)", id)}, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return tools.Result{Error: ctx.Err()}, nil
	}
}

func (c *deviceMCPClient) resolvePending(id int, result json.RawMessage) {
	c.mu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if ok {
		ch <- result
	}
}
