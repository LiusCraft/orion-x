package tools

import "strings"

// ManagerConfig 工具管理器配置
type ManagerConfig struct {
	// LLM 配置
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`

	// 工具类型映射（使用完整工具名）
	// MCP 工具使用完整前缀名，如 "mcp.demo.get_device_status"
	// 本地工具使用短名称，如 "getTime"
	ToolTypes map[string]string `json:"types"`

	// 动作响应模板（使用完整工具名）
	// 支持 {{key}} 形式的模板替换
	ActionResponses map[string]string `json:"action_responses"`

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

// ToolType 工具类型
type ToolType int

const (
	ToolTypeQuery  ToolType = iota // 查询类：需要LLM总结
	ToolTypeAction                 // 动作类：直接执行+播报
)

func (t ToolType) String() string {
	switch t {
	case ToolTypeQuery:
		return "query"
	case ToolTypeAction:
		return "action"
	default:
		return "unknown"
	}
}

// ParseToolType 解析工具类型字符串
func ParseToolType(value string) (ToolType, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "query":
		return ToolTypeQuery, nil
	case "action":
		return ToolTypeAction, nil
	default:
		return ToolTypeQuery, InvalidToolTypeError{Value: value}
	}
}

// InvalidToolTypeError 无效的工具类型错误
type InvalidToolTypeError struct {
	Value string
}

func (e InvalidToolTypeError) Error() string {
	return "invalid tool type: " + e.Value
}
