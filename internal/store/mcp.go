package store

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ── MCPMarketEntry ──

type MCPMarketStore struct{ db *gorm.DB }

func NewMCPMarketStore(db *gorm.DB) *MCPMarketStore { return &MCPMarketStore{db: db} }

func (s *MCPMarketStore) List() ([]MCPMarketEntry, error) {
	var list []MCPMarketEntry
	if err := s.db.Order("created_at asc").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("mcp market store: list: %w", err)
	}
	return list, nil
}

func (s *MCPMarketStore) ListPaginated(page, pageSize int) ([]MCPMarketEntry, int64, error) {
	var list []MCPMarketEntry
	var total int64
	s.db.Model(&MCPMarketEntry{}).Count(&total)
	offset := (page - 1) * pageSize
	if err := s.db.Order("created_at asc").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("mcp market store: list paginated: %w", err)
	}
	return list, total, nil
}

func (s *MCPMarketStore) GetByID(id string) (*MCPMarketEntry, error) {
	var m MCPMarketEntry
	if err := s.db.First(&m, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// ── MCPServer ──

type MCPServerStore struct{ db *gorm.DB }

func NewMCPServerStore(db *gorm.DB) *MCPServerStore { return &MCPServerStore{db: db} }

func (s *MCPServerStore) ListByOwner(userID string) ([]MCPServer, error) {
	var list []MCPServer
	if err := s.db.Where("owner_id = ? OR owner_id = ''", userID).
		Order("created_at asc").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("mcp server store: list: %w", err)
	}
	return list, nil
}

func (s *MCPServerStore) ListByOwnerPaginated(userID string, page, pageSize int) ([]MCPServer, int64, error) {
	var list []MCPServer
	var total int64
	s.db.Model(&MCPServer{}).Where("owner_id = ? OR owner_id = ''", userID).Count(&total)
	offset := (page - 1) * pageSize
	if err := s.db.Where("owner_id = ? OR owner_id = ''", userID).
		Order("created_at asc").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("mcp server store: list paginated: %w", err)
	}
	return list, total, nil
}

func (s *MCPServerStore) GetByID(id string) (*MCPServer, error) {
	var c MCPServer
	if err := s.db.First(&c, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *MCPServerStore) GetAccessibleByID(id, userID string) (*MCPServer, error) {
	var c MCPServer
	if err := s.db.First(&c, "id = ? AND (owner_id = ? OR owner_id = '')", id, userID).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

type CreateMCPServerParams struct {
	OwnerID      string
	MarketID     *string
	Name         string
	Description  string
	Icon         string
	Tags         pq.StringArray
	Transport    MCPTransport
	Command      string
	Args         pq.StringArray
	Env          datatypes.JSONMap
	CWD          string
	Endpoint     string
	Headers      datatypes.JSONMap
	ToolNameList pq.StringArray
	TimeoutMs    int
	Creator      string
}

func (s *MCPServerStore) Create(params CreateMCPServerParams) (*MCPServer, error) {
	if params.TimeoutMs <= 0 {
		params.TimeoutMs = 30000
	}
	c := &MCPServer{
		ID:           uuid.NewString(),
		OwnerID:      params.OwnerID,
		MarketID:     params.MarketID,
		Name:         params.Name,
		Description:  params.Description,
		Icon:         params.Icon,
		Tags:         params.Tags,
		Transport:    params.Transport,
		Command:      params.Command,
		Args:         params.Args,
		Env:          params.Env,
		CWD:          params.CWD,
		Endpoint:     params.Endpoint,
		Headers:      params.Headers,
		ToolNameList: params.ToolNameList,
		TimeoutMs:    params.TimeoutMs,
		BaseModel:    BaseModel{Creator: params.Creator},
	}
	if err := s.db.Create(c).Error; err != nil {
		return nil, fmt.Errorf("mcp server store: create: %w", err)
	}
	return c, nil
}

func (s *MCPServerStore) Update(id string, updates map[string]any) (*MCPServer, error) {
	if err := s.db.Model(&MCPServer{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("mcp server store: update: %w", err)
	}
	return s.GetByID(id)
}

func (s *MCPServerStore) Delete(id string) error {
	if err := s.db.Delete(&MCPServer{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("mcp server store: delete: %w", err)
	}
	return nil
}

// ── VoicebotMCPBinding ──

type VoicebotMCPBindingStore struct{ db *gorm.DB }

func NewVoicebotMCPBindingStore(db *gorm.DB) *VoicebotMCPBindingStore {
	return &VoicebotMCPBindingStore{db: db}
}

func (s *VoicebotMCPBindingStore) ListByVoicebot(voicebotID string) ([]VoicebotMCPBinding, error) {
	var list []VoicebotMCPBinding
	if err := s.db.Raw("SELECT * FROM voicebot_mcp_bindings WHERE voicebot_id = ?", voicebotID).
		Scan(&list).Error; err != nil {
		return nil, fmt.Errorf("mcp binding store: list: %w", err)
	}
	return list, nil
}

// ListByVoicebotWithServers 返回 voicebot 已绑定的 MCP server 列表（含 enabled 状态）
func (s *VoicebotMCPBindingStore) ListByVoicebotWithServers(voicebotID string) ([]MCPServer, error) {
	var list []MCPServer
	if err := s.db.Raw(
		`SELECT m.* FROM mcp_servers m
		 INNER JOIN voicebot_mcp_bindings b ON b.mcp_server_id = m.id
		 WHERE b.voicebot_id = ? AND b.enabled = ?`, voicebotID, true).
		Scan(&list).Error; err != nil {
		return nil, fmt.Errorf("mcp binding store: list with servers: %w", err)
	}
	return list, nil
}

func (s *VoicebotMCPBindingStore) DeleteByServerID(mcpServerID string) error {
	if err := s.db.Exec("DELETE FROM voicebot_mcp_bindings WHERE mcp_server_id = ?", mcpServerID).Error; err != nil {
		return fmt.Errorf("mcp binding store: delete by server: %w", err)
	}
	return nil
}

type CreateBindingParams struct {
	VoicebotID  string
	MCPServerID string
	Creator     string
}

func (s *VoicebotMCPBindingStore) Bind(params CreateBindingParams) error {
	sql := "INSERT INTO voicebot_mcp_bindings (voicebot_id, mcp_server_id, enabled, created_at, creator) VALUES (?, ?, true, NOW(), ?)"
	if err := s.db.Exec(sql, params.VoicebotID, params.MCPServerID, params.Creator).Error; err != nil {
		return fmt.Errorf("mcp binding store: bind: %w", err)
	}
	return nil
}

func (s *VoicebotMCPBindingStore) Unbind(voicebotID, mcpServerID string) error {
	result := s.db.Exec(
		"DELETE FROM voicebot_mcp_bindings WHERE voicebot_id = ? AND mcp_server_id = ?",
		voicebotID, mcpServerID)
	if result.Error != nil {
		return fmt.Errorf("mcp binding store: unbind: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("mcp binding store: unbind: no rows affected (binding not found)")
	}
	return nil
}

func (s *VoicebotMCPBindingStore) ToggleEnabled(voicebotID, mcpServerID string) (bool, error) {
	var binding VoicebotMCPBinding
	if err := s.db.First(&binding, "voicebot_id = ? AND mcp_server_id = ?", voicebotID, mcpServerID).Error; err != nil {
		return false, err
	}
	newVal := !binding.Enabled
	result := s.db.Exec(
		"UPDATE voicebot_mcp_bindings SET enabled = ? WHERE voicebot_id = ? AND mcp_server_id = ?",
		newVal, voicebotID, mcpServerID)
	if result.Error != nil {
		return false, fmt.Errorf("mcp binding store: toggle: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return false, gorm.ErrRecordNotFound
	}
	return newVal, nil
}

func (s *VoicebotMCPBindingStore) IsBound(voicebotID, mcpServerID string) (bool, error) {
	var count int64
	if err := s.db.Model(&VoicebotMCPBinding{}).
		Where("voicebot_id = ? AND mcp_server_id = ?", voicebotID, mcpServerID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
