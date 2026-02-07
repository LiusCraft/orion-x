package tools

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
)

// ToolLoader 工具加载器接口
// 负责从特定来源（本地、MCP服务器等）加载工具
type ToolLoader interface {
	// Load 从加载器加载工具
	Load(ctx context.Context) ([]tool.BaseTool, error)

	// Name 返回加载器名称
	Name() string

	// Close 清理资源（如关闭 MCP 客户端连接）
	Close() error
}
