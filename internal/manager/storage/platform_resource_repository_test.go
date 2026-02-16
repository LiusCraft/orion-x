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
	"github.com/liuscraft/orion-x/internal/manager/platformresource"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPlatformResourceRepository_CreateListAndInitialVersion(t *testing.T) {
	repo, db := newPlatformResourceRepositoryTestHarness(t)
	ctx := context.Background()

	resource := platformresource.Resource{
		ID:            uuid.New(),
		Category:      contracts.ResourceLLM,
		Provider:      platformresource.ProviderZhipu,
		ResourceKey:   "llm-zhipu-prod",
		Name:          "LLM Zhipu Prod",
		SchemaVersion: 1,
		BaseURL:       "https://open.bigmodel.cn/api/v4",
		AccessKey:     "encrypted:sk-zhipu-prod",
		Capabilities:  json.RawMessage(`{"stream":true}`),
		Config:        json.RawMessage(`{"model":"glm-4-flash"}`),
		Status:        contracts.ResourceStatusActive,
		CreatedBy:     uuid.New(),
	}

	created, err := repo.Create(ctx, resource)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	category := contracts.ResourceLLM
	status := contracts.ResourceStatusActive
	items, err := repo.List(ctx, platformresource.ListFilter{
		Category: &category,
		Provider: platformresource.ProviderZhipu,
		Status:   &status,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected list size 1, got %d", len(items))
	}
	if items[0].ID != created.ID {
		t.Fatalf("expected listed id %s, got %s", created.ID, items[0].ID)
	}

	var versions []PlatformResourceVersionModel
	if err := db.WithContext(ctx).
		Where("entry_id = ?", created.ID).
		Order("version ASC").
		Find(&versions).Error; err != nil {
		t.Fatalf("query versions error = %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected one initial version, got %d", len(versions))
	}
	if versions[0].Version != 1 {
		t.Fatalf("expected initial version to be 1, got %d", versions[0].Version)
	}
}

func TestPlatformResourceRepository_UpdateCreatesVersionSnapshot(t *testing.T) {
	repo, db := newPlatformResourceRepositoryTestHarness(t)
	ctx := context.Background()

	resource := platformresource.Resource{
		ID:            uuid.New(),
		Category:      contracts.ResourceLLM,
		Provider:      platformresource.ProviderZhipu,
		ResourceKey:   "llm-zhipu-stage",
		Name:          "LLM Zhipu Stage",
		SchemaVersion: 1,
		BaseURL:       "https://open.bigmodel.cn/api/v4",
		AccessKey:     "encrypted:sk-zhipu-stage",
		Capabilities:  json.RawMessage(`{"stream":true}`),
		Config:        json.RawMessage(`{"model":"glm-4-flash"}`),
		Status:        contracts.ResourceStatusActive,
		CreatedBy:     uuid.New(),
	}
	created, err := repo.Create(ctx, resource)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updatedConfig := json.RawMessage(`{"model":"glm-4-air"}`)
	updatedBaseURL := "https://open.bigmodel.cn/api/v5"
	updatedAccessKey := "encrypted:sk-zhipu-stage-v2"
	updatedSchemaVersion := 2
	updatedStatus := contracts.ResourceStatusInactive

	updated, err := repo.Update(ctx, created.ID, platformresource.UpdatePatch{
		Config:        &updatedConfig,
		BaseURL:       &updatedBaseURL,
		AccessKey:     &updatedAccessKey,
		SchemaVersion: &updatedSchemaVersion,
		Status:        &updatedStatus,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if updated.SchemaVersion != updatedSchemaVersion {
		t.Fatalf("expected schema version %d, got %d", updatedSchemaVersion, updated.SchemaVersion)
	}
	if updated.BaseURL != updatedBaseURL {
		t.Fatalf("expected base url %q, got %q", updatedBaseURL, updated.BaseURL)
	}
	if updated.AccessKey != updatedAccessKey {
		t.Fatalf("expected access key %q, got %q", updatedAccessKey, updated.AccessKey)
	}
	if updated.Status != updatedStatus {
		t.Fatalf("expected status %q, got %q", updatedStatus, updated.Status)
	}

	var versions []PlatformResourceVersionModel
	if err := db.WithContext(ctx).
		Where("entry_id = ?", created.ID).
		Order("version ASC").
		Find(&versions).Error; err != nil {
		t.Fatalf("query versions error = %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected two versions after update, got %d", len(versions))
	}
	if versions[0].Version != 1 || versions[1].Version != 2 {
		t.Fatalf("expected versions 1 and 2, got %d and %d", versions[0].Version, versions[1].Version)
	}
	if string(versions[1].ConfigSnapshot) != string(updatedConfig) {
		t.Fatalf("expected config snapshot %s, got %s", string(updatedConfig), string(versions[1].ConfigSnapshot))
	}
	if versions[1].BaseURLSnapshot != updatedBaseURL {
		t.Fatalf("expected base url snapshot %q, got %q", updatedBaseURL, versions[1].BaseURLSnapshot)
	}
	if versions[1].AccessKeySnapshot != updatedAccessKey {
		t.Fatalf("expected access key snapshot %q, got %q", updatedAccessKey, versions[1].AccessKeySnapshot)
	}
}

func TestPlatformResourceRepository_DeleteRemovesVersions(t *testing.T) {
	repo, db := newPlatformResourceRepositoryTestHarness(t)
	ctx := context.Background()

	resource := platformresource.Resource{
		ID:            uuid.New(),
		Category:      contracts.ResourceTTS,
		Provider:      platformresource.ProviderDashScope,
		ResourceKey:   "tts-dashscope-main",
		Name:          "TTS DashScope Main",
		SchemaVersion: 1,
		BaseURL:       "https://dashscope.aliyuncs.com/api-ws/v1/inference",
		AccessKey:     "encrypted:sk-dashscope-main",
		Capabilities:  json.RawMessage(`{"emotion":true}`),
		Config:        json.RawMessage(`{"model":"cosyvoice-v3-flash"}`),
		Status:        contracts.ResourceStatusActive,
		CreatedBy:     uuid.New(),
	}
	created, err := repo.Create(ctx, resource)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = repo.GetByID(ctx, created.ID)
	if !errors.Is(err, platformresource.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	var count int64
	if err := db.WithContext(ctx).
		Model(&PlatformResourceVersionModel{}).
		Where("entry_id = ?", created.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count versions error = %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 version rows after delete, got %d", count)
	}
}

func TestPlatformResourceRepository_CreateConflict(t *testing.T) {
	repo, _ := newPlatformResourceRepositoryTestHarness(t)
	ctx := context.Background()

	base := platformresource.Resource{
		Category:      contracts.ResourceASR,
		Provider:      platformresource.ProviderDashScope,
		ResourceKey:   "asr-dashscope-main",
		Name:          "ASR DashScope Main",
		SchemaVersion: 1,
		BaseURL:       "https://dashscope.aliyuncs.com/api-ws/v1/inference",
		AccessKey:     "encrypted:sk-dashscope-main",
		Capabilities:  json.RawMessage(`{"stream":true}`),
		Config:        json.RawMessage(`{"model":"fun-asr-realtime"}`),
		Status:        contracts.ResourceStatusActive,
		CreatedBy:     uuid.New(),
	}

	base.ID = uuid.New()
	if _, err := repo.Create(ctx, base); err != nil {
		t.Fatalf("Create() first error = %v", err)
	}

	base.ID = uuid.New()
	_, err := repo.Create(ctx, base)
	if !errors.Is(err, platformresource.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func newPlatformResourceRepositoryTestHarness(t *testing.T) (platformresource.Repository, *gorm.DB) {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("MANAGER_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set MANAGER_TEST_POSTGRES_DSN to run platform resource repository tests")
	}

	ctx := context.Background()
	schemaName := fmt.Sprintf("manager_platform_resource_repo_%d", time.Now().UnixNano())

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

	return NewPlatformResourceRepository(store.DB()), store.DB()
}
