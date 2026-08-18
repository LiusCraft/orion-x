# 智能体广场闭环完善

日期：2026-08-18
状态：已确认设计

## 背景

智能体广场（`/agents/plaza`）提供系统模板，"基于此创建"流程为：

1. 前端 `POST /api/agent-templates/:id/use` → 后端递增 `use_count` 并返回模板 `config_json`
2. 前端用返回的 name + config 调 `POST /api/voicebots` 创建 voicebot
3. 用户进入详情页继续配置

现状问题：

- 9 个种子模板中 8 个 `ConfigJSON` 为 `{}`，"基于此创建"得到空壳 voicebot
- `POST /api/voicebots` 创建时不校验 config_json（Update 校验），`{}` 被原样存储
- 运行时 `DeviceConfig` 只在 `agentCfg.ASR.ModelID != ""` 时才按 AgentConfig 组装，否则把 config_json 当旧式 AppConfig 原样返回 —— 部分模板配置（如只带 `llm.soul_prompt` 的小暖）运行时拿不到，模板内容无法生效
- 前端有死代码（`hasData` + fallback 注释）、无意义的 `setUsing("_new")`、失败无错误提示

## 范围

- 服务端健壮性：创建校验 + 空壳归一化 + DeviceConfig 判定改进
- 模板内容充实：8 个空模板补 AgentConfig 形状的 prompt
- 前端清理：死代码、错误提示

不做：种子同步策略（保持只增不更）、voicebot 运行时 key 填充（用户在详情页配置模型）。

## ① 服务端：VoicebotHandler.Create 校验与归一化

文件：`cmd/manager/handler/voicebot.go`

- config_json 为空或 `"null"` → 现有行为不变（存 `json.Marshal(config.DefaultConfig())`）
- 非法 JSON → 400（与 Update 对齐）
- JSON 合法但解析后 Provider 段全空（`Provider.ASR.Type`、`Provider.TTS.Type`、`Provider.LLM.Type` 均为空，即 `{}` 等空壳）→ 归一化为 DefaultConfig JSON

## ② 服务端：DeviceConfig 判定改进

文件：`cmd/manager/handler/internal.go`

现状（internal.go:53-62）：

```go
var agentCfg AgentConfig
if err := json.Unmarshal([]byte(v.ConfigJSON), &agentCfg); err == nil && agentCfg.ASR.ModelID != "" {
    // assemble
    return
}
// legacy AppConfig 原样返回
```

改为：

- 顶层 JSON 含 `provider` 键 → 旧式路径原样返回（兼容存量完整 AppConfig）
- 否则按 AgentConfig 组装（不再要求 `ASR.ModelID != ""`）

行为变化：

- 部分 AgentConfig（如只带 `llm.soul_prompt` 的模板配置）运行时组装生效，prompt 真正落到 voicebot
- 零值 model_id 由 assembleConfig 天然兜底（保留 DefaultConfig 默认值）
- 存量 `{}` 数据组装出完整结构，而非裸 `{}`；存量含 `provider` 的完整配置不受影响

## ③ 模板内容充实

文件：`internal/store/agent_template_seed.go`

8 个空模板补 AgentConfig 形状的 `ConfigJSON`（`{"llm": {"rules_prompt": ...}}` 或 `{"llm": {"soul_prompt": ...}}`）：

| 模板 | 内容 |
| --- | --- |
| 通用对话助手 | rules_prompt：通用多轮助手规则 |
| 代码智能体 | rules_prompt：代码生成/调试专注 |
| 内容创作助手 | soul_prompt：创作风格与文体 |
| 知识库问答 | rules_prompt：基于知识库回答 |
| 旅行规划师 | rules_prompt：行程规划流程 |
| 健康问诊助手 | rules_prompt：免责声明 + 引导就医 |
| 数据分析师 | rules_prompt：分析步骤 |
| 语音播报员 | rules_prompt：口语化短句适配语音 |

均为 2-4 句中文 prompt，与现有小暖 prompt 风格一致；模板是起点，用户可在详情页再改。

## ④ 前端清理

文件：`web/manager/src/pages/agents/AgentPlazaPage.tsx`

- 删 `hasData` 变量与 "将现有硬编码模板转换为 API 格式，作为 fallback" 注释（58-59 行）
- 删 `setUsing("_new")`
- `handleUseTemplate` catch 分支加内联错误提示（沿用 AgentDetailPage 的 `saveErr` 内联 error 模式，无 toast 组件）

## 测试

- `go test ./...` 全量通过
- `golangci-lint run ./...` 无告警
- 前端 `cd web/manager && npm run lint && npm run build`
- 手工验证：用"小暖"模板创建 voicebot → 详情页可看到 soul_prompt 已填充；设备配置接口返回组装后的完整配置
