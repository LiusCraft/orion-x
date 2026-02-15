package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/liuscraft/orion-x/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Store struct {
	gormDB      *gorm.DB
	sqlDB       *sql.DB
	pingTimeout time.Duration
}

func OpenPostgres(ctx context.Context, cfg config.ManagerDatabaseConfig) (*Store, error) {
	gormDB, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open gorm postgres: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db from gorm: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	if cfg.ConnMaxLifetimeMs > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeMs) * time.Millisecond)
	}
	if cfg.ConnMaxIdleTimeMs > 0 {
		sqlDB.SetConnMaxIdleTime(time.Duration(cfg.ConnMaxIdleTimeMs) * time.Millisecond)
	}

	store := &Store{
		gormDB:      gormDB,
		sqlDB:       sqlDB,
		pingTimeout: time.Duration(cfg.PingTimeoutMs) * time.Millisecond,
	}
	if err := store.Ping(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return store, nil
}

func (s *Store) DB() *gorm.DB {
	return s.gormDB
}

func (s *Store) Ping(ctx context.Context) error {
	pingCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		pingCtx, cancel = context.WithTimeout(ctx, s.pingTimeout)
		defer cancel()
	}
	if err := s.sqlDB.PingContext(pingCtx); err != nil {
		return fmt.Errorf("sql ping: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	return s.sqlDB.Close()
}
