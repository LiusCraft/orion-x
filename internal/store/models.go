package store

import (
	"time"

	"github.com/lib/pq"
	"gorm.io/datatypes"
)

// BaseModel 所有表的公共字段
type BaseModel struct {
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	Creator   string    `gorm:"type:varchar(36);not null;default:''" json:"creator"`
}

// User 管理控制台用户
// Email 是账号系统的主体标识，用于登录和唯一识别。
// Username 为可选的显示名，不参与认证。
type User struct {
	ID           string `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Email        string `gorm:"uniqueIndex;not null;type:varchar(128)" json:"email"`
	Username     string `gorm:"type:varchar(64)" json:"username,omitempty"`
	PasswordHash string `gorm:"not null" json:"-"`
	// GithubID 已废弃：OAuth 绑定统一存入 OAuthBinding 表，此列仅保留历史数据。
	GithubID string `gorm:"uniqueIndex;type:varchar(64)" json:"-"`
	IsAdmin  bool   `gorm:"not null;default:false;index" json:"is_admin"`
	BaseModel
}

// OAuthBinding 用户与第三方 OAuth 平台的绑定关系（多平台、每平台唯一）。
type OAuthBinding struct {
	UserID      string    `gorm:"primaryKey;type:varchar(36)" json:"user_id"`
	Provider    string    `gorm:"primaryKey;type:varchar(32);index:idx_oauth_provider_uid,unique" json:"provider"`
	ProviderUID string    `gorm:"type:varchar(64);index:idx_oauth_provider_uid,unique" json:"provider_uid"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	Creator     string    `gorm:"type:varchar(36)" json:"creator"`
}

// Voicebot 一个 voicebot 实例，持有完整 ASR/TTS/LLM 配置
type Voicebot struct {
	ID         string   `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name       string   `gorm:"not null;type:varchar(128)" json:"name"`
	OwnerID    string   `gorm:"not null;index;type:varchar(36)" json:"owner_id"`
	ConfigJSON string   `gorm:"type:text;not null" json:"config_json"`
	Devices    []Device `gorm:"foreignKey:VoicebotID" json:"devices,omitempty"`
	BaseModel
}

// Device 注册到某个 voicebot 下的设备
type Device struct {
	ID         string `gorm:"primaryKey;type:varchar(128)" json:"id"` // 即 hello.DeviceID
	VoicebotID string `gorm:"not null;index;type:varchar(36)" json:"voicebot_id"`
	Name       string `gorm:"type:varchar(128)" json:"name"`
	// TgBotToken is only exposed to the internal service-to-service endpoint.
	// Public manager API responses use a masked channel status instead.
	TgBotToken string `gorm:"type:varchar(256)" json:"-"` // 设备绑定的 TG Bot Token
	BaseModel
}

// Provider AI 模型厂商
type Provider struct {
	ID        string            `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name      string            `gorm:"not null;type:varchar(64)" json:"name"`
	Slug      string            `gorm:"uniqueIndex;not null;type:varchar(32)" json:"slug"`
	BaseURL   string            `gorm:"not null;type:varchar(512)" json:"base_url"`
	APIKeyEnc string            `gorm:"not null;type:text" json:"-"`
	IsSystem  bool              `gorm:"not null;default:false;index" json:"is_system"`
	MetaHash  string            `gorm:"type:varchar(64)" json:"-"`
	Extra     datatypes.JSONMap `gorm:"type:jsonb" json:"extra,omitempty"`
	BaseModel
}

// ModelType AI 模型类型
type ModelType string

const (
	ModelTypeText       ModelType = "text"
	ModelTypeVision     ModelType = "vision"
	ModelTypeSpeech     ModelType = "speech"
	ModelTypeMultimodal ModelType = "multimodal"
	ModelTypeEmbedding  ModelType = "embedding"
)

func AllModelTypes() []ModelType {
	return []ModelType{
		ModelTypeText, ModelTypeVision, ModelTypeSpeech,
		ModelTypeMultimodal, ModelTypeEmbedding,
	}
}

// AIModel 用户配置的 AI 模型实例
type AIModel struct {
	ID         string            `gorm:"primaryKey;type:varchar(36)" json:"id"`
	ProviderID string            `gorm:"not null;index;type:varchar(36)" json:"provider_id"`
	Provider   *Provider         `gorm:"foreignKey:ProviderID" json:"provider,omitempty"`
	Name       string            `gorm:"not null;type:varchar(128)" json:"name"`
	Type       ModelType         `gorm:"not null;type:varchar(16);index" json:"type"`
	BaseURL    string            `gorm:"type:varchar(512)" json:"base_url"`
	ModelID    string            `gorm:"not null;type:varchar(128)" json:"model_id"`
	IsSystem   bool              `gorm:"not null;default:false;index" json:"is_system"`
	Langs      pq.StringArray    `gorm:"type:text[]" json:"langs,omitempty"`
	MetaHash   string            `gorm:"type:varchar(64)" json:"-"`
	Extra      datatypes.JSONMap `gorm:"type:jsonb" json:"extra,omitempty"`
	Voices     []ModelVoice      `gorm:"foreignKey:ModelID" json:"voices,omitempty"`
	BaseModel
}

// VoiceGender 音色性别
type VoiceGender string

const (
	VoiceGenderMale    VoiceGender = "male"
	VoiceGenderFemale  VoiceGender = "female"
	VoiceGenderNeutral VoiceGender = "neutral"
)

// ModelVoice TTS 模型下的音色
type ModelVoice struct {
	ID             string            `gorm:"primaryKey;type:varchar(36)" json:"id"`
	ModelID        string            `gorm:"not null;index;type:varchar(36)" json:"model_id"`
	VoiceID        string            `gorm:"not null;type:varchar(128)" json:"voice_id"`
	Name           string            `gorm:"not null;type:varchar(128)" json:"name"`
	Description    string            `gorm:"type:text" json:"description,omitempty"`
	Gender         VoiceGender       `gorm:"type:varchar(16)" json:"gender,omitempty"`
	AvatarURL      string            `gorm:"type:varchar(512)" json:"avatar_url,omitempty"`
	PreviewURL     string            `gorm:"type:varchar(512)" json:"preview_url,omitempty"`
	Tags           pq.StringArray    `gorm:"type:text[]" json:"tags,omitempty"`
	Langs          pq.StringArray    `gorm:"type:text[]" json:"langs,omitempty"`
	Emotions       datatypes.JSONMap `gorm:"type:jsonb" json:"emotions,omitempty"`
	IsSystem       bool              `gorm:"not null;default:false;index" json:"is_system"`
	IsCloned       bool              `gorm:"not null;default:false" json:"is_cloned"`
	SourceAudioURL string            `gorm:"type:varchar(512)" json:"source_audio_url,omitempty"`
	MetaHash       string            `gorm:"type:varchar(64)" json:"-"`
	Extra          datatypes.JSONMap `gorm:"type:jsonb" json:"extra,omitempty"`
	BaseModel
}

// MCPTransport MCP 传输协议类型
type MCPTransport string

const (
	MCPTransportStdio      MCPTransport = "stdio"
	MCPTransportSSE        MCPTransport = "sse"
	MCPTransportStreamable MCPTransport = "streamable"
)

// MCPMarketEntry 系统预置的 MCP 市场条目
type MCPMarketEntry struct {
	ID          string            `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name        string            `gorm:"not null;type:varchar(128)" json:"name"`
	Description string            `gorm:"type:text" json:"description,omitempty"`
	Icon        string            `gorm:"type:varchar(64)" json:"icon,omitempty"`
	Tags        pq.StringArray    `gorm:"type:text[]" json:"tags,omitempty"`
	Provider    string            `gorm:"type:varchar(64)" json:"provider,omitempty"`
	Billing     string            `gorm:"type:varchar(64)" json:"billing,omitempty"`
	Price       string            `gorm:"type:varchar(64)" json:"price,omitempty"`
	Config      datatypes.JSONMap `gorm:"type:jsonb;not null" json:"config"`
	HeaderMeta  datatypes.JSONMap `gorm:"type:jsonb" json:"header_meta,omitempty"`
	BaseModel
}

// MCPServer 独立的 MCP 服务器定义
type MCPServer struct {
	ID           string            `gorm:"primaryKey;type:varchar(36)" json:"id"`
	OwnerID      string            `gorm:"type:varchar(36);index" json:"owner_id,omitempty"`
	MarketID     *string           `gorm:"type:varchar(36)" json:"market_id,omitempty"`
	Name         string            `gorm:"not null;type:varchar(128)" json:"name"`
	Description  string            `gorm:"type:text" json:"description,omitempty"`
	Icon         string            `gorm:"type:varchar(64)" json:"icon,omitempty"`
	Tags         pq.StringArray    `gorm:"type:text[]" json:"tags,omitempty"`
	Transport    MCPTransport      `gorm:"not null;type:varchar(32)" json:"transport"`
	Command      string            `gorm:"type:text" json:"command,omitempty"`
	Args         pq.StringArray    `gorm:"type:text[]" json:"args,omitempty"`
	Env          datatypes.JSONMap `gorm:"type:jsonb" json:"env,omitempty"`
	CWD          string            `gorm:"type:varchar(512)" json:"cwd,omitempty"`
	Endpoint     string            `gorm:"type:varchar(512)" json:"endpoint,omitempty"`
	Headers      datatypes.JSONMap `gorm:"type:jsonb" json:"headers,omitempty"`
	ToolNameList pq.StringArray    `gorm:"type:text[]" json:"tool_name_list,omitempty"`
	TimeoutMs    int               `gorm:"not null;default:30000" json:"timeout_ms"`
	BaseModel
}

// VoicebotMCPBinding voicebot 与 MCP 服务器的多对多绑定关系
type VoicebotMCPBinding struct {
	VoicebotID  string    `gorm:"primaryKey;type:varchar(36)" json:"voicebot_id"`
	MCPServerID string    `gorm:"primaryKey;type:varchar(36)" json:"mcp_server_id"`
	Enabled     bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	Creator     string    `gorm:"type:varchar(36)" json:"creator"`
}
