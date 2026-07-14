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
	if err := db.AutoMigrate(&User{}, &Voicebot{}, &Device{}, &Provider{}, &AIModel{}, &ModelVoice{}, &Language{}, &MCPMarketEntry{}, &MCPServer{}, &VoicebotMCPBinding{}, &MemoryEntry{}, &SessionTurn{}, &KnowledgeBase{}, &Document{}, &Chunk{}); err != nil {
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	logging.Infof("store: migration done (users, voicebots, devices, providers, ai_models, model_voices, languages, mcp_market_entries, mcp_servers, voicebot_mcp_bindings, memory_entries, session_turns, knowledge_bases, documents, chunks)")

	if err := seedLanguages(db); err != nil {
		return nil, fmt.Errorf("store: seed languages: %w", err)
	}

	if err := ensureTurnFTSIndex(db); err != nil {
		return nil, fmt.Errorf("store: fts index: %w", err)
	}

	if err := ensureProviderSlugIndex(db); err != nil {
		return nil, fmt.Errorf("store: provider slug index: %w", err)
	}

	return db, nil
}

var seedLanguageEntries = []Language{
	{Code: "zh", Name: "中文"},
	{Code: "zh-CN", Name: "中文（简体）", ParentCode: strPtr("zh")},
	{Code: "zh-TW", Name: "中文（繁体）", ParentCode: strPtr("zh")},
	{Code: "zh-HK", Name: "中文（香港）", ParentCode: strPtr("zh")},
	{Code: "yue", Name: "粤语", ParentCode: strPtr("zh")},
	{Code: "zh-minnan", Name: "闽南语", ParentCode: strPtr("zh")},
	{Code: "zh-dongbei", Name: "东北话", ParentCode: strPtr("zh")},
	{Code: "zh-henan", Name: "河南话", ParentCode: strPtr("zh")},
	{Code: "zh-hunan", Name: "湖南话", ParentCode: strPtr("zh")},
	{Code: "zh-shaanxi", Name: "陕西话", ParentCode: strPtr("zh")},
	{Code: "zh-shandong", Name: "山东话", ParentCode: strPtr("zh")},
	{Code: "zh-sichuan", Name: "四川话", ParentCode: strPtr("zh")},
	{Code: "zh-anhui", Name: "安徽话", ParentCode: strPtr("zh")},
	{Code: "en", Name: "英语"},
	{Code: "en-US", Name: "英语（美式）", ParentCode: strPtr("en")},
	{Code: "en-GB", Name: "英语（英式）", ParentCode: strPtr("en")},
	{Code: "en-AU", Name: "英语（澳洲）", ParentCode: strPtr("en")},
	{Code: "ja", Name: "日语"},
	{Code: "ko", Name: "韩语"},
	{Code: "fr", Name: "法语"},
	{Code: "fr-FR", Name: "法语（法国）", ParentCode: strPtr("fr")},
	{Code: "fr-CA", Name: "法语（加拿大）", ParentCode: strPtr("fr")},
	{Code: "de", Name: "德语"},
	{Code: "de-DE", Name: "德语（德国）", ParentCode: strPtr("de")},
	{Code: "es", Name: "西班牙语"},
	{Code: "es-ES", Name: "西班牙语（西班牙）", ParentCode: strPtr("es")},
	{Code: "es-MX", Name: "西班牙语（墨西哥）", ParentCode: strPtr("es")},
	{Code: "pt", Name: "葡萄牙语"},
	{Code: "pt-BR", Name: "葡萄牙语（巴西）", ParentCode: strPtr("pt")},
	{Code: "pt-PT", Name: "葡萄牙语（葡萄牙）", ParentCode: strPtr("pt")},
	{Code: "ar", Name: "阿拉伯语"},
	{Code: "ar-SA", Name: "阿拉伯语（沙特）", ParentCode: strPtr("ar")},
	{Code: "ru", Name: "俄语"},
	{Code: "it", Name: "意大利语"},
	{Code: "it-IT", Name: "意大利语（意大利）", ParentCode: strPtr("it")},
	{Code: "nl", Name: "荷兰语"},
	{Code: "nl-NL", Name: "荷兰语（荷兰）", ParentCode: strPtr("nl")},
	{Code: "pl", Name: "波兰语"},
	{Code: "tr", Name: "土耳其语"},
	{Code: "vi", Name: "越南语"},
	{Code: "th", Name: "泰语"},
	{Code: "th-TH", Name: "泰语（泰国）", ParentCode: strPtr("th")},
	{Code: "id", Name: "印尼语"},
}

func strPtr(s string) *string { return &s }

func ensureTurnFTSIndex(db *gorm.DB) error {
	return db.Exec(`CREATE INDEX IF NOT EXISTS idx_turns_fts ON session_turns
		USING gin(to_tsvector('simple', coalesce(user_text,'') || ' ' || coalesce(assistant_text,'')))`).Error
}

func ensureProviderSlugIndex(db *gorm.DB) error {
	// Drop unique constraint on slug so multiple providers can share the same slug
	_ = db.Exec(`DROP INDEX IF EXISTS idx_providers_slug`).Error
	return db.Exec(`CREATE INDEX IF NOT EXISTS idx_providers_slug ON providers(slug)`).Error
}

func seedLanguages(db *gorm.DB) error {
	var count int64
	if err := db.Model(&Language{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	for _, l := range seedLanguageEntries {
		if err := db.Create(&l).Error; err != nil {
			return fmt.Errorf("seed language %s: %w", l.Code, err)
		}
	}
	logging.Infof("store: seeded %d languages", len(seedLanguageEntries))
	return nil
}
