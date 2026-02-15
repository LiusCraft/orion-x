package storage

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestGormMigrator_PostgresIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("MANAGER_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set MANAGER_TEST_POSTGRES_DSN to run postgres integration test")
	}

	ctx := context.Background()
	schemaName := fmt.Sprintf("manager_it_%d", time.Now().UnixNano())

	bootstrapDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open bootstrap db: %v", err)
	}
	if err := bootstrapDB.WithContext(ctx).Exec(fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS "%s"`, schemaName)).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer func() {
		_ = bootstrapDB.WithContext(context.Background()).Exec(fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schemaName)).Error
	}()

	testCfg := config.DefaultManagerConfig().Database
	testCfg.DSN = dsn + " search_path=" + schemaName
	testCfg.PingTimeoutMs = 5000

	store, err := OpenPostgres(ctx, testCfg)
	if err != nil {
		t.Fatalf("OpenPostgres() error = %v", err)
	}
	defer store.Close()

	migrator := NewMigrator(store.DB())
	result, err := migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if !result.Applied {
		t.Fatalf("expected first migration to apply")
	}
	if result.TargetVersion != CurrentSchemaVersion {
		t.Fatalf("expected target version %d, got %d", CurrentSchemaVersion, result.TargetVersion)
	}

	result, err = migrator.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate() second run error = %v", err)
	}
	if result.Applied {
		t.Fatalf("expected second migration to be no-op")
	}

	allTables := append([]string{"schema_migrations"}, migrationTableNames()...)
	for _, table := range allTables {
		var count int64
		err := store.DB().WithContext(ctx).
			Raw(`SELECT count(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = ?`, schemaName, table).
			Scan(&count).Error
		if err != nil {
			t.Fatalf("check table %s exists: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("expected table %s in schema %s", table, schemaName)
		}
	}
}
