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
	if err := db.AutoMigrate(&User{}, &Voicebot{}, &Device{}, &Provider{}, &AIModel{}, &ModelVoice{}, &MCPMarketEntry{}, &MCPServer{}, &VoicebotMCPBinding{}, &MemoryEntry{}, &SessionTurn{}, &KnowledgeBase{}, &Document{}, &Chunk{}, &VoicebotKB{}, &OAuthBinding{}, &AgentTemplate{}); err != nil {
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	logging.Infof("store: migration done (users, voicebots, devices, providers, ai_models, model_voices, mcp_market_entries, mcp_servers, voicebot_mcp_bindings, memory_entries, session_turns, knowledge_bases, documents, chunks, oauth_bindings, agent_templates)")

	if err := ensureTurnFTSIndex(db); err != nil {
		return nil, fmt.Errorf("store: fts index: %w", err)
	}

	return db, nil
}

func ensureTurnFTSIndex(db *gorm.DB) error {
	return db.Exec(`CREATE INDEX IF NOT EXISTS idx_turns_fts ON session_turns
		USING gin(to_tsvector('simple', coalesce(user_text,'') || ' ' || coalesce(assistant_text,'')))`).Error
}
