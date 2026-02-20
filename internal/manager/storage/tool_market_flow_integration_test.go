package storage

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/toolentitlement"
	"github.com/liuscraft/orion-x/internal/manager/toolmarket"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestToolMarketFlowRepositories_PostgresIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("MANAGER_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set MANAGER_TEST_POSTGRES_DSN to run tool market integration test")
	}

	ctx := context.Background()
	schemaName := fmt.Sprintf("manager_tool_market_it_%d", time.Now().UnixNano())

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
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	marketRepo := NewToolMarketRepository(store.DB())
	entitlementRepo := NewToolEntitlementRepository(store.DB())

	itemID := uuid.New()
	item, err := marketRepo.Create(ctx, toolmarket.Item{
		ID:        itemID,
		ToolKey:   "mcp-device-helper",
		Name:      "Device Helper",
		Provider:  "acme",
		Protocol:  contracts.ToolProtocolMCP,
		Config:    []byte(`{"transport":"stream_http","stream_http":{"endpoint":"https://example.com/mcp"}}`),
		Status:    contracts.ToolStatusActive,
		CreatedBy: uuid.New(),
	})
	if err != nil {
		t.Fatalf("marketRepo.Create() error = %v", err)
	}

	userID := uuid.New()
	entitlementID := uuid.New()
	entitlement, err := entitlementRepo.Create(ctx, toolentitlement.Entitlement{
		ID:         entitlementID,
		UserID:     userID,
		ToolItemID: item.ID,
		SourceType: contracts.EntitlementSourcePurchase,
		SourceRef:  "self:test",
		Status:     contracts.EntitlementStatusActive,
		StartsAt:   time.Now().UTC(),
		QuotaUsed:  0,
	})
	if err != nil {
		t.Fatalf("entitlementRepo.Create() error = %v", err)
	}

	entitlements, err := entitlementRepo.ListByUser(ctx, userID, nil)
	if err != nil {
		t.Fatalf("entitlementRepo.ListByUser() error = %v", err)
	}
	if len(entitlements) != 1 {
		t.Fatalf("expected 1 entitlement, got %d", len(entitlements))
	}

	if err := store.DB().WithContext(ctx).Create(&ToolUsageLedgerModel{
		ID:            uuid.New(),
		EntitlementID: entitlement.ID,
		ConsumedUnits: 20,
		CreatedAt:     time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("insert usage ledger error = %v", err)
	}

	usageEntries, err := entitlementRepo.ListUsageByEntitlement(ctx, entitlement.ID)
	if err != nil {
		t.Fatalf("entitlementRepo.ListUsageByEntitlement() error = %v", err)
	}
	if len(usageEntries) != 1 {
		t.Fatalf("expected 1 usage entry, got %d", len(usageEntries))
	}
	if usageEntries[0].ConsumedUnits != 20 {
		t.Fatalf("expected consumed_units 20, got %d", usageEntries[0].ConsumedUnits)
	}

}
