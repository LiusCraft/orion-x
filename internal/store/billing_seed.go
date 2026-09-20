package store

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 内置计费项目录的 seed（docs/billing-design.md §3.1）。
//
// ⚠️ 这份表是 internal/billing.DefaultItems() 的手工投影：store 层不能 import 计费
// 领域包（§19 的分层要求），所以 code / name / charge_mode / unit / meter_source /
// enabled 必须与那边逐行保持一致，改一边就要改另一边。billing_seed_test.go 把这份
// 期望值钉住了，至少能在改名时立刻发现。
//
// 口径提醒：tts:characters 与 tts:audio:seconds、asr:audio:seconds 与“通话时长”是
// 互斥的计费口径，同一部署只应启用一组，否则同一份用量会被计两次。
type billingSeedItem struct {
	Code        string
	Name        string
	ChargeMode  string
	Unit        string
	MeterSource string
	Enabled     bool
}

// billingSeedItems 返回首批内置计费项。
func billingSeedItems() []billingSeedItem {
	return []billingSeedItem{
		{Code: "llm:tokens:input", Name: "LLM 输入 token", ChargeMode: "usage", Unit: "token", MeterSource: "llm", Enabled: true},
		{Code: "llm:tokens:output", Name: "LLM 输出 token", ChargeMode: "usage", Unit: "token", MeterSource: "llm", Enabled: true},
		{Code: "llm:tokens:reasoning", Name: "LLM 推理 token", ChargeMode: "usage", Unit: "token", MeterSource: "llm", Enabled: true},
		{Code: "llm:tokens:cache:read", Name: "LLM 缓存命中 token", ChargeMode: "usage", Unit: "token", MeterSource: "llm", Enabled: true},
		{Code: "llm:tokens:cache:write", Name: "LLM 缓存写入 token", ChargeMode: "usage", Unit: "token", MeterSource: "llm", Enabled: true},
		{Code: "tts:characters", Name: "TTS 合成字符", ChargeMode: "usage", Unit: "char", MeterSource: "tts", Enabled: true},
		// 与 tts:characters 互斥的口径，默认关闭。
		{Code: "tts:audio:seconds", Name: "TTS 音频时长", ChargeMode: "duration", Unit: "second", MeterSource: "tts", Enabled: false},
		{Code: "asr:audio:seconds", Name: "ASR 识别时长", ChargeMode: "duration", Unit: "second", MeterSource: "asr", Enabled: true},
		{Code: "voice:clone", Name: "音色复刻", ChargeMode: "count", Unit: "call", MeterSource: "voice:clone", Enabled: true},
		// 预留：复刻音色按月保留，由控制面定时任务产生 quantity。
		{Code: "voice:retention", Name: "音色保留", ChargeMode: "recurring", Unit: "period", MeterSource: "voice:clone", Enabled: false},
		{Code: "mcp:tool:call", Name: "MCP 工具调用", ChargeMode: "count", Unit: "call", MeterSource: "mcp", Enabled: false},
		{Code: "kb:embedding:tokens", Name: "知识库入库 token", ChargeMode: "usage", Unit: "token", MeterSource: "kb", Enabled: false},
	}
}

// SyncBillingItems 把内置计费项目录投影进 billing_items：
//   - 按 code upsert：名称 / 模式 / 单位 / 计量点跟着代码走，**enabled 保留管理员的设置**
//     （口径互斥的两组项就靠管理端只启用一组）；
//   - 已经不在 seed 里的项不物删，只标 enabled=false——历史事件和流水还引用着这个 code。
//
// 幂等，每次启动都可以跑。建议在 AutoMigrate 之后、HTTP 服务起来之前调用，
// 和 SyncSystemTemplates 放在一起。
// sessionMeterSource 判断这个计量点是否在会话内产生用量（会走 authorize 的价格
// 快照）；voice:clone / mcp / kb 这些控制面计量点由各自的业务动作直接报账。
func sessionMeterSource(source string) bool {
	switch source {
	case "llm", "tts", "asr":
		return true
	}
	return false
}

// SyncBillingFallbackPrices 给还没有生效价格的会话计费项插一条 item 级兜底价，
// 返回插入的行数。
//
// 为什么需要它：billing 一开、价格表为空的话，每个会话都会被 price_missing 拒掉，
// 而放行又会静默变成免费服务（§3.4 / §14.3）。兜底价只在“这一项当前没有任何生效
// 价格”时插入，绝不覆盖运营录的真实价格；unitPriceMicro <= 0 时什么都不做——宁可
// 不放行，也不要一个静默的免费档。
func SyncBillingFallbackPrices(db *gorm.DB, unitPriceMicro int64) (int, error) {
	if unitPriceMicro <= 0 {
		return 0, nil
	}
	now := time.Now()
	var created []BillingPrice
	for _, seed := range billingSeedItems() {
		if !seed.Enabled || !sessionMeterSource(seed.MeterSource) {
			continue
		}
		var count int64
		err := db.Model(&BillingPrice{}).
			Where("item_code = ? AND account_id = ''", seed.Code).
			Where("effective_from <= ? AND (effective_to IS NULL OR effective_to > ?)", now, now).
			Count(&count).Error
		if err != nil {
			return 0, fmt.Errorf("billing seed: count prices for %s: %w", seed.Code, err)
		}
		if count > 0 {
			continue
		}
		rounding := "none"
		if seed.ChargeMode == "duration" {
			// duration 类默认向上取整（§3.3）。
			rounding = "ceil"
		}
		created = append(created, BillingPrice{
			ID:             uuid.NewString(),
			ItemCode:       seed.Code,
			AccountID:      "",
			ResourceType:   "item",
			Currency:       "CNY",
			UnitPriceMicro: unitPriceMicro,
			UnitSize:       1,
			Rounding:       rounding,
			EffectiveFrom:  now,
		})
	}
	if len(created) == 0 {
		return 0, nil
	}
	if err := db.CreateInBatches(created, 50).Error; err != nil {
		return 0, fmt.Errorf("billing seed: create fallback prices: %w", err)
	}
	return len(created), nil
}

func SyncBillingItems(db *gorm.DB) error {
	seeds := billingSeedItems()
	rows := make([]BillingItem, 0, len(seeds))
	codes := make([]string, 0, len(seeds))
	for _, seed := range seeds {
		rows = append(rows, BillingItem{
			Code:        seed.Code,
			Name:        seed.Name,
			ChargeMode:  seed.ChargeMode,
			Unit:        seed.Unit,
			MeterSource: seed.MeterSource,
			Enabled:     seed.Enabled,
			IsSystem:    true,
		})
		codes = append(codes, seed.Code)
	}

	err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "code"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "charge_mode", "unit", "meter_source", "is_system", "updated_at",
		}),
	}).CreateInBatches(rows, 50).Error
	if err != nil {
		return fmt.Errorf("billing seed: upsert items: %w", err)
	}

	res := db.Model(&BillingItem{}).
		Where("is_system = true AND code NOT IN ?", codes).
		Update("enabled", false)
	if res.Error != nil {
		return fmt.Errorf("billing seed: disable removed items: %w", res.Error)
	}
	return nil
}
