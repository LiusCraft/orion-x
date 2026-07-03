package store

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/liuscraft/orion-x/internal/logging"
)

func Open(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("store: open db: %w", err)
	}
	if err := db.AutoMigrate(&User{}, &Voicebot{}, &Device{}); err != nil {
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	logging.Infof("store: migration done (users, voicebots, devices)")
	return db, nil
}
