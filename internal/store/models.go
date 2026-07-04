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
type User struct {
	ID           string `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Username     string `gorm:"uniqueIndex;not null;type:varchar(64)" json:"username"`
	PasswordHash string `gorm:"not null" json:"-"`
	Email        string `gorm:"type:varchar(128)" json:"email,omitempty"`
	BaseModel
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
	BaseModel
}

// Provider AI 模型厂商（Anthropic / Aliyun / OpenAI 等）
type Provider struct {
	ID        string            `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name      string            `gorm:"not null;type:varchar(64)" json:"name"`
	Slug      string            `gorm:"uniqueIndex;not null;type:varchar(32)" json:"slug"`
	BaseURL   string            `gorm:"not null;type:varchar(512)" json:"base_url"`
	APIKeyEnc string            `gorm:"not null;type:text" json:"-"`
	IsSystem  bool              `gorm:"not null;default:false;index" json:"is_system"`
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

// AllModelTypes returns all supported model types in display order.
func AllModelTypes() []ModelType {
	return []ModelType{
		ModelTypeText,
		ModelTypeVision,
		ModelTypeSpeech,
		ModelTypeMultimodal,
		ModelTypeEmbedding,
	}
}

// AIModel 用户配置的 AI 模型实例
type AIModel struct {
	ID         string            `gorm:"primaryKey;type:varchar(36)" json:"id"`
	ProviderID string            `gorm:"not null;index;type:varchar(36)" json:"provider_id"`
	Provider   *Provider         `gorm:"foreignKey:ProviderID" json:"provider,omitempty"`
	Name       string            `gorm:"not null;type:varchar(128)" json:"name"`
	Type       ModelType         `gorm:"not null;type:varchar(16);index" json:"type"`
	BaseURL    string            `gorm:"type:varchar(512)" json:"base_url"` // 空则用 provider.base_url
	ModelID    string            `gorm:"not null;type:varchar(128)" json:"model_id"`
	IsSystem   bool              `gorm:"not null;default:false;index" json:"is_system"`
	Extra      datatypes.JSONMap `gorm:"type:jsonb" json:"extra,omitempty"`
	Voices     []ModelVoice      `gorm:"foreignKey:ModelID" json:"voices,omitempty"`
	BaseModel
}

// Language TTS 音色语言标签字典（两级：语言→方言）
type Language struct {
	Code       string      `gorm:"primaryKey;type:varchar(16)" json:"code"`
	Name       string      `gorm:"not null;type:varchar(64)" json:"name"`
	ParentCode *string     `gorm:"type:varchar(16);index" json:"parent_code,omitempty"`
	Parent     *Language   `gorm:"foreignKey:ParentCode;references:Code" json:"parent,omitempty"`
	Children   []*Language `gorm:"foreignKey:ParentCode;references:Code" json:"children,omitempty"`
	BaseModel
}

// VoiceGender 音色性别
type VoiceGender string

const (
	VoiceGenderMale    VoiceGender = "male"
	VoiceGenderFemale  VoiceGender = "female"
	VoiceGenderNeutral VoiceGender = "neutral"
)

// ModelVoice TTS 模型下的音色（含系统内置音色和用户复刻音色）
type ModelVoice struct {
	ID          string         `gorm:"primaryKey;type:varchar(36)" json:"id"`
	ModelID     string         `gorm:"not null;index;type:varchar(36)" json:"model_id"`
	VoiceID     string         `gorm:"not null;type:varchar(128)" json:"voice_id"` // 调用 TTS API 时传的实际 ID
	Name        string         `gorm:"not null;type:varchar(128)" json:"name"`
	Description string         `gorm:"type:text" json:"description,omitempty"`
	Gender      VoiceGender    `gorm:"type:varchar(16)" json:"gender,omitempty"`
	AvatarURL   string         `gorm:"type:varchar(512)" json:"avatar_url,omitempty"`
	PreviewURL  string         `gorm:"type:varchar(512)" json:"preview_url,omitempty"`
	Tags        pq.StringArray `gorm:"type:text[]" json:"tags,omitempty"`
	Langs       pq.StringArray `gorm:"type:text[]" json:"langs,omitempty"`
	// Emotions maps system emotion keys to per-voice config.
	// e.g. {"happy": {"mapped_value": "joy", "preview_url": "https://..."}, "neutral": {}}
	Emotions       datatypes.JSONMap `gorm:"type:jsonb" json:"emotions,omitempty"`
	IsSystem       bool              `gorm:"not null;default:false;index" json:"is_system"`
	IsCloned       bool              `gorm:"not null;default:false" json:"is_cloned"`
	SourceAudioURL string            `gorm:"type:varchar(512)" json:"source_audio_url,omitempty"` // 复刻时上传的音频文件
	Extra          datatypes.JSONMap `gorm:"type:jsonb" json:"extra,omitempty"`
	BaseModel
}
