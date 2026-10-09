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

	if err := db.AutoMigrate(&User{}, &Voicebot{}, &Device{}, &DeviceChannel{}, &Provider{}, &AIModel{}, &ModelVoice{}, &MCPMarketEntry{}, &MCPServer{}, &VoicebotMCPBinding{}, &MemoryEntry{}, &SessionTurn{}, &KnowledgeBase{}, &Document{}, &Chunk{}, &VoicebotKB{}, &OAuthBinding{}, &AgentTemplate{}, &Asset{}, &APIKey{}, &BillingItem{}, &BillingPrice{}, &BillingAccount{}, &BillingLedger{}, &BillingUsageEvent{}, &BillingReservation{}, &BillingGrant{}, &BillingPeriodState{}, &BillingDailyStat{}, &PaymentOrder{}, &PaymentNotification{}); err != nil {
		return nil, fmt.Errorf("store: migrate: %w", err)
	}

	logging.Infof("store: migration done (users, voicebots, devices, device_channels, providers, ai_models, model_voices, mcp_market_entries, mcp_servers, voicebot_mcp_bindings, memory_entries, session_turns, knowledge_bases, documents, chunks, oauth_bindings, agent_templates, assets, api_keys, billing_items, billing_prices, billing_accounts, billing_ledger, billing_usage_events, billing_reservations, billing_grants, billing_period_states, billing_daily_stats, billing_payment_orders, billing_payment_notifications)")

	if err := ensureTurnFTSIndex(db); err != nil {
		return nil, fmt.Errorf("store: fts index: %w", err)
	}

	// 必须在 SyncSystemProviders 之前：sync 按 slug 匹配系统 provider，
	// 旧格式 slug 会被当成过期记录删除并重建（连带 models 换 ID）。
	if err := migrateLegacyIdentifiers(db); err != nil {
		return nil, fmt.Errorf("store: migrate legacy identifiers: %w", err)
	}

	// 同样必须在 SyncSystemProviders 之前：把 source 列上线前的历史记录解析成
	// code / manual（凭 meta_hash），否则 sync 要么不敢清理自己名下的 stale
	// 记录，要么把管理员手工标成官方的记录误删。
	if err := backfillResourceSources(db); err != nil {
		return nil, fmt.Errorf("store: backfill resource sources: %w", err)
	}

	return db, nil
}

func ensureTurnFTSIndex(db *gorm.DB) error {
	return db.Exec(`CREATE INDEX IF NOT EXISTS idx_turns_fts ON session_turns
		USING gin(to_tsvector('simple', coalesce(user_text,'') || ' ' || coalesce(assistant_text,'')))`).Error
}

// backfillResourceSources 把 provider / model / voice 历史行从「未知」解析成明确的
// source 标记。
//
// source 列上线前，只有 SyncSystemProviders 会写 meta_hash：meta_hash 非空 ==
// 这条记录由代码注册表写过 → code；其余（管理员从后台创建）→ manual。
//
// 幂等，每次启动都跑：从新版本回滚到旧 binary 期间写出来的行（旧代码不认 source
// 列，落库是空值）也会在下次启动被解析。管理员编辑记录不会写 meta_hash；显式标成
// code / manual 的行不会被覆盖，把某条 code 记录交还给人工维护的做法是有效的。
func backfillResourceSources(db *gorm.DB) error {
	for _, table := range []string{"providers", "ai_models", "model_voices"} {
		res := db.Exec("UPDATE " + table + " SET source = CASE WHEN meta_hash <> '' THEN 'code' ELSE 'manual' END WHERE source = ''")
		if res.Error != nil {
			return fmt.Errorf("%s: %w", table, res.Error)
		}
		if res.RowsAffected > 0 {
			logging.Infof("store: resolved %d legacy %s row(s) to source=code/manual", res.RowsAffected, table)
		}
	}
	return nil
}

// migrateLegacyIdentifiers 把历史标识值迁移到命名约定（`:` 分段，见 AGENTS.md）。
// 一次性、幂等：迁移后再执行不会匹配到任何行。
func migrateLegacyIdentifiers(db *gorm.DB) error {
	migrations := []struct {
		name string
		sql  string
	}{
		{
			name: "assets.purpose",
			sql: `UPDATE assets SET purpose = CASE purpose
				WHEN 'voice_sample' THEN 'voice:sample'
				WHEN 'kb_document' THEN 'kb:document'
			END
			WHERE purpose IN ('voice_sample', 'kb_document')`,
		},
		{
			name: "providers.slug",
			sql:  `UPDATE providers SET slug = replace(slug, '/', ':') WHERE slug ~ '^(asr|tts|llm)/'`,
		},
	}

	for _, m := range migrations {
		res := db.Exec(m.sql)
		if res.Error != nil {
			return fmt.Errorf("%s: %w", m.name, res.Error)
		}
		if res.RowsAffected > 0 {
			logging.Infof("store: migrated %d %s row(s) to the `:` identifier convention", res.RowsAffected, m.name)
		}
	}
	return nil
}
