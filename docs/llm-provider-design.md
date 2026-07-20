# LLM Provider 能力设计

> 状态：设计提案 | 日期：2026-07-20 | 范围：OpenAI Chat Completions、OpenAI Responses、Anthropic Messages

## 1. 结论

LLM 层不应该继续以 OpenAI Chat Completions 的 `message + tool_calls` 作为公共模型。建议把三种 API 当成三个独立的**协议适配器**：

- `openai-completions`：`POST /v1/chat/completions`
- `openai-responses`：`POST /v1/responses`
- `anthropic-messages`：`POST /v1/messages`

公共层只统一 Agent 真正依赖的语义：指令、按顺序排列的内容块、函数工具、结构化输出、停止原因、用量、错误和流式生命周期。协议特有能力通过各自的 typed options 和 opaque provider context 保留，不用 `map[string]any` 把差异偷偷穿透公共接口。

三个 adapter 内部必须直接使用对应的官方 Go SDK：两个 OpenAI adapter 统一使用 `github.com/openai/openai-go/v3`，Anthropic adapter 使用 `github.com/anthropics/anthropic-sdk-go`。adapter 不自行实现 HTTP client、SSE parser、重试器或 wire protocol。

推荐的核心调用接口是 `Generate + Stream`，而不是当前的 `Chat + ChatSync`。流式响应必须输出 typed events，最终以一个完整 `Response` 收口。Agent 只消费 `text_delta` 和最终结果，provider adapter 负责把 SDK 类型转换为公共领域类型、聚合工具参数、保留原生上下文并归一化错误。

首期支持边界：文本、客户端函数工具、并行工具调用、流式输出、JSON Schema 结构化输出、usage、stop reason、取消和可诊断错误。OpenAI hosted tools、Anthropic server tools、图像、文件、音频、batch、background mode 暂不纳入公共能力。

## 2. 当前实现的问题

当前实现位于：

- `internal/llm/types.go`
- `internal/llm/provider/provider.go`
- `internal/llm/provider/openai/chat.go`
- `internal/agent/step.go`
- `internal/session/session.go`

主要问题如下。

1. `llm.Message` 只有 `Content string`、`ToolCalls` 和 `ToolCallID`，实际绑定了 Chat Completions 数据模型，无法无损表达 Responses Items 或 Claude content blocks。
2. `StreamReader` 输出的仍是 `Message`，没有 response start、block start/delta/done、usage、stop reason 和 response done 语义。
3. 工具参数是 `string`。它适合暂存流式 JSON 片段，但完成态应当是 `json.RawMessage`，并在执行前统一验证。
4. system prompt 混在消息历史里。Responses 更适合 top-level `instructions`，Claude 则禁止在 `messages` 中使用 `system` role，要求 top-level `system`。
5. provider 原生上下文完全丢失。Responses reasoning Items、Claude thinking block 的签名都无法在工具调用后的下一请求中原样回传。
6. 没有 stop reason 和 usage，无法区分自然结束、工具调用、长度截断、拒绝和上游异常，也无法做成本与缓存命中观测。
7. `ExtraFields map[string]any` 无 schema、无归属、无冲突检测，公共层无法判断实际启用了什么能力。
8. provider type、厂商和协议被混成 `openai`。同一 OpenAI 账号下的 Completions 与 Responses 需要不同适配逻辑；OpenAI-compatible 厂商通常只兼容 Chat Completions，不能自动视为 Responses-compatible。
9. `ChatSync` 构造请求时没有传 `req.Tools`，同步调用与流式调用行为不一致。
10. 当前 `StreamReader.Close` 同时承担取消和关闭 channel，生产者也调用它；后续重构应明确“消费者只取消，生产者负责结束事件源”，避免 send/close 竞争和阻塞。

## 3. 三种协议的能力差异

| 维度 | OpenAI Chat Completions | OpenAI Responses | Anthropic Messages |
| --- | --- | --- | --- |
| 输入主模型 | `messages[]` | typed `input` Items | `messages[]`，content 为 blocks |
| 系统指令 | `system`/`developer` message | top-level `instructions` 或 input message | top-level `system` |
| 输出主模型 | `choices[].message` | typed `output[]` Items | assistant `content[]` blocks |
| 函数调用 | assistant `tool_calls[]` | `function_call` Item | `tool_use` block |
| 函数结果 | `tool` role message | `function_call_output` Item，用 `call_id` 关联 | 后续 user message 中的 `tool_result` block |
| 流式协议 | choices/chunk/delta | typed SSE events | typed SSE events + block index |
| 对话状态 | 客户端重放 transcript | 手工 Item 重放、`previous_response_id` 或 Conversations | 客户端重放 transcript |
| 推理上下文 | 能力有限且随模型变化 | reasoning Items；工具循环时必须保留 | thinking/redacted thinking blocks；签名必须原样保留 |
| 结构化输出 | `response_format` | `text.format` | `output_config.format` |
| 停止信息 | `finish_reason` | response status、incomplete details、output item status | `stop_reason`、`stop_sequence` |
| 用量 | completion usage；流式需请求 usage | response usage | `message_start` + 累计 `message_delta.usage` |
| 存储语义 | 由请求及账号行为决定 | Responses 默认存储，需显式决定 `store` | API 本身是 stateless conversation |

关键事实：

- Responses 的输出不只是 message；reasoning、function call、function output 都是独立 Item。只读取文本会破坏工具循环和推理连续性。
- 使用 `previous_response_id` 时，上一次 top-level `instructions` 不会自动继承，稳定指令必须每次重发。
- Claude 的 tool result 不是独立 `tool` role，而是 user content 中的 block。thinking 开启时，最后一条 assistant message 的 thinking block 必须完整且未修改地回传。
- Claude 流中工具参数通过 `input_json_delta.partial_json` 分片；Responses 通过 `response.function_call_arguments.delta` 分片；Chat Completions 则通过 choice delta 分片。公共层不能让 Agent 分别处理三种聚合规则。
- Claude 可能在 HTTP 200 后通过 SSE 发送 error，并且官方明确允许未来新增事件类型。adapter 必须容忍未知事件，同时不能吞掉已知错误。

## 4. 设计原则

### 4.1 统一语义，不统一 wire shape

公共类型表达“模型说了什么、要求调用什么、为什么停止、花了多少 token”。每个 adapter 使用官方 SDK 构造请求、消费 SDK 暴露的流式事件和聚合器，再转换为公共领域类型。

不把 OpenAI 的 `choices`、Responses 的 `Items` 或 Claude 的 `content_block_delta` 暴露给 Agent。

### 4.2 不以最小公分母丢数据

可移植的内容进入公共字段；为了下一轮正确回放但不适合公共建模的数据进入 `ProviderContext`。典型内容包括：

- Responses reasoning Item、encrypted reasoning content、原生 output Item ID；
- Claude thinking/redacted thinking block、signature；
- provider 原生但调用方不应解析的关联信息。

`ProviderContext` 与生成它的 adapter、model、provider 配置作用域绑定。切换 adapter、model、base URL 或账号信任域，或重建/裁剪上下文时必须丢弃不再对应的 context，禁止跨 provider 回放。

### 4.3 公共参数必须有稳定语义

只有三种协议都能合理映射的参数进入公共 `Request`。无法稳定映射的能力放入 adapter-specific options，并由 adapter 严格解码、校验未知字段。

例如 `max_output_tokens`、stop sequences、tool choice、parallel tools、JSON Schema output 可以公共化；Responses hosted tools、Claude thinking budget、prompt caching marker 不应伪装成公共参数。

### 4.4 完成态是权威数据

delta 只用于低延迟展示。工具执行、session 持久化、usage 和 stop reason 一律使用最终 `Response`，避免调用方自行拼接不完整 JSON。

### 4.5 SDK-first

三个 adapter 都遵循以下硬性约束：

- 只通过官方 Go SDK 发起同步和流式请求；
- 使用 SDK 提供的 request/response/event union 和 accumulator，不复制官方 wire struct；
- 使用 SDK 的 base URL、header、context cancellation 和 error 机制；
- adapter 只负责公共类型与 SDK 类型之间的双向转换，以及项目统一的 validation、error mapping 和 telemetry；
- SDK 暂未暴露的 API 能力先不支持，不以手写 JSON、HTTP 或 SSE 绕过 SDK；需要时先升级 SDK 并补兼容测试。

为便于单元测试，可以在 adapter 内部定义最小的 SDK service wrapper interface，但生产实现必须委托官方 SDK，不能出现第二套协议客户端。

### 4.6 OpenAI-compatible dialect

`openai-completions` 和 `openai-responses` 表示 wire protocol，不代表所有兼容模型具有完全相同的参数语义。Adapter 根据确定的 model ID 推导 MiniMax、DeepSeek、Qwen、Kimi dialect，再结合模型能力映射，详细调研见 [OpenAI-Compatible Thinking 方言调研](./llm-thinking-dialects.md)。Dialect 不是 provider 或用户配置项。

公共层可以定义 `ThinkingConfig` 的 mode、effort、budget、history preservation 等稳定语义；dialect mapper 负责转换为目标字段。不支持的组合必须在请求前拒绝，不能静默忽略或假装生效。

## 5. 公共 API

以下代码用于约束接口形状，不是逐字实现要求。

```go
package llm

type Client interface {
	Generate(ctx context.Context, req Request) (Response, error)
	Stream(ctx context.Context, req Request) (Stream, error)
	Capabilities() Capabilities
}

type Stream interface {
	Recv() (Event, error)
	Close() error // idempotent cancel; producer owns end-of-stream
}
```

`Generate` 和 `Stream` 必须产生同构的最终 `Response`。`Stream` 的固定契约：

1. 恰好一个 `response_start`；
2. 零到多个内容 delta 事件；
3. 恰好一个 `response_done`，其中携带完整 `Response`；
4. 随后 `Recv` 返回 `io.EOF`。

SDK 建立流之前返回的错误由 `Stream` 直接返回；SDK 流迭代过程中返回的错误由 `Recv` 映射为 typed `*APIError`，不会伪装为 EOF。

### 5.1 Request

```go
type Request struct {
	Instructions    []TextBlock
	Messages        []Message
	Tools           []ToolDefinition
	ToolChoice      ToolChoice
	ParallelTools   *bool
	OutputFormat    *JSONSchemaFormat
	MaxOutputTokens *int
	Temperature     *float64
	StopSequences   []string
	ProviderOptions json.RawMessage
}
```

设计说明：

- model、API key、base URL 属于 client 配置，不在每次请求中重复。
- `Instructions` 与 transcript 分离。adapter 可映射到 `developer/system` message、Responses `instructions` 或 Claude `system`。
- 可选标量使用 pointer，区分“调用方未指定”和零值。
- `ProviderOptions` 由当前 adapter 解码为它自己的具名结构；禁止逐字段 `SetExtraFields`。

### 5.2 Message 和内容块

```go
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	Role            Role
	Blocks          []Block
	ProviderContext *ProviderContext
}

type Block struct {
	Type       BlockType
	Text       string
	ToolCall   *ToolCall
	ToolResult *ToolResult
	Refusal    *Refusal
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type ToolResult struct {
	ToolCallID string
	Content    string
	IsError    bool
}

type ProviderContext struct {
	Adapter string
	Model   string
	Scope   string // stable provider-config identity; never an API key
	Data    json.RawMessage
}
```

约束：

- block 顺序有语义，不能把所有 text 合并后再把 tools 放到末尾。
- assistant message 可包含 text、tool call、refusal；user message 可包含 text、tool result。
- 完成态 `ToolCall.Arguments` 必须是合法 JSON。流式过程中的半截 JSON 只存在于 delta event。
- `ToolCall.ID` 始终表示工具结果使用的关联 ID：Responses 映射 `call_id`，另外的 output item ID 只保存在 `ProviderContext`。
- `ProviderContext` 不发送给 UI，不进入 prompt 文本，不允许业务层修改。
- 如果 session 落盘保存 `ProviderContext`，需要把它视为敏感模型上下文，采用与会话内容相同或更高等级的访问控制；其中可能包含加密 reasoning 或签名材料。

### 5.3 Response

```go
type Response struct {
	ID         string
	Model      string
	Message    Message
	StopReason StopReason
	StopDetail string
	Usage      Usage
	RequestID  string
}

type Usage struct {
	InputTokens       int64
	OutputTokens      int64
	ReasoningTokens   int64
	CacheReadTokens   int64
	CacheWriteTokens  int64
	TotalTokens       int64
	ProviderBreakdown json.RawMessage
}
```

公共 `StopReason` 建议限定为：

- `stop`：自然完成；
- `tool_calls`：需要客户端执行工具；
- `length`：达到输出或上下文限制；
- `content_filter`：拒绝或安全拦截；
- `pause`：provider 要求继续同一 turn；
- `error`：生成失败；
- `unknown`：新值尚未映射。

`StopDetail` 保留原始值，例如 `finish_reason=length`、`stop_reason=pause_turn` 或 Responses incomplete reason，便于诊断且不污染控制流。

### 5.4 Stream events

```go
type Event struct {
	Type       EventType
	Index      int
	TextDelta  string
	ToolCall   *ToolCallDelta
	Reasoning  *ReasoningSummaryDelta
	Response   *Response // only response_done
}

type ToolCallDelta struct {
	ID             string
	Name           string
	ArgumentsDelta string
	Done           bool
}
```

首期事件集合：

- `response_start`
- `text_delta`
- `tool_call_start`
- `tool_call_delta`
- `tool_call_done`
- `reasoning_summary_delta`
- `response_done`

原生 ping、keepalive 和没有公共语义的中间事件不向上冒泡。未知事件记录 debug 日志后跳过；未知的终止状态必须映射为 `unknown` 并保留原值。

### 5.5 Tools

```go
type ToolDefinition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	SchemaMode  SchemaMode
}
```

`SchemaMode` 首期提供 `best_effort` 和 `strict`。迁移默认使用 `best_effort`，以保持现有 MCP tool schema 的兼容性；不能依赖 Responses 与 Chat Completions 不同的隐式 strict 默认值。

启用 `strict` 前统一校验 schema：对象应设置 `additionalProperties: false`，属性 required/nullable 规则满足目标 API 的严格模式约束。adapter 不应静默降级用户明确要求的 `strict`。

`ToolChoice` 公共语义为 `auto`、`none`、`required` 和指定函数。若目标模型或 provider options 与选择冲突，例如 Claude extended thinking 搭配强制指定工具，adapter 在发请求前返回 `UnsupportedOptionError`。

## 6. Provider 配置与注册

```go
type Config struct {
	Adapter     string
	Dialect     string
	APIKey      string
	BaseURL     string
	Model       string
	Headers     map[string]string
	Defaults    RequestDefaults
	Options     json.RawMessage
}
```

`Dialect` 只描述同一协议下的扩展语义，不注册成新的顶层 adapter。例如 `Adapter=openai-completions, Dialect=deepseek` 仍由 OpenAI Completions adapter 和 `openai-go/v3` 执行。

注册 key 使用协议名，而不是厂商简称：

| Key | 默认 Base URL | 说明 |
| --- | --- | --- |
| `openai-completions` | `https://api.openai.com/v1` | OpenAI 及明确兼容 Chat Completions 的第三方 |
| `openai-responses` | `https://api.openai.com/v1` | 只有明确兼容 Responses 的服务才能使用 |
| `anthropic-messages` | `https://api.anthropic.com` | Anthropic Messages wire protocol |

兼容迁移期可以保留 `openai` alias，解析为 `openai-completions` 并记录一次 deprecation warning。不要根据 base URL 或 model 名自动猜协议。

建议包布局：

```text
internal/llm/
  client.go
  types.go
  stream.go
  errors.go
  capabilities.go
  provider/
    registry.go
    openai/
      completions/
      responses/
    anthropic/
      messages/
```

SDK 依赖关系固定为：

| Adapter | 官方 Go SDK | SDK 服务 |
| --- | --- | --- |
| `openai-completions` | `github.com/openai/openai-go/v3` | `client.Chat.Completions` |
| `openai-responses` | `github.com/openai/openai-go/v3` | `client.Responses` |
| `anthropic-messages` | `github.com/anthropics/anthropic-sdk-go` | `client.Messages` |

OpenAI 两个 adapter 可以复用 SDK client 构造、鉴权配置和 error mapping，但 request/response converter 必须分开。Anthropic 必须使用其官方 Messages SDK，不能经过 OpenAI compatibility endpoint，也不能自行实现 Messages wire protocol。

### 6.1 Capabilities

`ProviderMeta` 可以声明协议的最大能力集合，用于 Manager UI；它不能代替请求校验，因为具体能力还受 model、base URL 和账户开关影响。

```go
type Capabilities struct {
	Streaming        bool
	FunctionTools    bool
	ParallelTools    bool
	StructuredOutput bool
	ReasoningSummary bool
	ProviderState    bool
}
```

adapter 必须在发送前执行 `ValidateRequest`。UI 中的 capabilities 是提示，运行时 validation 才是权威结果。不要维护一份很快过期的完整 model capability 硬编码表；只有确知且需要阻止的协议级约束才写入代码。

## 7. 三个 Adapter 的映射规则

### 7.1 openai-completions

- 使用 `openai-go/v3` 的 `client.Chat.Completions.New` 和 `NewStreaming`。
- 标准流式字段使用 SDK 的 `ChatCompletionAccumulator`；厂商扩展从同一 SDK chunk 的 `JSON.ExtraFields`/`RawJSON()` 提取，不能指望 accumulator 自动保留未知字段。
- `Instructions` 转为前置 developer/system messages；保留输入顺序。
- text block 转为 message content。
- assistant tool call blocks 合并为 assistant `tool_calls`。
- user tool result blocks 展开为 `tool` role messages。
- streaming 使用 accumulator 按 tool call index 聚合 ID、name 和 arguments。
- 请求流式 usage 时显式启用对应 stream option；最终 usage 只写入 `response_done`。
- 只接受第一个 choice。公共 API 不支持 `n > 1`；用户若需要多候选应发多个请求。
- `finish_reason` 映射为公共 stop reason，并保留原值。

### 7.2 openai-responses

- 使用 `openai-go/v3` 的 `client.Responses` 同步和流式 API，以及 SDK 定义的 Responses event union。
- `Instructions` 每次请求都发送 top-level `instructions`。
- messages 和 blocks 转为 typed input Items；tool call result 转为 `function_call_output`，使用 call ID 关联。
- 输出遍历全部 `response.output`，不能只使用 `output_text` helper。
- text 处理 `response.output_text.delta`；工具参数处理 `response.function_call_arguments.delta/done`；完成以 `response.completed` 或相应终止事件收口。
- 原生 reasoning 和需要回放的 output Items 写入 assistant message 的 `ProviderContext`。
- 首期默认 `store: false` + 客户端上下文重放，避免接入 Responses 后无意改变数据保留语义。
- 如启用 reasoning，必须保留并回放官方要求的 reasoning Items/encrypted content，尤其是带工具调用的 response。
- 后续可增加显式 `state_mode=server`，使用 `previous_response_id`；它不能与任意裁剪后的本地 transcript 混用，且每次仍需重发 instructions。
- structured output 映射到 `text.format`，不能复用 Chat Completions 的 `response_format` wire 字段。

### 7.3 anthropic-messages

- 使用 `anthropic-sdk-go` 的 `client.Messages` 同步和流式 API，并使用 SDK 的 message accumulation 能力获得完成态 Message。
- `Instructions` 合并为 top-level `system` blocks，不生成 system role message。
- transcript 只产生 user/assistant roles。工具结果放进 user content 的 `tool_result` blocks，并保持 provider 要求的 block 顺序。
- assistant 输出按 content block 原顺序转换；`tool_use.input` 完成后编码为 `json.RawMessage`。
- streaming 按 block index 处理 `content_block_start/delta/stop`；工具参数聚合 `input_json_delta.partial_json`。
- thinking/signature/redacted thinking 不作为语音文本输出，完整写入 `ProviderContext`，下一工具步骤原样回放。
- `message_start` usage 与 `message_delta` 的累计 usage 合并，不能把多个累计值相加。
- SSE error 转为 `APIError`；ping 忽略；未知事件容忍。
- `max_tokens` 是 Messages 请求必填项。公共配置应提供明确的 `max_output_tokens` 默认值，adapter 启动时校验其大于零。
- structured output 映射到 `output_config.format`。

## 8. ProviderContext 与 Session

不要让 adapter 暗中维护整段会话状态。Agent 的 session/context builder 仍是对话历史的唯一事实来源，否则 session 裁剪、摘要和 memory 注入会与 provider 私有状态分叉。

推荐做法：

1. 每个完成的 assistant `Message` 同时保存公共 blocks 和该次响应的 `ProviderContext`。
2. context builder 裁剪某条 message 时，其 provider context 一起裁剪。
3. adapter 构建下一请求时，优先使用匹配当前 adapter/model/scope 的 context 还原该 assistant turn；不匹配时退回公共 blocks。
4. 切换 model、adapter、base URL 信任域后清空所有 provider context。
5. 修改 assistant blocks 后必须清空对应 context，避免“显示内容”和“真实回放内容”不一致。

这比只保存一个 `previous_response_id` 更符合当前架构：本地 session 可以被 memory service 重组，且工具循环需要可恢复、可测试、可迁移。

首期不建议启用 Responses server-managed state。等本地 transcript 模式稳定后，再把它作为显式策略加入，并定义清晰的 fork、过期、删除和数据保留行为。

## 9. 错误模型与可观测性

```go
type APIError struct {
	Adapter    string
	StatusCode int
	Type       string
	Code       string
	Message    string
	RequestID  string
	Retryable  bool
	Cause      error
}
```

至少区分：

- 本地 request validation；
- 鉴权/权限；
- rate limit；
- provider overloaded；
- context length；
- content policy/refusal；
- SDK transport/stream decode；
- context cancellation/deadline；
- provider protocol violation。

日志与指标统一记录 adapter、model、request ID、首 token 延迟、总耗时、stop reason、input/output/cache/reasoning tokens、tool call 数和错误分类。API key、完整 prompt、tool arguments、tool result、ProviderContext 不进入普通日志。

retry 只放在明确安全的阶段：建立响应前的可重试错误可以按策略重试；已经输出 text delta 或 tool call 后默认不自动重试，避免重复播报或重复执行有副作用的工具。

## 10. 实施顺序

### Phase 0：行为固化

- 为当前 openai adapter 增加公共类型到 SDK 参数的 conversion 测试，以及基于 SDK accumulator 的 stream aggregation fixture 测试。
- 修复或在重构中消除 `ChatSync` 未传 tools 的不一致。
- 为 Agent 添加“只以最终 response 执行工具”的契约测试。

### Phase 1：公共类型与兼容层

- 引入 blocks、Response、Usage、StopReason、Event、APIError。
- 实现新 `Generate/Stream` 接口。
- 临时保留旧 `Chat/ChatSync` wrapper，调用新接口，避免一次修改所有 memory/agent 调用点。
- session 增加 ProviderContext，并完善 clone/序列化/裁剪行为。

### Phase 2：openai-completions

- 把现有 adapter 拆到新目录并实现完整 contract。
- 继续使用仓库已有的 `openai-go/v3`，不增加自定义 OpenAI HTTP/SSE 实现。
- 移除公共路径中的 `ExtraFields`；提供 typed completions options。
- 完成 stop reason、usage、取消、SSE error 和并行 tool calls 测试。

### Phase 3：anthropic-messages

- 引入 Messages adapter。
- 添加官方 `anthropic-sdk-go` 依赖，所有请求与流式消费通过 SDK 完成。
- 验证 system mapping、tool result block 顺序、分片 JSON、累计 usage。
- 增加 thinking/signature 原样回放 fixture，即使首期 UI 不展示 thinking。

### Phase 4：openai-responses

- 实现 typed Items 与 SSE events。
- 复用 `openai-go/v3` 的 Responses service 和 event union，不定义平行的 wire structs。
- 默认 `store:false`，完成 reasoning/output Item 的本地回放。
- 验证工具循环、structured output、incomplete response 和未知事件。

### Phase 5：删除旧接口

- Agent、memory compressor、background review 全部迁移到 `Generate/Stream`。
- 删除旧 Message/tool role 兼容代码和 `openai` 注册 key，或将 alias 保留一个明确的弃用周期。

## 11. 测试策略与验收标准

三个 adapter 共享一套 contract test。通过 `httptest.Server` 配置官方 SDK 的 base URL，返回官方协议形状的本地 HTTP/SSE fixtures，以同时验证“公共类型转换 + SDK 行为 + adapter 归一化”，不依赖真实 API key。只 mock adapter 自己定义的最小 SDK service wrapper 时，仍需保留上述 SDK 集成 fixture，避免 mock 与 SDK 实际类型漂移。

必测场景：

1. 同步纯文本；
2. 流式纯文本与 UTF-8 分片；
3. 单工具调用；
4. 多个并行工具调用；
5. text 与 tool call blocks 交错；
6. 工具参数跨多个 delta，完成后 JSON 合法；
7. tool result 和 tool error 回传；
8. stop、tool calls、length、refusal、unknown stop reason；
9. usage 与 cache/reasoning 明细；
10. JSON Schema structured output；
11. 建连前错误与流内错误；
12. ctx cancel、deadline、调用方提前 Close；
13. 未知 SSE event 不破坏后续事件；
14. provider context 在下一工具步骤无损回放；
15. adapter/model 不匹配时 context 被拒绝或清除；
16. sync 与 stream 聚合后的最终 Response 等价。

可选的真实 API smoke tests 通过环境变量单独开启，不进入默认 `go test ./...`。实现完成后按仓库要求运行：

```bash
go test ./...
golangci-lint run ./...
```

## 12. 明确不做的事情

- 不做一个自动猜测任意厂商兼容程度的“万能 OpenAI provider”。
- 不把 Responses hosted tools 与本地 `internal/tools` 混成同一种执行路径。
- 不把 Claude thinking 当作普通 assistant text 送入 TTS 或 UI。
- 不允许未经校验的 `map[string]any` 覆盖公共请求字段。
- 不自行实现 OpenAI 或 Anthropic 的 HTTP client、SSE parser、wire structs 和 retry policy。
- 不绕过官方 SDK 使用裸 JSON 调用 SDK 尚未支持的能力。
- 不把 MiniMax、DeepSeek、Qwen、Kimi 的 thinking 扩展当成同一套 wire 参数；必须经过 dialect/model validation。
- 不在首期提供跨 adapter 的 server-side conversation migration。
- 不为所有模型硬编码能力表；协议校验和 provider 返回错误优先。

## 13. 官方资料

- OpenAI, [Migrate to the Responses API](https://developers.openai.com/api/docs/guides/migrate-to-responses)
- OpenAI, [Function calling](https://developers.openai.com/api/docs/guides/function-calling)
- OpenAI, [Chat Completions API reference](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)
- OpenAI, [Responses API reference](https://developers.openai.com/api/reference/resources/responses/methods/create)
- Anthropic, [Messages API reference](https://platform.claude.com/docs/en/api/messages)
- Anthropic, [Streaming Messages](https://platform.claude.com/docs/en/build-with-claude/streaming)
- Anthropic, [Define tools](https://platform.claude.com/docs/en/agents-and-tools/tool-use/implement-tool-use)
- Anthropic, [Extended thinking](https://platform.claude.com/docs/en/build-with-claude/extended-thinking)
- OpenAI, [openai-go](https://github.com/openai/openai-go)
- Anthropic, [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go)
- [OpenAI-Compatible Thinking 方言调研](./llm-thinking-dialects.md)
