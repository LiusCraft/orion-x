# 后台任务执行抽象（Task Runner）

## 背景

`internal/task` 之前把「任务编排」和「子代理执行」耦合在一起：`Registry` 直接持有
`*agent.SubAgent`，渠道层在 `ToolSpecs` 的 `start` 回调里现场拼一个 `agent.Agent` +
`session.Session` 并启动。未来要对接远程托管 agent（Claude Managed Agents、阿里云
百炼 Managed Agents），执行后端会变成远端会话，必须先把「执行」抽象出来。

本文记录协议调研结论、抽象接口设计，以及 MVP（进程内 loop）的实现取舍。

## 调研：两家托管 agent 的协议（2026-10）

### Claude Managed Agents（beta，`managed-agents-2026-04-01`）

官方文档：<https://platform.claude.com/docs/en/managed-agents/overview>

- 四个概念：**Agent**（模型 / system / 工具 / MCP / Skills，按版本引用）、
  **Environment**（Anthropic 云沙箱或自托管沙箱）、**Session**（一次运行实例）、
  **Events**（双向事件流）。
- 生命周期：`POST /v1/agents` → `POST /v1/environments` → `POST /v1/sessions` →
  订阅 SSE（session events stream）→ 发送 `user.message` event → 消费事件 →
  收到 `session.status_idle` 表示本轮结束。
- 关键事件类型（`anthropic-sdk-go` v1.58 已内置 `Beta.Agents / Beta.Environments /
  Beta.Sessions.Events`）：`agent.message`（含 text block）、`agent.tool_use`、
  `agent.custom_tool_use`、`session.status_{running,idle,terminated,rescheduled}`、
  `session.error`、`user.interrupt`。
- 会话在服务端持久化，可以 resume、可以 steer（继续追加事件）。
- SDK 附带 `SessionToolRunner`：把 `agent.custom_tool_use` 路由到本地工具执行，再把
  结果事件回填给远端会话——即「远端出脑子，工具在本地」的混合模式。

### 阿里云百炼 Managed Agents（Agent Studio）

官方文档：<https://help.aliyun.com/zh/model-studio/managed-agents-api-overview>

- 同一套概念：Agent / Environment / Session & Event（另有 File / Memory Store /
  Skill / Vault / Webhook / Deployment）。
- Endpoint 按工作空间拼装：`https://{workspace_id}.{region}.maas.aliyuncs.com/api/v1/agentstudio`，
  `Authorization: Bearer <api-key>`。
- `POST /sessions/{id}/events` 投递事件（用户消息、工具回执），
  `GET /sessions/{id}/events` 订阅 SSE 流；事件历史服务端持久化。

### 共性（抽象的依据）

| 维度 | 共性 |
| --- | --- |
| 配置 | Agent 定义 + 运行环境是长期配置，创建一次、按 ID 复用/升版本 |
| 执行 | 每个任务 = 一个远端 session |
| 输入 | 投递 `user.message` 事件 |
| 输出 | SSE 事件流：文本增量 / 工具调用 / 状态变化 |
| 终止 | session 空闲（end_turn）/ 终止 / 错误事件 |
| 控制 | 可继续发事件（steer）、可 interrupt、可 resume |
| 历史 | 服务端持久化，断线可续传 |
| 工具 | 平台内置 + MCP；也支持把自定义工具回调到本地执行 |

结论：任务编排层只应该依赖「可启动、可流式消费、可打断、可关闭」的执行单元；
Agent/Environment 这类长期配置留在各后端实现内部。事件模型需要装得下
文本、工具、用量与终止状态。

## 抽象接口

`internal/task/runner.go`：

```go
// Runner 为一个任务启动后台执行。
type Runner interface {
    Start(req RunRequest) (Run, error)
}

type RunRequest struct {
    TaskID string
    Title  string
}

// Run 是一次后台执行的句柄。
type Run interface {
    Events() <-chan Event          // 在 EventDone/EventError 之后关闭
    Ref() string                   // 写入 Task.RunID（run:<taskID> 或远端 session ID）
    Interrupt(ctx context.Context) error
    Close() error                  // 幂等
}

type EventType string

const (
    EventText    EventType = "task:text"
    EventToolUse EventType = "task:tool:use"
    EventDone    EventType = "task:done"
    EventError   EventType = "task:error"
)

type Event struct {
    Type  EventType
    Text  string
    Tool  *ToolUse
    Usage *agent.Usage // 口径与计费一致，Registry 放进 pipeline 元数据
    Err   error
}
```

生命周期约定：

- `Runner` 自己持有执行生命周期（构造时注入 base ctx，如渠道 root ctx），
  `Start` 返回后执行继续在后台跑，**不受 tool-call ctx 影响**——这是对旧
  `start` 回调「必须传 `c.rootCtx`」这一隐式约束的显式化。
- `Registry.Attach(taskID, run)` 是唯一的接线点：把 `Run` 绑到 `Task`（写
  `Task.RunID`），并启动该任务的输出扇出。
- `Interrupt` 在进程内 loop 中退化为取消整个执行（单轮任务没有 steer 语义）；
  远程后端可做到真正的「打断但不终止」。`Close` 释放资源，幂等。

## Registry 与事件映射

`Registry` 不再认识任何具体执行器，`ToolSpecs(sessionID, runner)` 在创建任务后
调用 `runner.Start` 并 `Attach`。事件到会话 pipeline 的转换集中在 `Registry`
一处（`eventMessage`）：

| Event | pipeline.Message | 说明 |
| --- | --- | --- |
| `task:text` | `data`（string） | `Extra.source=sub_agent`、`task_id`，同时追加 `Task.Progress` |
| `task:tool:use` | 不进入 pipeline | 仅供日志/后续 UI；避免被 TTS 当正文朗读 |
| `task:done` | `finished` | `Usage` 非空时写入 `agent.UsageMetadataKey`，计费口径不变 |
| `task:error` | `error` | `Task.Status=cancelled`、`Progress=错误文本`（沿用旧行为）；`Usage` 非空时同样写入 `agent.UsageMetadataKey` |

失败路径也要带上用量：厂商已经收过钱的 token 不能因为出错或被取消而消失。
`LocalRunner` 在流中途报错时保留本步已收到的用量，随 `EventError` 上报。

`source` 保持 `"sub_agent"`，因为各渠道对输出流的路由依赖它
（例如 xiaozhi 的 `WSOutputStage` 把非 `llm` 文本当作 STT 文本处理）。

## MVP：进程内 loop（LocalRunner）

`internal/task/local_runner.go` 实现一个简单的「LLM + 工具」循环：

1. system prompt（`taskSystemPrompt`：自主完成、不要反问）+ 任务标题作为 user message；
2. 流式调用 LLM，文本增量转成 `EventText`；
3. 有工具调用就串行执行（未知工具生成可恢复的 tool 错误消息；
   `Execute` 返回 Go error 视为框架故障终止），把 assistant/tool 消息写回本地消息列表；
4. 重复直到无工具调用（→ `EventDone`）、达到 `MaxSteps`（→ `EventError`）或取消。

有意的取舍（与旧的「子代理复用完整对话 Agent」相比）：

- 不写长期记忆、不套 soul/rules 对话提示——任务 loop 是独立执行者；
- 工具串行执行（旧实现有 `ParallelSafe` 并发路径）；
- 不经过对话 Agent 的 pipeline/stage，事件由 `Registry` 直接扇出。

保留的关键行为：

- `ProviderContext` 随 assistant 消息回传（部分 provider 依赖它重放思考块）；
- 每步 LLM 响应累加用量（`Steps` 计调用次数），终态随 `EventDone` 上报；失败
  路径（流中途报错、达到步数上限）随 `EventError` 上报已发生的用量；
- 工具集在每次 `Start` 时快照（渠道传 `connMgr.CloneRegistry`），任务代理能看到
  当时已注册的工具（含任务工具、记忆工具），与旧行为一致；
- 渠道配置（thinking/options/max output tokens）透传到每次请求。

取消语义：`Interrupt` / `Close` 会置上 `stopped` 标志，`emit` 与它互斥，
因此 stop 返回之后不会再送出终态事件——被取消的任务不会被误判成正常完成。

顺带修正的一处口径偏差：旧实现把子代理跑在渠道 root ctx 上，turn 级的
usage collector 挂不到它，`FinishedEvent.Usage` 实际恒为 nil——任务用量从未进入
计费。新实现把每步用量累加进终态事件，由 `Registry` 写进消息元数据，
与 `docs/billing-design.md` §13「子代理用量金额不丢」的意图一致（维度落在一个
后续 turn 上，属已知可接受偏差）。

渠道层接线从「回调里现场拼 Agent」简化为：

```go
taskRunner := task.NewLocalRunner(c.rootCtx, task.LocalConfig{
    Client: llmClient, Tools: connMgr.CloneRegistry, Model: agentCfg.Model,
    Thinking: agentCfg.Thinking, Options: agentCfg.ProviderOptions,
    MaxOutputTokens: agentCfg.MaxOutputTokens,
})
for _, spec := range c.tasks.ToolSpecs(sess.ID, taskRunner) {
    connMgr.Registry().Add(spec)
}
```

`internal/agent.SubAgent` 已删除（被本方案取代）。

## 后续扩展（不在本次实现）

1. **Claude 适配器**：`Runner` 实现内部维护 Agent/Environment（首次创建后缓存
   ID），每个任务 `POST /v1/sessions` + SSE 映射：
   `agent.message → EventText`、`agent.tool_use / custom_tool_use → EventToolUse`、
   `session.status_idle(stop_reason=end_turn) → EventDone`、
   `session.error / status_terminated(失败) → EventError`、
   `Run.Interrupt → user.interrupt`。SDK 已内置全部端点。
2. **百炼适配器**：同一形状，差异（workspace endpoint、Bearer、事件字段）封装在
   适配器内。
3. **运行时选择**：按设备/任务配置选择 Runner（配置增加 backend 字段），
   `Registry`/`ToolSpecs` 不用改。
4. **能力协商**：`Run` 可扩展 `Capabilities()`（steer/resume/files），由编排层
   在启动前决策；本地工具回填（custom tools）单独设计。
5. **远端用量计费**：远程平台的 token 由平台计费，是否转成我们的计费量取决于
   产品口径；`Event.Usage` 保持可选。

## 开放问题

- 任务取消 / 列举的 UI 入口（`Registry` 目前只有 mount 语义，没有 Cancel/ListBySession）。
- 远端 `idle` 与真正完成的裁决（含 `stop_reason`、`rescheduled` 等状态）。
- 失败与取消目前都落到 `StatusCancelled`，是否增加 `StatusFailed`。
- 工具权限确认（远端 permission/confirmation 事件）如何回传到渠道。
