package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/liuscraft/orion-x/internal/logging"
)

// LocalLoader 本地工具加载器
type localLoader struct {
	tools []tool.BaseTool
}

// NewLocalLoader 创建新的本地工具加载器
func NewLocalLoader() ToolLoader {
	return &localLoader{
		tools: make([]tool.BaseTool, 0),
	}
}

func (l *localLoader) Load(ctx context.Context) ([]tool.BaseTool, error) {
	logging.Infof("[ToolLoader] Loading local tools...")
	tools := make([]tool.BaseTool, 0, 6)
	appendTool := func(name string, t tool.BaseTool, err error) error {
		if err != nil {
			return fmt.Errorf("%s tool: %w", name, err)
		}
		tools = append(tools, t)
		logging.Infof("[ToolLoader]   Loaded tool: %s", name)
		return nil
	}

	getTimeTool, err := utils.InferTool("getTime", "获取当前时间", func(_ context.Context, _ struct{}) (map[string]interface{}, error) {
		now := time.Now()
		result := map[string]interface{}{
			"current":   now.Format("2006-01-02 15:04:05"),
			"weekday":   now.Weekday().String(),
			"timestamp": now.Unix(),
		}
		logging.Infof("[Tool] getTime 执行完成，结果: %v", result)
		return result, nil
	})
	if err := appendTool("getTime", getTimeTool, err); err != nil {
		return nil, err
	}

	l.tools = tools
	logging.Infof("[ToolLoader] Local tools loaded: %d tools", len(tools))
	return tools, nil
}

func (l *localLoader) Name() string {
	return "local"
}

func (l *localLoader) Close() error {
	return nil
}
