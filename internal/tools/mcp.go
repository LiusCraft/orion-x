package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpPrefix = "mcp"

type mcpSessionData struct {
	id      string
	session *mcp.ClientSession
}

func (s *mcpSessionData) Close() error {
	return s.session.Close()
}

func loadMCPSpecs(ctx context.Context, cfg MCPServerConfig) ([]Spec, *mcpSession, error) {
	logging.Infof("[Tools] Loading MCP tools from: %s (transport: %s)", cfg.ID, cfg.Transport)

	session, err := connectMCP(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("connect MCP server: %w", err)
	}

	result, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		_ = session.Close()
		return nil, nil, fmt.Errorf("list MCP tools: %w", err)
	}

	allTools := result.Tools
	if len(cfg.ToolNameList) > 0 {
		filtered := make([]*mcp.Tool, 0, len(cfg.ToolNameList))
		for _, name := range cfg.ToolNameList {
			for _, t := range allTools {
				if t.Name == name {
					filtered = append(filtered, t)
					break
				}
			}
		}
		allTools = filtered
	}

	specs := make([]Spec, 0, len(allTools))
	for _, t := range allTools {
		prefixedName := fmt.Sprintf("%s__%s__%s", mcpPrefix, cfg.ID, t.Name)
		inputSchema, _ := t.InputSchema.(map[string]any)

		originalName := t.Name
		sess := session

		specs = append(specs, Spec{
			Name:        prefixedName,
			Description: t.Description,
			Parameters:  inputSchema,
			Execute: func(ctx context.Context, args json.RawMessage) (Result, error) {
				var argMap map[string]any
				if len(args) > 0 {
					_ = json.Unmarshal(args, &argMap)
				}
				res, err := sess.CallTool(ctx, &mcp.CallToolParams{
					Name:      originalName,
					Arguments: argMap,
				})
				if err != nil {
					return Result{Error: err}, nil
				}
				if res.IsError {
					return Result{Error: fmt.Errorf("mcp tool error: %v", res.Content)}, nil
				}
				for _, c := range res.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						return Result{Output: tc.Text}, nil
					}
				}
				return Result{Output: ""}, nil
			},
		})

		logging.Infof("[Tools]   Loaded MCP tool: %s - %s", prefixedName, t.Description)
	}

	logging.Infof("[Tools] MCP tools loaded: %d tools from server '%s'", len(specs), cfg.ID)

	mcpSess := &mcpSession{
		id:  cfg.ID,
		cli: &mcpSessionData{id: cfg.ID, session: session},
	}
	return specs, mcpSess, nil
}

func connectMCP(ctx context.Context, cfg MCPServerConfig) (*mcp.ClientSession, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "orion-x", Version: "1.0.0"}, nil)
	transport, err := buildMCPTransport(cfg)
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(cfg.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return client.Connect(connectCtx, transport, nil)
}

func buildMCPTransport(cfg MCPServerConfig) (mcp.Transport, error) {
	transportType := strings.ToLower(strings.TrimSpace(cfg.Transport))
	if transportType == "" {
		transportType = "sse"
	}

	switch transportType {
	case "stdio":
		command := strings.TrimSpace(cfg.Command)
		if command == "" {
			return nil, errors.New("mcp stdio command is required")
		}
		cmd := exec.Command(command, cfg.Args...)
		cmd.Env = append(cmd.Environ(), flattenMCPEnv(cfg.Env)...)
		if cwd := strings.TrimSpace(cfg.CWD); cwd != "" {
			cmd.Dir = cwd
		}
		return &mcp.CommandTransport{Command: cmd}, nil
	case "sse", "streamable", "stream_http":
		endpoint := strings.TrimSpace(cfg.Endpoint)
		if endpoint == "" {
			return nil, errors.New("mcp endpoint is required")
		}
		return &mcp.StreamableClientTransport{Endpoint: endpoint}, nil
	default:
		return nil, fmt.Errorf("unsupported mcp transport: %s", cfg.Transport)
	}
}

func flattenMCPEnv(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	flattened := make([]string, 0, len(env))
	for _, key := range keys {
		flattened = append(flattened, fmt.Sprintf("%s=%s", key, env[key]))
	}
	return flattened
}

func ParseMCPToolName(name string) (serverID, toolName string, ok bool) {
	prefix := mcpPrefix + "__"
	if !strings.HasPrefix(name, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(name, prefix)
	parts := strings.SplitN(rest, "__", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
