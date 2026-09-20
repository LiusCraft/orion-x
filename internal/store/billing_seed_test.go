package store

import (
	"strings"
	"testing"
)

// TestBillingSeedItems 盯住 store 层的 seed 投影（billingSeedItems）与
// internal/billing.DefaultItems() 的一致性。store 层不能 import 领域包，所以只能
// 靠这份硬编码期望值兜底：改了一边没改另一边，这里立刻红。
//
// 期望值抄自 internal/billing/item.go 的 DefaultItems()。
func TestBillingSeedItems(t *testing.T) {
	want := []struct {
		code        string
		name        string
		chargeMode  string
		unit        string
		meterSource string
		enabled     bool
	}{
		{code: "llm:tokens:input", name: "LLM 输入 token", chargeMode: "usage", unit: "token", meterSource: "llm", enabled: true},
		{code: "llm:tokens:output", name: "LLM 输出 token", chargeMode: "usage", unit: "token", meterSource: "llm", enabled: true},
		{code: "llm:tokens:reasoning", name: "LLM 推理 token", chargeMode: "usage", unit: "token", meterSource: "llm", enabled: true},
		{code: "llm:tokens:cache:read", name: "LLM 缓存命中 token", chargeMode: "usage", unit: "token", meterSource: "llm", enabled: true},
		{code: "llm:tokens:cache:write", name: "LLM 缓存写入 token", chargeMode: "usage", unit: "token", meterSource: "llm", enabled: true},
		{code: "tts:characters", name: "TTS 合成字符", chargeMode: "usage", unit: "char", meterSource: "tts", enabled: true},
		{code: "tts:audio:seconds", name: "TTS 音频时长", chargeMode: "duration", unit: "second", meterSource: "tts", enabled: false},
		{code: "asr:audio:seconds", name: "ASR 识别时长", chargeMode: "duration", unit: "second", meterSource: "asr", enabled: true},
		{code: "voice:clone", name: "音色复刻", chargeMode: "count", unit: "call", meterSource: "voice:clone", enabled: true},
		{code: "voice:retention", name: "音色保留", chargeMode: "recurring", unit: "period", meterSource: "voice:clone", enabled: false},
		{code: "mcp:tool:call", name: "MCP 工具调用", chargeMode: "count", unit: "call", meterSource: "mcp", enabled: false},
		{code: "kb:embedding:tokens", name: "知识库入库 token", chargeMode: "usage", unit: "token", meterSource: "kb", enabled: false},
	}

	got := billingSeedItems()
	if len(got) != len(want) {
		t.Fatalf("seed has %d items, want %d (sync with internal/billing.DefaultItems)", len(got), len(want))
	}

	seen := make(map[string]bool, len(got))
	for i, w := range want {
		g := got[i]
		t.Run(w.code, func(t *testing.T) {
			if g.Code != w.code {
				t.Fatalf("code = %q, want %q", g.Code, w.code)
			}
			if g.Name != w.name {
				t.Errorf("name = %q, want %q", g.Name, w.name)
			}
			if g.ChargeMode != w.chargeMode {
				t.Errorf("charge_mode = %q, want %q", g.ChargeMode, w.chargeMode)
			}
			if g.Unit != w.unit {
				t.Errorf("unit = %q, want %q", g.Unit, w.unit)
			}
			if g.MeterSource != w.meterSource {
				t.Errorf("meter_source = %q, want %q", g.MeterSource, w.meterSource)
			}
			if g.Enabled != w.enabled {
				t.Errorf("enabled = %v, want %v", g.Enabled, w.enabled)
			}
		})
	}

	for _, g := range got {
		if seen[g.Code] {
			t.Errorf("duplicate seed code %q", g.Code)
		}
		seen[g.Code] = true

		if !strings.Contains(g.Code, ":") {
			t.Errorf("seed code %q must be `:`-separated (see AGENTS.md)", g.Code)
		}
		if g.Name == "" || g.ChargeMode == "" || g.Unit == "" || g.MeterSource == "" {
			t.Errorf("seed %q has an empty field: %+v", g.Code, g)
		}
	}

	// 口径互斥的两组只能默认启用一组，否则同一份用量会被计两次（§3.1）。
	if !seen["tts:characters"] || !seen["tts:audio:seconds"] {
		t.Fatal("seed is missing one of the mutually exclusive TTS items")
	}
}
