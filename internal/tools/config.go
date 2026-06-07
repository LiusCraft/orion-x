package tools

// ManagerConfig 工具管理器配置
type ManagerConfig struct {
	// MCP 服务器配置
	MCPServers []MCPServerConfig `json:"mcp"`
}

// MCPServerConfig MCP 服务器配置
type MCPServerConfig struct {
	ID        string            `json:"id"`
	Transport string            `json:"transport"` // stdio | sse | streamable | stream_http
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	CWD       string            `json:"cwd"`
	Endpoint  string            `json:"endpoint"`
	Headers   map[string]string `json:"headers"`
	// ToolNameList 指定要加载的工具列表，为空则加载所有工具
	ToolNameList []string `json:"tool_name_list"`
	TimeoutMs    int      `json:"timeout_ms"`
}
