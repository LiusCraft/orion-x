package store

import "testing"

// TestPlanStaleCleanup 盯住清理边界：只有 source=code（代码注册表写入）的记录才可能被
// 清理。is_system=true 只代表可见性，管理员从后台创建、之后标成官方的记录
// （source=manual）哪怕 slug 不在代码注册表里也必须保留。
//
// 回归 2026-09-30 事故：管理员官方化的 llm:deepseek 被当成 stale provider 删除，
// 撞上 ai_models 的外键，manager 启动 FATAL、无限重启。
func TestPlanStaleCleanup(t *testing.T) {
	codeSlugs := map[string]bool{"llm:openai-completions": true}
	codeModelKeys := map[string]bool{"p1|gpt-4o": true}
	codeVoiceKeys := map[string]bool{"m1|alloy": true}

	providers := []Provider{
		{ID: "p1", Slug: "llm:openai-completions", Source: SourceCode}, // 注册表里还有 → 保留
		{ID: "p2", Slug: "llm:deepseek", Source: SourceManual},         // 管理员官方化 → 保留
		{ID: "p3", Slug: "tts:retired", Source: SourceCode},            // 代码里已删掉 → 清理
		{ID: "p4", Slug: "llm:legacy", Source: ""},                     // source 列上线前的历史行 → 不碰
	}
	models := []AIModel{
		{ID: "m1", ProviderID: "p1", ModelID: "gpt-4o", Source: SourceCode},           // 保留
		{ID: "m2", ProviderID: "p2", ModelID: "deepseek-flash", Source: SourceManual}, // 保留
		{ID: "m3", ProviderID: "p3", ModelID: "retired-model", Source: SourceCode},    // 清理
		{ID: "m4", ProviderID: "p1", ModelID: "cloned", Source: SourceManual},         // 管理员自建 → 保留
	}
	voices := []ModelVoice{
		{ID: "v1", ModelID: "m1", VoiceID: "alloy", Source: SourceCode},          // 保留
		{ID: "v2", ModelID: "m1", VoiceID: "cloned-voice", Source: SourceManual}, // 管理员的复刻音色 → 保留
		{ID: "v3", ModelID: "m3", VoiceID: "retired-voice", Source: SourceCode},  // 随 stale 模型一起清理
	}

	plan := planStaleCleanup(providers, models, voices, codeSlugs, codeModelKeys, codeVoiceKeys)

	if len(plan.providers) != 1 || plan.providers[0].ID != "p3" {
		t.Errorf("stale providers = %+v, want only p3", plan.providers)
	}
	if len(plan.models) != 1 || plan.models[0].ID != "m3" {
		t.Errorf("stale models = %+v, want only m3", plan.models)
	}
	if len(plan.voices) != 1 || plan.voices[0].ID != "v3" {
		t.Errorf("stale voices = %+v, want only v3", plan.voices)
	}
}
