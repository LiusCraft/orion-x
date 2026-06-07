package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/liuscraft/orion-x/internal/logging"
)

// ToolManager 工具管理器接口
// 负责管理所有加载的工具，提供统一的工具查询和访问接口
type ToolManager interface {
	// ToolInfos 获取所有工具的 ToolInfo（用于 LLM.BindTools）
	ToolInfos() []*schema.ToolInfo

	// GetTool 获取特定工具
	GetTool(name string) (tool.BaseTool, bool)

	// Has 检查工具是否存在
	Has(name string) bool

	// Tools 获取所有加载的工具（用于 compose.NewToolsNode）
	Tools() []tool.BaseTool

	// Close 关闭所有 Loader
	Close() error
}

// toolManager 工具管理器实现
type toolManager struct {
	loaders         []ToolLoader
	tools           []tool.BaseTool
	toolByName      map[string]tool.BaseTool
	toolInfos       []*schema.ToolInfo
	toolInfosByName map[string]*schema.ToolInfo
	mu              sync.RWMutex
}

// NewToolManager 创建新的工具管理器
func NewToolManager(ctx context.Context, cfg ManagerConfig) (ToolManager, error) {
	m := &toolManager{
		loaders:         make([]ToolLoader, 0),
		tools:           make([]tool.BaseTool, 0),
		toolByName:      make(map[string]tool.BaseTool),
		toolInfos:       make([]*schema.ToolInfo, 0),
		toolInfosByName: make(map[string]*schema.ToolInfo),
	}

	// 创建并添加本地工具加载器
	localLoader := NewLocalLoader()
	m.loaders = append(m.loaders, localLoader)

	// 创建并添加 MCP 工具加载器
	for _, serverCfg := range cfg.MCPServers {
		mcpLoader, err := NewMCPLoader(serverCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create MCP loader for %s: %w", serverCfg.ID, err)
		}
		m.loaders = append(m.loaders, mcpLoader)
	}

	// 加载所有工具
	if err := m.loadAll(ctx); err != nil {
		_ = m.Close()
		return nil, err
	}

	// 输出工具总览
	logging.Infof("[ToolManager] =====================================")
	logging.Infof("[ToolManager] Total tools loaded: %d", len(m.toolInfos))
	logging.Infof("[ToolManager] Tool names:")
	for _, info := range m.toolInfos {
		logging.Infof("[ToolManager]   - %s", info.Name)
	}
	logging.Infof("[ToolManager] =====================================")

	return m, nil
}

func (m *toolManager) loadAll(ctx context.Context) error {
	for _, loader := range m.loaders {
		tools, err := loader.Load(ctx)
		if err != nil {
			return fmt.Errorf("loader %s failed: %w", loader.Name(), err)
		}

		for _, t := range tools {
			info, err := t.Info(ctx)
			if err != nil {
				return fmt.Errorf("tool info failed: %w", err)
			}
			m.addTool(t, info)
		}
	}
	return nil
}

func (m *toolManager) addTool(t tool.BaseTool, info *schema.ToolInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.tools = append(m.tools, t)
	m.toolByName[info.Name] = t
	m.toolInfos = append(m.toolInfos, info)
	m.toolInfosByName[info.Name] = info
}

func (m *toolManager) ToolInfos() []*schema.ToolInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.toolInfos
}

func (m *toolManager) GetTool(name string) (tool.BaseTool, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.toolByName[name]
	return t, ok
}

func (m *toolManager) Has(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.toolByName[name]
	return ok
}

func (m *toolManager) Tools() []tool.BaseTool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tools
}

func (m *toolManager) Close() error {
	var errs []error

	for _, loader := range m.loaders {
		if err := loader.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}
