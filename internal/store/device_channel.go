package store

import (
	"fmt"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/liuscraft/orion-x/internal/logging"
)

type DeviceChannelStore struct{ db *gorm.DB }

func NewDeviceChannelStore(db *gorm.DB) *DeviceChannelStore { return &DeviceChannelStore{db: db} }

// Get 返回设备在指定平台上的配置；不存在时返回 gorm.ErrRecordNotFound。
func (s *DeviceChannelStore) Get(deviceID, platformName string) (*DeviceChannel, error) {
	var dc DeviceChannel
	if err := s.db.First(&dc, "device_id = ? AND platform = ?", deviceID, platformName).Error; err != nil {
		return nil, err
	}
	return &dc, nil
}

// ListByDevice 返回设备在所有平台上已配置的通道。
func (s *DeviceChannelStore) ListByDevice(deviceID string) ([]DeviceChannel, error) {
	var list []DeviceChannel
	if err := s.db.Where("device_id = ?", deviceID).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("device channel store: list by device: %w", err)
	}
	return list, nil
}

// ListByPlatform 返回指定平台上所有已配置的设备通道。
func (s *DeviceChannelStore) ListByPlatform(platformName string) ([]DeviceChannel, error) {
	var list []DeviceChannel
	if err := s.db.Where("platform = ?", platformName).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("device channel store: list by platform: %w", err)
	}
	return list, nil
}

// ListByDevices 一次取多台设备的通道配置，返回按 device_id 归组的行。
func (s *DeviceChannelStore) ListByDevices(deviceIDs []string) (map[string][]DeviceChannel, error) {
	if len(deviceIDs) == 0 {
		return nil, nil
	}
	var list []DeviceChannel
	if err := s.db.Where("device_id IN ?", deviceIDs).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("device channel store: list by devices: %w", err)
	}
	grouped := make(map[string][]DeviceChannel, len(deviceIDs))
	for _, row := range list {
		grouped[row.DeviceID] = append(grouped[row.DeviceID], row)
	}
	return grouped, nil
}

// Upsert 写入/覆盖设备在某个平台上的配置（幂等，单条 ON CONFLICT 语句）。
func (s *DeviceChannelStore) Upsert(deviceID, platformName string, config map[string]string) error {
	dc := DeviceChannel{DeviceID: deviceID, Platform: platformName, Config: encodeConfig(config)}
	err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "device_id"}, {Name: "platform"}},
		DoUpdates: clause.AssignmentColumns([]string{"config", "updated_at"}),
	}).Create(&dc).Error
	if err != nil {
		return fmt.Errorf("device channel store: upsert: %w", err)
	}
	return nil
}

// Delete 解绑，幂等：不存在也返回 nil。
func (s *DeviceChannelStore) Delete(deviceID, platformName string) error {
	if err := s.db.Where("device_id = ? AND platform = ?", deviceID, platformName).
		Delete(&DeviceChannel{}).Error; err != nil {
		return fmt.Errorf("device channel store: delete: %w", err)
	}
	return nil
}

// DeleteByDevice 删除设备在所有平台上的配置，随设备删除一起发生。
func (s *DeviceChannelStore) DeleteByDevice(deviceID string) error {
	if err := s.db.Where("device_id = ?", deviceID).Delete(&DeviceChannel{}).Error; err != nil {
		return fmt.Errorf("device channel store: delete by device: %w", err)
	}
	return nil
}

// ConfigStrings 把行里的配置转回字符串 map；非字符串值丢弃（写入路径只会写字符串）。
func (dc *DeviceChannel) ConfigStrings() map[string]string {
	out := make(map[string]string, len(dc.Config))
	for k, v := range dc.Config {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func encodeConfig(config map[string]string) datatypes.JSONMap {
	out := make(datatypes.JSONMap, len(config))
	for k, v := range config {
		out[k] = v
	}
	return out
}

// BackfillTelegramChannels 把存量 devices.tg_bot_token 搬进 device_channels
// （platform=telegramPlatform，config={"bot_token": ...}）。
//
// 幂等：已存在的行不动，可重复执行。旧的 devices.tg_bot_token 列不删除，作为回滚
// 余地；本函数是它唯一剩余的读者。建议在 AutoMigrate 之后、HTTP 服务起来之前调用。
func BackfillTelegramChannels(db *gorm.DB, telegramPlatform string) (int64, error) {
	result := db.Exec(`
		INSERT INTO device_channels (device_id, platform, config, created_at, updated_at)
		SELECT id, ?, jsonb_build_object('bot_token', tg_bot_token), NOW(), NOW()
		FROM devices
		WHERE tg_bot_token IS NOT NULL AND tg_bot_token <> ''
		ON CONFLICT (device_id, platform) DO NOTHING`, telegramPlatform)
	if result.Error != nil {
		return 0, fmt.Errorf("store: backfill telegram channels: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		logging.Infof("store: backfilled %d legacy telegram bot token(s) into device_channels", result.RowsAffected)
	}
	return result.RowsAffected, nil
}
