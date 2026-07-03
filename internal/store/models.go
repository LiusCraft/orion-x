package store

import "time"

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
