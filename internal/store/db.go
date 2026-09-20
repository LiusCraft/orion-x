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
	if err := db.AutoMigrate(&User{}, &Voicebot{}, &Device{}, &Provider{}, &AIModel{}, &ModelVoice{}, &MCPMarketEntry{}, &MCPServer{}, &VoicebotMCPBinding{}, &MemoryEntry{}, &SessionTurn{}, &KnowledgeBase{}, &Document{}, &Chunk{}, &VoicebotKB{}, &OAuthBinding{}, &AgentTemplate{}, &Asset{}, &BillingItem{}, &BillingPrice{}, &BillingAccount{}, &BillingLedger{}, &BillingUsageEvent{}, &BillingReservation{}, &BillingGrant{}, &BillingPeriodState{}, &BillingDailyStat{}); err != nil {
		return nil, fmt.Errorf("store: migrate: %w", err)
	}

	logging.Infof("store: migration done (users, voicebots, devices, providers, ai_models, model_voices, mcp_market_entries, mcp_servers, voicebot_mcp_bindings, memory_entries, session_turns, knowledge_bases, documents, chunks, oauth_bindings, agent_templates, assets, billing_items, billing_prices, billing_accounts, billing_ledger, billing_usage_events, billing_reservations, billing_grants, billing_period_states, billing_daily_stats)")

	if err := ensureTurnFTSIndex(db); err != nil {
		return nil, fmt.Errorf("store: fts index: %w", err)
	}

	// 必须在 SyncSystemProviders 之前：sync 按 slug 匹配系统 provider，
	// 旧格式 slug 会被当成过期记录删除并重建（连带 models 换 ID）。
	if err := migrateLegacyIdentifiers(db); err != nil {
		return nil, fmt.Errorf("store: migrate legacy identifiers: %w", err)
	}

	return db, nil
}

func ensureTurnFTSIndex(db *gorm.DB) error {
	return db.Exec(`CREATE INDEX IF NOT EXISTS idx_turns_fts ON session_turns
		USING gin(to_tsvector('simple', coalesce(user_text,'') || ' ' || coalesce(assistant_text,'')))`).Error
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
