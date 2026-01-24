package tools

import (
	"fmt"
	"io"
)

// ToolExecutor 工具执行器接口
type ToolExecutor interface {
	Execute(tool string, args map[string]interface{}) (result interface{}, audio io.Reader, err error)
	RegisterTool(name string, executor ToolExecutorFunc)
}

// ToolExecutorFunc 工具执行函数
type ToolExecutorFunc func(args map[string]interface{}) (interface{}, io.Reader, error)

// ToolResult 工具执行结果
type ToolResult struct {
	Data  interface{} // 工具返回的数据
	Audio io.Reader   // 资源音频流
	Error error       // 错误信息
}

// ToolRegistry 工具注册表
type ToolRegistry struct {
	tools map[string]ToolExecutorFunc
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]ToolExecutorFunc),
	}
}

func (r *ToolRegistry) RegisterTool(name string, executor ToolExecutorFunc) {
	r.tools[name] = executor
}

func (r *ToolRegistry) Execute(tool string, args map[string]interface{}) (interface{}, io.Reader, error) {
	executor, ok := r.tools[tool]
	if !ok {
		return nil, nil, ErrToolNotFound
	}
	return executor(args)
}

// ToolExecutor 实现ToolExecutor接口
type toolExecutor struct {
	registry *ToolRegistry
}

func NewToolExecutor() ToolExecutor {
	return &toolExecutor{
		registry: NewToolRegistry(),
	}
}

func (e *toolExecutor) Execute(tool string, args map[string]interface{}) (interface{}, io.Reader, error) {
	return e.registry.Execute(tool, args)
}

func (e *toolExecutor) RegisterTool(name string, executor ToolExecutorFunc) {
	e.registry.RegisterTool(name, executor)
}

// 错误定义
var (
	ErrToolNotFound = fmt.Errorf("tool not found")
)
