package store

import (
	"net/url"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	// 让代码注册表里有内置 provider（llm:openai-completions），才能验证
	// 「管理员上架的官方记录与内置记录同 slug」的匹配行为。
	_ "github.com/liuscraft/orion-x/internal/llm/provider/openai"
)

// TestSyncSystemProvidersPostgres 是 store 包里唯一需要真实 Postgres 的测试：sync 的
// 清理逻辑（外键、删除顺序、失败降级成告警）用 DryRun 造不出来。设置
// ORION_TEST_POSTGRES_DSN（URL 形式，如
// postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable）后运行，未设置则跳过
// （CI 里由 go-ci.yml 的 postgres 服务提供）。测试只在自己的 schema 里建表，结束即删，
// 不碰 public。
//
// 覆盖 2026-09-30 事故的现场：
//   - is_system=true 的管理员记录（source=manual）不被 sync 删除；
//   - stale 的 code 记录按 voices → models → providers 的顺序清理，不撞外键；
//   - 仍被引用的记录只告警保留，sync 返回 nil，不再 FATAL 崩启动；
//   - 使用内置 slug 的管理员上架记录不被 sync 覆盖/接管。
func TestSyncSystemProvidersPostgres(t *testing.T) {
	rawDSN := os.Getenv("ORION_TEST_POSTGRES_DSN")
	if rawDSN == "" {
		t.Skip("set ORION_TEST_POSTGRES_DSN (URL) to run the postgres-backed sync test")
	}

	const schema = "orion_sync_test"
	admin, err := gorm.Open(postgres.Open(rawDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	if err := admin.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE").Error; err != nil {
		t.Fatalf("drop stale test schema: %v", err)
	}
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		if err := admin.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE").Error; err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})

	dsn := testSchemaDSN(t, rawDSN, schema)
	db, err := Open(dsn)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	adminID := uuid.NewString()

	// 管理员在后台创建、手工标成官方的记录：注册表里没有对应 slug，但一条都不能删。
	manualProv := Provider{ID: uuid.NewString(), Name: "DeepSeek", Slug: "llm:deepseek", BaseURL: "https://api.deepseek.com", IsSystem: true, Source: SourceManual, BaseModel: BaseModel{Creator: adminID}}
	manualModel := AIModel{ID: uuid.NewString(), ProviderID: manualProv.ID, Name: "deepseek-flash", Type: ModelTypeText, ModelID: "deepseek-flash", IsSystem: true, Source: SourceManual, BaseModel: BaseModel{Creator: adminID}}
	manualVoice := ModelVoice{ID: uuid.NewString(), ModelID: manualModel.ID, VoiceID: "deepseek-voice", Name: "DeepSeek Voice", IsSystem: true, Source: SourceManual, BaseModel: BaseModel{Creator: adminID}}

	// 代码注册过、代码里已经删掉的 stale 记录：sync 应该全部清理。
	staleProv := Provider{ID: uuid.NewString(), Name: "Retired", Slug: "llm:retired", BaseURL: "https://retired.example", IsSystem: true, Source: SourceCode, MetaHash: "retired-hash"}
	staleModel := AIModel{ID: uuid.NewString(), ProviderID: staleProv.ID, Name: "retired-model", Type: ModelTypeText, ModelID: "retired-model", IsSystem: true, Source: SourceCode, MetaHash: "retired-hash"}
	staleVoice := ModelVoice{ID: uuid.NewString(), ModelID: staleModel.ID, VoiceID: "retired-voice", Name: "Retired Voice", IsSystem: true, Source: SourceCode, MetaHash: "retired-hash"}

	// stale 的 code provider/model，但模型下挂着管理员的音色：删不动，应保留并告警，
	// 而不是抛错（旧实现删 model 前会连带删音色，等于静默剁掉管理员的数据）。
	pinnedProv := Provider{ID: uuid.NewString(), Name: "Pinned", Slug: "llm:pinned", BaseURL: "https://pinned.example", IsSystem: true, Source: SourceCode, MetaHash: "pinned-hash"}
	pinnedModel := AIModel{ID: uuid.NewString(), ProviderID: pinnedProv.ID, Name: "pinned-model", Type: ModelTypeSpeech, ModelID: "pinned-model", IsSystem: true, Source: SourceCode, MetaHash: "pinned-hash"}
	pinnedVoice := ModelVoice{ID: uuid.NewString(), ModelID: pinnedModel.ID, VoiceID: "admin-clone", Name: "Admin Clone", IsSystem: true, Source: SourceManual, BaseModel: BaseModel{Creator: adminID}}

	// 管理员上架的官方记录用了内置（代码注册表）的 slug：两条记录是独立账号，sync
	// 只能更新自己那条，不能覆盖管理员的 base_url、也不能把它接管成 source=code。
	shadowedProv := Provider{ID: uuid.NewString(), Name: "OpenAI 代理", Slug: "llm:openai-completions", BaseURL: "https://proxy.example/v1", APIKeyEnc: "sk-admin", IsSystem: true, Source: SourceManual, BaseModel: BaseModel{Creator: adminID}}

	seedRows(t, db,
		&manualProv, &manualModel, &manualVoice,
		&staleProv, &staleModel, &staleVoice,
		&pinnedProv, &pinnedModel, &pinnedVoice,
		&shadowedProv,
	)

	// 事故版本（v0.0.2–v0.0.5）在这里 FATAL：删 stale provider 撞上 ai_models 外键。
	if err := SyncSystemProviders(db); err != nil {
		t.Fatalf("SyncSystemProviders: %v", err)
	}

	for _, tc := range []struct {
		name  string
		model any
		id    string
	}{
		{"admin provider", &Provider{}, manualProv.ID},
		{"admin model", &AIModel{}, manualModel.ID},
		{"admin voice", &ModelVoice{}, manualVoice.ID},
		{"referenced stale provider", &Provider{}, pinnedProv.ID},
		{"referenced stale model", &AIModel{}, pinnedModel.ID},
		{"referencing admin voice", &ModelVoice{}, pinnedVoice.ID},
		{"admin-official provider sharing a built-in slug", &Provider{}, shadowedProv.ID},
	} {
		wantRow(t, db, tc.name, tc.model, tc.id, true)
	}

	// 同 slug 的管理员记录没被 sync 覆盖，也没被接管（source 仍是 manual）；同步自己
	// 另外建了一条 source=code 的内置记录。
	var shadowed Provider
	if err := db.First(&shadowed, "id = ?", shadowedProv.ID).Error; err != nil {
		t.Fatalf("load shadowed provider: %v", err)
	}
	if shadowed.BaseURL != "https://proxy.example/v1" || shadowed.Source != SourceManual || !shadowed.IsSystem {
		t.Errorf("admin provider was touched by sync: %+v", shadowed)
	}
	var codeRows int64
	if err := db.Model(&Provider{}).
		Where("slug = ? AND source = ? AND id <> ?", shadowedProv.Slug, SourceCode, shadowedProv.ID).
		Count(&codeRows).Error; err != nil {
		t.Fatalf("count built-in providers: %v", err)
	}
	if codeRows != 1 {
		t.Errorf("built-in rows for %s = %d, want 1 (sync must manage its own row)", shadowedProv.Slug, codeRows)
	}

	for _, tc := range []struct {
		name  string
		model any
		id    string
	}{
		{"stale provider", &Provider{}, staleProv.ID},
		{"stale model", &AIModel{}, staleModel.ID},
		{"stale voice", &ModelVoice{}, staleVoice.ID},
	} {
		wantRow(t, db, tc.name, tc.model, tc.id, false)
	}

	// source 列上线前后回滚出来的行：旧 binary 只写 meta_hash，source 落空值。
	// Open 的 backfill 要把它认回 code，sync 才能继续清理它。
	legacyProv := Provider{ID: uuid.NewString(), Name: "Legacy", Slug: "llm:legacy", BaseURL: "https://legacy.example", IsSystem: true, MetaHash: "legacy-hash", BaseModel: BaseModel{Creator: adminID}}
	seedRows(t, db, &legacyProv)

	if _, err := Open(dsn); err != nil {
		t.Fatalf("store.Open after legacy insert: %v", err)
	}
	var source ResourceSource
	if err := db.Model(&Provider{}).Where("id = ?", legacyProv.ID).Select("source").Scan(&source).Error; err != nil {
		t.Fatalf("read legacy provider source: %v", err)
	}
	if source != SourceCode {
		t.Errorf("legacy provider source = %q after backfill, want %q", source, SourceCode)
	}

	if err := SyncSystemProviders(db); err != nil {
		t.Fatalf("SyncSystemProviders after backfill: %v", err)
	}
	wantRow(t, db, "legacy stale provider", &Provider{}, legacyProv.ID, false)
}

// testSchemaDSN 在 DSN 上挂 search_path，让 AutoMigrate 与 sync 都落在测试 schema 里。
// 要求 URL 形式的 DSN（pgx 把查询参数当会话参数发给服务端）。
func testSchemaDSN(t *testing.T, rawDSN, schema string) string {
	t.Helper()
	u, err := url.Parse(rawDSN)
	if err != nil {
		t.Fatalf("ORION_TEST_POSTGRES_DSN must be a URL: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

func seedRows(t *testing.T, db *gorm.DB, rows ...any) {
	t.Helper()
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}
}

func wantRow(t *testing.T, db *gorm.DB, name string, model any, id string, want bool) {
	t.Helper()
	var got int64
	if err := db.Model(model).Where("id = ?", id).Count(&got).Error; err != nil {
		t.Fatalf("count %s: %v", name, err)
	}
	if present := got > 0; present != want {
		t.Errorf("%s (%T %s) present = %v, want %v", name, model, id, present, want)
	}
}
