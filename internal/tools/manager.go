package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/liuscraft/orion-x/internal/logging"
)

type Manager struct {
	registry *Registry
	sessions []*mcpSession
}

type mcpSession struct {
	id  string
	cli interface{ Close() error }
}

func NewManager(ctx context.Context, cfg ManagerConfig) (*Manager, error) {
	m := &Manager{registry: NewRegistry()}

	localSpecs := LocalSpecs()
	for _, spec := range localSpecs {
		m.registry.Add(spec)
	}

	for _, serverCfg := range cfg.MCPServers {
		specs, session, err := loadMCPSpecs(ctx, serverCfg)
		if err != nil {
			m.Close()
			return nil, fmt.Errorf("failed to load MCP tools from %s: %w", serverCfg.ID, err)
		}
		m.sessions = append(m.sessions, session)
		for _, spec := range specs {
			m.registry.Add(spec)
		}
	}

	defs := m.registry.Definitions()
	logging.Infof("[Tools] =====================================")
	logging.Infof("[Tools] Total tools loaded: %d", len(defs))
	for _, def := range defs {
		logging.Infof("[Tools]   - %s", def.Name)
	}
	logging.Infof("[Tools] =====================================")

	return m, nil
}

func (m *Manager) Registry() *Registry {
	return m.registry
}

func (m *Manager) Close() error {
	var errs []error
	for _, s := range m.sessions {
		if err := s.cli.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

func LocalSpecs() []Spec {
	return []Spec{
		{
			Name:        "getTime",
			Description: "获取当前时间",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, args json.RawMessage) (Result, error) {
				now := time.Now()
				data := map[string]interface{}{
					"current":   now.Format("2006-01-02 15:04:05"),
					"weekday":   now.Weekday().String(),
					"timestamp": now.Unix(),
				}
				jsonBytes, _ := json.Marshal(data)
				logging.Infof("[Tool] getTime 执行完成，结果: %v", data)
				return Result{Output: string(jsonBytes)}, nil
			},
		},
	}
}
