package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/liuscraft/orion-x/internal/logging"
)

// ExecutorAdapter 适配器，将 ToolManager 适配为 ToolExecutor 接口
// 用于向后兼容 orchestrator.ToolExecutor
type ExecutorAdapter struct {
	ctx      context.Context
	manager  ToolManager
	registry *ToolRegistry
}

// NewExecutorAdapter 创建新的执行器适配器
func NewExecutorAdapter(ctx context.Context, manager ToolManager) *ExecutorAdapter {
	if ctx == nil {
		ctx = context.Background()
	}
	return &ExecutorAdapter{
		ctx:      ctx,
		manager:  manager,
		registry: NewToolRegistry(),
	}
}

// Execute 执行工具并返回结果
// 实现 tools.ToolExecutor 接口（用于向后兼容）
func (a *ExecutorAdapter) Execute(tool string, args map[string]interface{}) (interface{}, io.Reader, error) {
	// 首先检查本地注册的工具
	result, audio, err := a.registry.Execute(tool, args)
	if err == nil {
		return result, audio, nil
	}

	if a.manager == nil {
		return nil, nil, fmt.Errorf("tool manager is not initialized")
	}

	// 尝试从 ToolManager 获取工具并执行
	loadedTool, ok := a.manager.GetTool(tool)
	if !ok {
		return nil, nil, fmt.Errorf("tool not found: %s", tool)
	}
	invokableTool, ok := loadedTool.(einotool.InvokableTool)
	if !ok {
		return nil, nil, fmt.Errorf("tool is not invokable: %s", tool)
	}

	if args == nil {
		args = map[string]interface{}{}
	}
	argumentsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal tool args for %s: %w", tool, err)
	}

	logging.Infof("ExecutorAdapter: executing tool: %s, args: %v", tool, args)
	rawResult, err := invokableTool.InvokableRun(a.ctx, string(argumentsJSON))
	if err != nil {
		return nil, nil, fmt.Errorf("invoke tool %s: %w", tool, err)
	}

	return parseToolOutput(rawResult), nil, nil
}

// RegisterTool 注册本地工具（向后兼容）
func (a *ExecutorAdapter) RegisterTool(name string, executor ToolExecutorFunc) {
	logging.Infof("ExecutorAdapter: registered local tool: %s", name)
	a.registry.RegisterTool(name, executor)
}

func parseToolOutput(raw string) interface{} {
	var parsed interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return raw
	}
	return parsed
}
