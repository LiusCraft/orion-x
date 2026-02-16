package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/providertemplate"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestProviderTemplateRepository_CreateListUpdateDelete(t *testing.T) {
	repo, db := newProviderTemplateRepositoryTestHarness(t)
	ctx := context.Background()

	fieldsRaw := json.RawMessage(`[
		{"key":"model","label":"Model","type":"select","required":true,"options":[{"label":"GLM-4","value":"glm-4"}]},
		{"key":"temperature","label":"Temperature","type":"number","min":0,"max":2,"step":0.1}
	]`)

	created, err := repo.Create(ctx, providertemplate.Template{
		ID:        uuid.New(),
		Category:  contracts.ResourceLLM,
		Provider:  "zhipu",
		Status:    contracts.ResourceStatusActive,
		Version:   1,
		Fields:    fieldsRaw,
		CreatedBy: uuid.New(),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	category := contracts.ResourceLLM
	status := contracts.ResourceStatusActive
	items, err := repo.List(ctx, providertemplate.ListFilter{
		Category: &category,
		Provider: "zhipu",
		Status:   &status,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected list size 1, got %d", len(items))
	}

	version := 2
	updatedStatus := contracts.ResourceStatusInactive
	updatedFields := json.RawMessage(`[
		{"key":"model","label":"Model","type":"select","required":true,"options":[{"label":"GLM-4-Air","value":"glm-4-air"}]}
	]`)
	updated, err := repo.Update(ctx, created.ID, providertemplate.UpdatePatch{
		Version: &version,
		Status:  &updatedStatus,
		Fields:  &updatedFields,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("expected version 2, got %d", updated.Version)
	}
	if updated.Status != contracts.ResourceStatusInactive {
		t.Fatalf("expected inactive status, got %q", updated.Status)
	}

	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = repo.GetByID(ctx, created.ID)
	if !errors.Is(err, providertemplate.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	var count int64
	if err := db.WithContext(ctx).Model(&ProviderTemplateModel{}).Where("id = ?", created.ID).Count(&count).Error; err != nil {
		t.Fatalf("count provider_templates error = %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 visible rows after soft delete, got %d", count)
	}
}

func newProviderTemplateRepositoryTestHarness(t *testing.T) (providertemplate.Repository, *gorm.DB) {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("MANAGER_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set MANAGER_TEST_POSTGRES_DSN to run provider template repository tests")
	}

	ctx := context.Background()
	schemaName := fmt.Sprintf("manager_provider_template_repo_%d", time.Now().UnixNano())

	bootstrapDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open bootstrap db: %v", err)
	}
	if err := bootstrapDB.WithContext(ctx).Exec(fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS "%s"`, schemaName)).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_ = bootstrapDB.WithContext(context.Background()).Exec(fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schemaName)).Error
	})

	testCfg := config.DefaultManagerConfig().Database
	testCfg.DSN = dsn + " search_path=" + schemaName
	testCfg.PingTimeoutMs = 5000

	store, err := OpenPostgres(ctx, testCfg)
	if err != nil {
		t.Fatalf("OpenPostgres() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	migrator := NewMigrator(store.DB())
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	return NewProviderTemplateRepository(store.DB()), store.DB()
}
