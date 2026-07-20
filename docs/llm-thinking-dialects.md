# OpenAI-Compatible Thinking 方言调研

> 调研日期：2026-07-20 | 范围：MiniMax、DeepSeek、Qwen、Kimi 官方 API

## 1. 结论

四家虽然都提供 OpenAI-compatible API，但 thinking 不能作为未经区分的 `extra_fields` 处理。正确的识别维度至少是：

```text
adapter + dialect + model family
```

- `adapter` 决定基础协议，例如 `openai-completions` 或 `openai-responses`；
- `dialect` 决定扩展字段如何映射，例如 `minimax`、`deepseek`、`qwen`、`kimi`；
- `model family` 决定某个能力是否可关闭、支持哪些 effort、是否强制保留历史 reasoning。

同一厂商也可能因 endpoint 或模型族而使用不同配置。最明显的例子是：Qwen Chat Completions 使用 `enable_thinking`，Qwen Responses 使用 `reasoning.effort`；Kimi K3 使用 `reasoning_effort`，Kimi K2.6 使用 `thinking.type/keep`。

因此，公共层可以提供统一的语义配置，但 adapter 必须通过 dialect/model capability 做严格映射和拒绝，不能把同一 JSON 原样发给所有兼容服务。

## 2. 总览

| 厂商/入口 | 模型族 | 开关 | 力度/预算 | 历史保留 | thinking 输出 |
| --- | --- | --- | --- | --- | --- |
| MiniMax Chat | MiniMax-M3 | `thinking.type=adaptive/disabled`；默认开启 | 未声明 effort/budget | 工具循环需保留完整 assistant | `reasoning_split=true` 时为 `reasoning_content` + `reasoning_details`；否则在 `content` 的 `<think>` 中 |
| MiniMax Chat | M2.x | 始终开启，无法关闭 | 未声明 | 工具循环需保留完整 assistant | 同上 |
| DeepSeek Chat | 当前 thinking 模型 | `thinking.type=enabled/disabled`；默认开启 | `reasoning_effort=high/max` | tool call 后必须完整回传 `reasoning_content` | `reasoning_content` |
| Qwen Chat | 混合思考模型 | `enable_thinking=true/false` | 部分模型支持 `thinking_budget` | 部分模型支持 `preserve_thinking=true` | `reasoning_content` |
| Qwen Chat | 仅思考模型 | 始终开启，无法关闭 | 依模型能力 | 依模型能力 | `reasoning_content` |
| Qwen Responses | 支持 Responses 的 Qwen | `reasoning.effort=none` 关闭 | `none/minimal/low/medium/high`；默认 `medium` | Responses Items/`previous_response_id` 语义 | `reasoning` output Item + reasoning token usage |
| Kimi Chat | kimi-k3 | 始终思考 | `reasoning_effort=low/high/max`；默认 `max` | Preserved Thinking 始终开启 | `reasoning_content`，可能为空 |
| Kimi Chat | kimi-k2.7-code | 始终思考；官方建议不传 `thinking` | 无 effort | Preserved Thinking 始终开启 | `reasoning_content` |
| Kimi Chat | kimi-k2.6 | `thinking.type=enabled/disabled`；默认开启 | 无 effort | `thinking.keep=null/all` | `reasoning_content` |
| Kimi Chat | kimi-k2.5 | `thinking.type=enabled/disabled`；默认开启 | 无 effort | 不支持 `thinking.keep` | `reasoning_content` |

注意：表中的“历史保留”与工具循环中的正确性不是同一概念。即使用户选择不跨用户轮次保留 reasoning，只要当前 assistant turn 尚处于 tool loop，adapter 仍必须按照厂商要求回放对应 reasoning 数据。

## 3. MiniMax

官方 OpenAI SDK 文档当前以 `MiniMax-M3` 为主，同时列出 M2.x 系列。

### 3.1 开关

MiniMax-M3：

```json
{"thinking": {"type": "adaptive"}}
```

- `adaptive`：显式开启；当前对 M3 等同于开启 thinking；
- `disabled`：关闭 thinking；
- 省略：默认开启。

```json
{"thinking": {"type": "disabled"}}
```

M2.x：thinking 无法关闭。即使传入 `disabled`，仍保持开启。adapter 应在本地拒绝不可能兑现的 `disabled`，而不是让用户误以为已经关闭。

### 3.2 返回格式

`reasoning_split` 不控制 thinking 开关，只控制返回格式：

```json
{"reasoning_split": true}
```

- `true`：thinking 分离到 `reasoning_content` 和 `reasoning_details`；
- `false` 或省略：原生 Chat Completions 响应将 thinking 放在 `content` 的 `<think>...</think>` 标签中。

为避免 thinking 被 Agent 当成回答送入 TTS，Orion-X 应对 MiniMax 默认设置 `reasoning_split=true`。

官方流式示例把 `reasoning_details[].text` 当作可能的**累计快照**，通过已见长度计算增量。dialect extractor 不能无条件把每次 `text` 都当成 delta 追加，否则可能重复输出。

### 3.3 工具调用

多轮 function call 中要把完整 assistant message 加回 history。启用分离格式时，`reasoning_content`/`reasoning_details` 与 `tool_calls` 都属于需要保留的原生上下文。

### 3.4 SDK 映射

请求扩展通过 `openai-go/v3` 参数的 `SetExtraFields` 注入：

```json
{
  "thinking": {"type": "adaptive"},
  "reasoning_split": true
}
```

响应扩展从 SDK response/chunk 的 `JSON.ExtraFields` 或 `RawJSON()` 读取，不自行解析 SSE。

## 4. DeepSeek

DeepSeek 当前 Thinking Mode 文档同时定义了 OpenAI Format 和 Anthropic Format。本节只讨论 OpenAI Chat Completions 方言。

### 4.1 开关与力度

```json
{
  "thinking": {"type": "enabled"},
  "reasoning_effort": "high"
}
```

- `thinking.type`：`enabled` 或 `disabled`，默认 `enabled`；
- OpenAI SDK 需要把 `thinking` 放进额外 body；
- `reasoning_effort`：有效语义为 `high` 或 `max`；
- thinking 模式普通请求默认 `high`，部分复杂 agent 请求会自动使用 `max`；
- `low`、`medium` 会映射成 `high`，`xhigh` 会映射成 `max`。

adapter 应只对外声明 DeepSeek 实际区分的 `high/max`，不要把兼容映射误报为四档真实能力。

### 4.2 参数冲突

thinking 模式不支持以下采样参数：

- `temperature`
- `top_p`
- `presence_penalty`
- `frequency_penalty`

官方行为是兼容性忽略，而不是报错。Orion-X 应在 validation 阶段报告这些字段不会生效；严格模式下直接拒绝，避免静默配置失效。

### 4.3 返回与工具调用

thinking 通过与 `content` 同级的 `reasoning_content` 返回。

- 没有 tool call 的普通历史 assistant reasoning，后续传入也会被忽略；
- 一旦 assistant 产生 tool call，本次及工具循环所需的 `reasoning_content` 必须完整回传；
- 缺失时 API 会返回 HTTP 400；
- 官方建议直接把完整 SDK assistant message append 回 history。

这说明 DeepSeek `ProviderContext` 不是展示增强，而是 tool loop 正确性数据。

## 5. Qwen

Qwen 同时提供 OpenAI-compatible Chat Completions 与 Responses，两者的推荐 thinking 配置不同。

### 5.1 Chat Completions

混合思考模型使用：

```json
{
  "enable_thinking": true,
  "thinking_budget": 4096,
  "preserve_thinking": true
}
```

- `enable_thinking=true/false`：开启或关闭混合思考模型的 thinking；
- Python OpenAI SDK 通过 extra body 传入；Go adapter 通过 `SetExtraFields`；
- 仅思考模型始终思考，不设置 `enable_thinking`，也无法关闭；
- thinking 通过 `reasoning_content` 返回；
- 部分开源思考模型只支持 streaming。

`thinking_budget`：

- 限制 reasoning 最大 token 数；
- 默认是该模型允许的最大思维链长度；
- 官方当前明确列为 Qwen3 thinking mode 等部分模型支持，不能对所有 Qwen 模型发送。

`preserve_thinking=true`：

- 让模型读取历史 assistant message 中的 `reasoning_content`；
- 默认情况下 Qwen 会忽略历史 reasoning；
- 只支持官方列出的部分新模型；
- 开启后历史 reasoning 计入输入 token 和计费。

另有 prompt 控制方式：部分 Qwen3 开源混合思考模型及 `qwen-plus-2025-04-28` 在 `enable_thinking=true` 时可通过最新 prompt 中的 `/think`、`/no_think` 动态切换。它属于 prompt dialect，不应替代 API 参数，也不建议作为 Orion-X 的默认实现。

### 5.2 Responses

Qwen Responses 的推荐配置已经转向 OpenAI Responses 语义：

```json
{
  "reasoning": {
    "effort": "medium"
  }
}
```

支持：

- `none`：关闭；
- `minimal`；
- `low`；
- `medium`：默认；
- `high`。

重要差异：

- Responses 不支持 `thinking_budget`；
- `reasoning.effort` 优先于 `enable_thinking`；
- 官方建议优先使用 `reasoning.effort`，并说明 Responses 的 `enable_thinking` 后续将不再支持；
- thinking 作为 `reasoning` output Item 返回，token 数在 `usage.output_tokens_details.reasoning_tokens`；
- 多轮可使用 `previous_response_id`，官方当前说明该 ID 有效期为 7 天。

所以 `qwen + openai-completions` 与 `qwen + openai-responses` 必须使用两套 mapper，不能共享 wire options。

## 6. Kimi

Kimi 当前官方 thinking 文档覆盖 `kimi-k3`、`kimi-k2.7-code`、`kimi-k2.6`、`kimi-k2.5`。它们虽然共用 Chat Completions endpoint，但配置差异显著。

### 6.1 kimi-k3

- 始终思考，不能关闭；
- 不使用 `thinking.type`；
- 顶层 `reasoning_effort` 支持 `low/high/max`，默认 `max`；
- Preserved Thinking 始终开启；
- 可能返回 `reasoning_content`；历史 assistant message 必须原样保留。

### 6.2 kimi-k2.7-code

- 始终思考；`disabled` 会报错；
- 官方正文建议调用时无需且不应传 `thinking`，只切换 model；
- Preserved Thinking 始终开启；
- `thinking.keep` 不传或传合法值 `all` 都按 `all` 处理，其他值报错；
- 必须把历史 assistant `reasoning_content` 原样保留。

实现上应把它建模成固定 capability，不需要为了表达固定行为而主动发送冗余参数。

### 6.3 kimi-k2.6

```json
{
  "thinking": {
    "type": "enabled",
    "keep": "all"
  }
}
```

- `type=enabled/disabled`，默认 enabled；
- `keep=null` 或不传：不读取历史轮次 reasoning；
- `keep=all`：开启 Preserved Thinking；
- `keep` 只影响历史 reasoning，不决定当前轮是否思考。

### 6.4 kimi-k2.5

- `thinking.type=enabled/disabled`，默认 enabled；
- 不支持 `thinking.keep`，没有 Preserved Thinking。

### 6.5 通用返回与工具约束

- thinking 在 `reasoning_content`，流式时先于 `content`；
- `reasoning_content + content` 都受 `max_tokens` 限制；
- K2.6/K2.7-code 多步工具调用建议 `max_tokens >= 16000`；
- K2.6/K2.7-code 的 temperature 不可修改，官方建议不要显式发送；
- 当前工具循环中的 reasoning 必须回传；跨用户轮次是否保留再由 fixed capability 或 `thinking.keep` 决定。

## 7. 公共语义配置

建议公共层提供语义配置，而不是直接暴露某一家 JSON：

```go
type ThinkingConfig struct {
	Mode            ThinkingMode
	Effort          ThinkingEffort
	BudgetTokens    *int
	PreserveHistory PreserveMode
	ExposeSummary   bool
}
```

建议枚举：

```text
ThinkingMode:   default | enabled | disabled
ThinkingEffort: default | minimal | low | medium | high | xhigh | max
PreserveMode:   default | none | all
```

映射规则：

1. `default` 表示省略字段，保留 provider/model 默认行为；
2. dialect mapper 只映射目标确实支持的组合；
3. 无法兑现的配置返回 `UnsupportedOptionError`，例如 M2.x/Kimi K3 关闭 thinking；
4. vendor 的兼容降级不应伪装成真实档位，例如 DeepSeek `medium -> high`；
5. `BudgetTokens` 只发给明确支持的 Qwen Chat 模型；
6. tool loop continuity 是 adapter 的强制正确性行为，不受 `PreserveHistory=none` 影响；
7. provider 原始扩展仍可通过受控 typed dialect options 表达，例如 MiniMax `reasoning_split`。

## 8. openai-go/v3 实现边界

三个 OpenAI-compatible Chat dialect 都继续使用 `github.com/openai/openai-go/v3`。

### 8.1 请求

标准字段直接填写 SDK params。非标准字段只在 dialect mapper 最内层调用 `SetExtraFields`：

```go
params.SetExtraFields(map[string]any{
	"thinking": thinking,
})
```

禁止扩展字段覆盖 `model`、`messages`、`tools`、`stream` 等 adapter 已拥有的标准字段。

### 8.2 响应

`openai-go/v3` 的 Chat response 和 chunk 类型保留：

- `RawJSON()`；
- `JSON.ExtraFields map[string]respjson.Field`；
- `respjson.Field.Raw()`。

因此 dialect extractor 可以从 SDK 对象读取 `reasoning_content`、`reasoning_details`，无需自行实现 HTTP/SSE。

需要注意：SDK 的 `ChatCompletionAccumulator` 聚合标准字段，但不会自动把未知扩展字段变成完成态 typed 字段。正确流程是：

1. SDK stream 产生 chunk；
2. 标准内容交给 SDK accumulator；
3. 同一个 chunk 在进入 accumulator 前交给 dialect extractor，读取 SDK 保存的 extra fields；
4. dialect extractor 按该厂商的 delta/cumulative 语义聚合 reasoning；
5. 最终公共 `Response` 合并标准结果与 dialect state；
6. 完整原生 reasoning 写入 `ProviderContext`，不混入可播报 text。

这仍然是 SDK-first：网络、SSE framing、JSON 到 SDK struct 的解码全部由官方 SDK 完成，Orion-X 只解释 SDK 明确保留的扩展字段。

## 9. 推荐配置模型

Provider 配置应区分 adapter 与 dialect：

```json
{
  "adapter": "openai-completions",
  "dialect": "deepseek",
  "model": "deepseek-v4-pro",
  "thinking": {
    "mode": "enabled",
    "effort": "high",
    "preserve_history": "default"
  }
}
```

Qwen Responses 示例：

```json
{
  "adapter": "openai-responses",
  "dialect": "qwen",
  "model": "qwen3.7-plus",
  "thinking": {
    "mode": "enabled",
    "effort": "medium"
  }
}
```

MiniMax 返回格式属于 dialect option：

```json
{
  "adapter": "openai-completions",
  "dialect": "minimax",
  "model": "MiniMax-M3",
  "thinking": {
    "mode": "enabled"
  },
  "dialect_options": {
    "reasoning_split": true
  }
}
```

## 10. 官方资料

- MiniMax, [OpenAI SDK](https://platform.minimaxi.com/docs/api-reference/text-openai-api)
- DeepSeek, [Thinking Mode](https://api-docs.deepseek.com/guides/thinking_mode)
- Alibaba Cloud Model Studio, [深度思考（Chat Completions）](https://help.aliyun.com/zh/model-studio/deep-thinking)
- Alibaba Cloud Model Studio, [OpenAI Responses 接口兼容](https://help.aliyun.com/zh/model-studio/compatibility-with-openai-responses-api)
- Kimi, [思考模式](https://platform.moonshot.cn/docs/guide/use-kimi-k2-thinking-model)

这些配置随模型发布变化较快。实现时要用 fixture 固化已支持模型族的行为，并把未知模型默认视为 `capabilities unknown`，不要根据模型名前缀静默猜测。
