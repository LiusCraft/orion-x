package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/liuscraft/orion-x/internal/manager/storage"
)

type Server interface {
	Start() error
	Shutdown(ctx context.Context) error
}

type Lifecycle struct {
	autoMigrate bool
	migrator    storage.Migrator
	server      Server
}

func NewLifecycle(autoMigrate bool, migrator storage.Migrator, server Server) *Lifecycle {
	return &Lifecycle{
		autoMigrate: autoMigrate,
		migrator:    migrator,
		server:      server,
	}
}

func (l *Lifecycle) Bootstrap(ctx context.Context, migrateOnly bool) (storage.MigrationResult, error) {
	result := storage.MigrationResult{TargetVersion: storage.CurrentSchemaVersion}

	if migrateOnly || l.autoMigrate {
		if l.migrator == nil {
			return result, errors.New("migrator is required")
		}
		migrationResult, err := l.migrator.Migrate(ctx)
		if err != nil {
			return result, fmt.Errorf("run manager migrations: %w", err)
		}
		result = migrationResult
	}

	if migrateOnly {
		return result, nil
	}
	if l.server == nil {
		return result, errors.New("server is required")
	}

	return result, nil
}

func (l *Lifecycle) Start() error {
	if l.server == nil {
		return errors.New("server is required")
	}
	if err := l.server.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("start manager server: %w", err)
	}
	return nil
}

func (l *Lifecycle) Shutdown(ctx context.Context) error {
	if l.server == nil {
		return nil
	}
	return l.server.Shutdown(ctx)
}
