# Agent 工具加载设计

## 背景

需要在 VoiceAgent 中统一加载工具（内置 + MCP），并为 LLM 绑定可调用的工具 Schema，同时在工具执行侧支持 MCP 调用。工具名称统一加前缀，确保多 MCP 服务不会冲突。

## 目标

- 内置工具与 MCP 工具统一加载。
- LLM 绑定工具信息（含参数 Schema）。
- MCP 工具命名统一为 `mcp.<id>.<tool>`。
- MCP 连接失败仅跳过该 MCP，不影响整体启动。
- 未加载工具调用直接返回 `FinishedEvent`。

## 核心结构

### ToolCatalog（agent）

- 负责管理工具信息（`schema.ToolInfo`）与工具类型、动作回复模板。
- 通过 `LoadToolCatalog` 生成。

主要字段：
- `toolInfos` / `toolInfoByName`
- `toolTypes`
- `actionResponses`
- `mcpToolRefs`（映射 prefixed 名称 → 原始 MCP 名称）

### MCP 前缀规范

- 工具名统一格式：`mcp.<id>.<tool>`
- `id` 由配置 `tools.mcp[].id` 指定，必须唯一。

## 加载流程

1. 预置内置工具列表（getTime/getWeather/search/playMusic/setVolume/pauseMusic）。
2. 根据配置覆盖工具类型与动作回复模板。
3. 遍历 MCP 配置：
   - 建立 MCP client（stdio / sse / streamable）。
   - 初始化 MCP（Initialize）。
   - 通过 `mcpp.GetTools` 拉取工具列表。
   - 对每个工具生成 `schema.ToolInfo`，并加前缀命名。
4. LLM 通过 `BindTools` 绑定最终工具列表。

## 工具执行路由

- ToolExecutor 增强为 CompositeExecutor：
  - 以 `mcp.` 前缀识别 MCP 工具；
  - 根据 `id` 路由到对应 MCP client；
  - 否则执行本地注册工具。

## 异常策略

- MCP 连接失败：记录 warn 日志，跳过该 MCP。
- 未加载工具调用：立即发送 `FinishedEvent` 并中止本次处理。

## 相关配置

参考 `docs/config.md` 中 `tools.mcp`。
