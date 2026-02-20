package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	mcpp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	// mcpLoaderPrefix MCP 工具名称前缀
	mcpLoaderPrefix = "mcp"
)

// mcpLoader MCP 工具加载器
type mcpLoader struct {
	cfg    MCPServerConfig
	client *client.Client
	tools  []tool.BaseTool
}

// NewMCPLoader 创建新的 MCP 工具加载器
func NewMCPLoader(cfg MCPServerConfig) (ToolLoader, error) {
	if strings.TrimSpace(cfg.ID) == "" {
		return nil, errors.New("mcp server id is required")
	}
	return &mcpLoader{
		cfg:   cfg,
		tools: make([]tool.BaseTool, 0),
	}, nil
}

func (l *mcpLoader) Load(ctx context.Context) ([]tool.BaseTool, error) {
	logging.Infof("[ToolLoader] Loading MCP tools from: %s (transport: %s)", l.cfg.ID, l.cfg.Transport)

	// 创建并初始化 MCP 客户端
	cli, err := l.buildMCPClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("build MCP client: %w", err)
	}
	l.client = cli
	logging.Infof("[ToolLoader] MCP client connected successfully")

	// 获取工具列表
	toolList, err := mcpp.GetTools(ctx, &mcpp.Config{
		Cli:          cli,
		ToolNameList: l.cfg.ToolNameList,
	})
	if err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("get MCP tools: %w", err)
	}

	// 为每个工具添加前缀
	tools := make([]tool.BaseTool, 0, len(toolList))
	for _, t := range toolList {
		info, err := t.Info(ctx)
		if err != nil {
			logging.Warnf("[ToolLoader] MCP tool info failed: %v", err)
			continue
		}

		// 创建带前缀的工具包装器
		prefixedName := fmt.Sprintf("%s.%s.%s", mcpLoaderPrefix, l.cfg.ID, info.Name)
		prefixedTool := &mcpToolWrapper{
			original: t,
			name:     prefixedName,
			desc:     info.Desc,
		}

		tools = append(tools, prefixedTool)
		logging.Infof("[ToolLoader]   Loaded MCP tool: %s - %s", prefixedName, info.Desc)
	}

	l.tools = tools
	logging.Infof("[ToolLoader] MCP tools loaded: %d tools from server '%s'", len(tools), l.cfg.ID)
	return tools, nil
}

func (l *mcpLoader) Name() string {
	return fmt.Sprintf("mcp.%s", l.cfg.ID)
}

func (l *mcpLoader) Close() error {
	if l.client != nil {
		return l.client.Close()
	}
	return nil
}

// buildMCPClient 创建并初始化 MCP 客户端
func (l *mcpLoader) buildMCPClient(ctx context.Context) (*client.Client, error) {
	transportType := strings.ToLower(strings.TrimSpace(l.cfg.Transport))
	if transportType == "" {
		transportType = "sse"
	}
	timeout := time.Duration(l.cfg.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	var cli *client.Client
	var err error

	switch transportType {
	case "stdio":
		command := strings.TrimSpace(l.cfg.Command)
		if command == "" {
			return nil, errors.New("mcp stdio command is required")
		}
		env := flattenEnv(l.cfg.Env)
		cwd := strings.TrimSpace(l.cfg.CWD)
		if cwd == "" {
			cli, err = client.NewStdioMCPClient(command, env, l.cfg.Args...)
		} else {
			cli, err = client.NewStdioMCPClientWithOptions(
				command,
				env,
				l.cfg.Args,
				transport.WithCommandFunc(func(ctx context.Context, command string, env []string, args []string) (*exec.Cmd, error) {
					cmd := exec.CommandContext(ctx, command, args...)
					cmd.Env = append(os.Environ(), env...)
					cmd.Dir = cwd
					return cmd, nil
				}),
			)
		}
	case "sse":
		endpoint := strings.TrimSpace(l.cfg.Endpoint)
		if endpoint == "" {
			return nil, errors.New("mcp sse endpoint is required")
		}
		sseOptions := make([]transport.ClientOption, 0, 1)
		if len(l.cfg.Headers) > 0 {
			sseOptions = append(sseOptions, transport.WithHeaders(l.cfg.Headers))
		}
		cli, err = client.NewSSEMCPClient(endpoint, sseOptions...)
	case "streamable", "stream_http":
		endpoint := strings.TrimSpace(l.cfg.Endpoint)
		if endpoint == "" {
			return nil, errors.New("mcp streamable endpoint is required")
		}
		httpOptions := []transport.StreamableHTTPCOption{transport.WithHTTPTimeout(timeout)}
		if len(l.cfg.Headers) > 0 {
			httpOptions = append(httpOptions, transport.WithHTTPHeaders(l.cfg.Headers))
		}
		cli, err = client.NewStreamableHttpClient(endpoint, httpOptions...)
	default:
		return nil, fmt.Errorf("unsupported mcp transport: %s", l.cfg.Transport)
	}
	if err != nil {
		return nil, err
	}

	startCtx := ctx
	if transportType != "sse" {
		boundedStartCtx, cancelStart := context.WithTimeout(ctx, timeout)
		defer cancelStart()
		startCtx = boundedStartCtx
	}

	if err := cli.Start(startCtx); err != nil {
		_ = cli.Close()
		return nil, err
	}

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "orion-x",
		Version: "1.0.0",
	}

	initCtx, cancelInit := context.WithTimeout(ctx, timeout)
	defer cancelInit()

	if _, err := cli.Initialize(initCtx, initRequest); err != nil {
		_ = cli.Close()
		return nil, err
	}

	return cli, nil
}

func flattenEnv(env map[string]string) []string {
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

// mcpToolWrapper MCP 工具包装器，用于添加前缀名称
type mcpToolWrapper struct {
	original tool.BaseTool
	name     string
	desc     string
}

func (w *mcpToolWrapper) Info(ctx context.Context) (*schema.ToolInfo, error) {
	info, err := w.original.Info(ctx)
	if err != nil {
		return nil, err
	}

	// 返回带前缀的工具信息
	return &schema.ToolInfo{
		Name:        w.name,
		Desc:        w.desc,
		ParamsOneOf: info.ParamsOneOf,
		Extra:       info.Extra,
	}, nil
}

func (w *mcpToolWrapper) Bind(ctx context.Context, inputFunc func(ctx context.Context, args string) (string, error)) error {
	// 对于 MCP 工具，我们不需要绑定，因为工具执行由 mcp 包处理
	return nil
}

func (w *mcpToolWrapper) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	invokable, ok := w.original.(tool.InvokableTool)
	if !ok {
		return "", fmt.Errorf("mcp tool is not invokable: %s", w.name)
	}
	return invokable.InvokableRun(ctx, argumentsInJSON, opts...)
}

// ParseMCPToolName 解析 MCP 工具名称
// 输入: "mcp.demo.get_device_status"
// 输出: serverID="demo", toolName="get_device_status", ok=true
func ParseMCPToolName(name string) (serverID, toolName string, ok bool) {
	if !strings.HasPrefix(name, mcpLoaderPrefix+".") {
		return "", "", false
	}
	rest := strings.TrimPrefix(name, mcpLoaderPrefix+".")
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
