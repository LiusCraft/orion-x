// Package billing 是计费的纯领域层：计费项、价格、金额、结算引擎、本地估算，
// 以及数据面与控制面之间的 wire 类型与 port 定义。
//
// 本包零依赖——不 import 数据库、HTTP 框架，也不 import 任何业务包，因此数据面
// （wsserver）和控制面（manager）都能安全引用其中的类型。实现分别住在
// internal/billing/service（控制面）和 internal/billing/client（数据面），
// 详见 docs/billing-design.md §19。
package billing

import "fmt"

// ChargeMode 是计费模式，只用于展示与录入校验；结算逻辑不按它分支，
// 所有模式都归一为 quantity × unit_price。
type ChargeMode string

const (
	ChargeModeCount     ChargeMode = "count"     // 按次
	ChargeModeDuration  ChargeMode = "duration"  // 按时长
	ChargeModeUsage     ChargeMode = "usage"     // 按用量
	ChargeModeRecurring ChargeMode = "recurring" // 周期计费，预留
)

// Unit 是计价单位，也是 quantity 的物理含义。
type Unit string

const (
	UnitCall   Unit = "call"   // 次数
	UnitSecond Unit = "second" // 秒
	UnitToken  Unit = "token"  // token 数
	UnitChar   Unit = "char"   // 字符（rune）数
	UnitByte   Unit = "byte"   // 字节，预留
	UnitPeriod Unit = "period" // 账期，quantity 恒为 1，由控制面定时任务产生
)

// 计量点：谁产生 quantity。
const (
	MeterSourceLLM        = "llm"
	MeterSourceTTS        = "tts"
	MeterSourceASR        = "asr"
	MeterSourceVoiceClone = "voice:clone"
	MeterSourceSession    = "session"
	MeterSourceMCP        = "mcp"
	MeterSourceKB         = "kb"
)

// 计费项 code。业务模块引用这些常量，不要写字面量。
const (
	ItemLLMInput       = "llm:tokens:input"  // 输入 token，已剔除缓存读写
	ItemLLMOutput      = "llm:tokens:output" // 输出 token，已剔除 reasoning
	ItemLLMReasoning   = "llm:tokens:reasoning"
	ItemLLMCacheRead   = "llm:tokens:cache:read"
	ItemLLMCacheWrite  = "llm:tokens:cache:write"
	ItemTTSCharacters  = "tts:characters"
	ItemTTSAudioSecond = "tts:audio:seconds"
	ItemASRAudioSecond = "asr:audio:seconds"
	ItemVoiceClone     = "voice:clone"
	ItemVoiceRetention = "voice:retention"
	ItemMCPToolCall    = "mcp:tool:call"
	ItemKBEmbedding    = "kb:embedding:tokens"
)

// 余额型流水 / 事件里的引用类型。
const (
	RefUsageEvent  = "usage:event"
	RefReservation = "reservation"
	RefOrder       = "order"
	RefManual      = "manual"
	RefVoiceClone  = "voice:clone"
)

// Item 是一条计费项目录记录。billing_items 表是 DefaultItems 的投影。
type Item struct {
	Code        string     // llm:tokens:input / tts:characters / voice:clone
	Name        string     // 展示名
	ChargeMode  ChargeMode // 仅用于 UI 与校验
	Unit        Unit
	MeterSource string // llm | tts | asr | voice:clone | session | mcp
	Enabled     bool
	System      bool // 内置项在管理端只读
}

// Validate 校验计费项自身的一致性。
func (i Item) Validate() error {
	if i.Code == "" {
		return fmt.Errorf("billing: item code is required")
	}
	if i.Unit == "" {
		return fmt.Errorf("billing: item %s: unit is required", i.Code)
	}
	if i.ChargeMode == "" {
		return fmt.Errorf("billing: item %s: charge mode is required", i.Code)
	}
	if !validUnit(i.Unit) {
		return fmt.Errorf("billing: item %s: unknown unit %q", i.Code, i.Unit)
	}
	if !validChargeMode(i.ChargeMode) {
		return fmt.Errorf("billing: item %s: unknown charge mode %q", i.Code, i.ChargeMode)
	}
	return nil
}

func validUnit(u Unit) bool {
	switch u {
	case UnitCall, UnitSecond, UnitToken, UnitChar, UnitByte, UnitPeriod:
		return true
	}
	return false
}

func validChargeMode(m ChargeMode) bool {
	switch m {
	case ChargeModeCount, ChargeModeDuration, ChargeModeUsage, ChargeModeRecurring:
		return true
	}
	return false
}

// DefaultItems 是首批内置计费项。口径向的“互斥项”（tts:characters 与
// tts:audio:seconds）默认只启用一组，避免同一份用量被计两次。
func DefaultItems() []Item {
	return []Item{
		{Code: ItemLLMInput, Name: "LLM 输入 token", ChargeMode: ChargeModeUsage, Unit: UnitToken, MeterSource: MeterSourceLLM, Enabled: true, System: true},
		{Code: ItemLLMOutput, Name: "LLM 输出 token", ChargeMode: ChargeModeUsage, Unit: UnitToken, MeterSource: MeterSourceLLM, Enabled: true, System: true},
		{Code: ItemLLMReasoning, Name: "LLM 推理 token", ChargeMode: ChargeModeUsage, Unit: UnitToken, MeterSource: MeterSourceLLM, Enabled: true, System: true},
		{Code: ItemLLMCacheRead, Name: "LLM 缓存命中 token", ChargeMode: ChargeModeUsage, Unit: UnitToken, MeterSource: MeterSourceLLM, Enabled: true, System: true},
		{Code: ItemLLMCacheWrite, Name: "LLM 缓存写入 token", ChargeMode: ChargeModeUsage, Unit: UnitToken, MeterSource: MeterSourceLLM, Enabled: true, System: true},
		{Code: ItemTTSCharacters, Name: "TTS 合成字符", ChargeMode: ChargeModeUsage, Unit: UnitChar, MeterSource: MeterSourceTTS, Enabled: true, System: true},
		{Code: ItemTTSAudioSecond, Name: "TTS 音频时长", ChargeMode: ChargeModeDuration, Unit: UnitSecond, MeterSource: MeterSourceTTS, Enabled: false, System: true},
		{Code: ItemASRAudioSecond, Name: "ASR 识别时长", ChargeMode: ChargeModeDuration, Unit: UnitSecond, MeterSource: MeterSourceASR, Enabled: true, System: true},
		{Code: ItemVoiceClone, Name: "音色复刻", ChargeMode: ChargeModeCount, Unit: UnitCall, MeterSource: MeterSourceVoiceClone, Enabled: true, System: true},
		{Code: ItemVoiceRetention, Name: "音色保留", ChargeMode: ChargeModeRecurring, Unit: UnitPeriod, MeterSource: MeterSourceVoiceClone, Enabled: false, System: true},
		{Code: ItemMCPToolCall, Name: "MCP 工具调用", ChargeMode: ChargeModeCount, Unit: UnitCall, MeterSource: MeterSourceMCP, Enabled: false, System: true},
		{Code: ItemKBEmbedding, Name: "知识库入库 token", ChargeMode: ChargeModeUsage, Unit: UnitToken, MeterSource: MeterSourceKB, Enabled: false, System: true},
	}
}

// DefaultItem 按 code 查内置计费项。
func DefaultItem(code string) (Item, bool) {
	for _, it := range DefaultItems() {
		if it.Code == code {
			return it, true
		}
	}
	return Item{}, false
}
