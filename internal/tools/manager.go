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
			_ = m.Close()
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

// CloneRegistry returns a clone of this manager's registry. The clone is
// independent — adding specs to it does not affect other clones or the
// original. Used by wsserver to give each connection its own per-connection
// tool set (IoT / device-MCP) without touching shared state.
func (m *Manager) CloneRegistry() *Registry {
	return m.registry.Clone()
}

// Clone returns a new Manager whose registry is an independent clone of this
// one's. The clone holds no MCP sessions and its Close() is a no-op — the
// original Manager is responsible for managing session lifecycle. Use this to
// create per-connection managers that can accumulate extra tools (IoT /
// device-MCP) without affecting other connections.
func (m *Manager) Clone() *Manager {
	return &Manager{
		registry: m.registry.Clone(),
		sessions: nil,
	}
}

// RegisterMCPServers connects to the given MCP server configs, loads their
// tool specs, and registers them in this manager's registry.  The manager
// tracks the sessions and closes them on Manager.Close().
func (m *Manager) RegisterMCPServers(ctx context.Context, servers []MCPServerConfig) error {
	for _, cfg := range servers {
		specs, session, err := loadMCPSpecs(ctx, cfg)
		if err != nil {
			return fmt.Errorf("register MCP server %s: %w", cfg.ID, err)
		}
		m.sessions = append(m.sessions, session)
		for _, spec := range specs {
			m.registry.Add(spec)
		}
		logging.Infof("[Tools] Registered MCP server %q with %d tools", cfg.ID, len(specs))
	}

	if len(servers) > 0 {
		defs := m.registry.Definitions()
		logging.Infof("[Tools] After MCP registration — total tools: %d", len(defs))
	}
	return nil
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
