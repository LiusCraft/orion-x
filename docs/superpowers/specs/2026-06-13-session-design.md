# Session 包设计

## 概述

`internal/session` 提供聊天记录管理能力，是一段对话的原始消息日志。

- **Session** = 聊天记录本身（纯消息日志）
- **Memory**（已有）= Agent 从对话中提炼的认知/记忆

两者是不同层次的抽象：Session 是原始数据，Memory 是对原始数据的加工。

## 与现有模块的关系

```
                    ┌─ 新建 ────┐
                    │           ▼
Orchestrator ──► Session ──► Agent ──► LLM
                    │           │
                    │           ▼
                    │       Memory (长期记忆/摘要)
                    │
                    └─ 加载/保存（未来持久化扩展）
```

- **Agent**：从 Session 读取消息历史，对话结束后向 Memory 写入 Turn 供提炼
- **Memory**：从 Turn 数据提炼用户偏好/事实，与 Session 不直接依赖
- **Session 不依赖 Memory**，两者独立

## 数据模型

```go
// Role 消息角色
type Role string

const (
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
    RoleTool      Role = "tool"
)

// ToolCall 工具调用
type ToolCall struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    Arguments string `json:"arguments"`
}

// Message 单条聊天消息
type Message struct {
    ID         string     `json:"id"`
    Role       Role       `json:"role"`
    Content    string     `json:"content"`
    ToolCallID string     `json:"tool_call_id,omitempty"`
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
    Status     string     `json:"status,omitempty"`
    CreatedAt  time.Time  `json:"created_at"`
}

// SessionMeta 会话元数据
type SessionMeta struct {
    UserID    string    `json:"user_id"`
    Model     string    `json:"model"`
    CreatedAt time.Time `json:"created_at"`
}

// Session 一段聊天记录
type Session struct {
    ID        string      `json:"id"`
    Meta      SessionMeta `json:"meta"`
    Messages  []Message   `json:"messages"`
    UpdatedAt time.Time   `json:"updated_at"`
}
```

## 公共接口

```go
// New 创建新会话
func New(meta SessionMeta) *Session

// Add 添加消息
func (s *Session) Add(msg Message)

// Pop 移除并返回最后一条消息
func (s *Session) Pop() (Message, bool)

// PopN 移除最后 n 条消息，按原序返回被移除的消息
func (s *Session) PopN(n int) []Message

// ToLLMMessages 转换为 LLM 请求格式
func (s *Session) ToLLMMessages() []llm.Message

// Clone 深拷贝（并发安全场景使用）
func (s *Session) Clone() *Session

// LastN 获取最近 n 条消息（只读）
func (s *Session) LastN(n int) []Message
```

### Pop 使用场景

| 场景 | 操作 | 调用方 |
|------|------|--------|
| 用户打断正在生成的 assistant 回复 | Pop 掉不完整的 assistant 消息 | Orchestrator |
| ASR 识别错误，用户重新输入 | Pop 掉错误识别的 user 消息，再 Add 正确的 | Orchestrator |
| 工具调用失败需重试 | Pop 掉失败的 tool 消息 | Agent |

Pop/PopN 仅移除消息，不修改消息内容。需要修改已存消息的场景应通过 Pop + 重新 Add 实现。

## 消息存储时机

核心原则：**Session 里的是"如果看聊天记录，你能看到什么"——完整的用户发言和助手回复（含工具调用）。只存最终结果，不存中间状态。**

### 应存入

| 时机 | 消息 | 写入方 |
|------|------|--------|
| ASR 给出最终识别结果 | `{role: user, content: "..."}` | Orchestrator |
| Agent 完成一轮流式输出（文本拼接完毕） | `{role: assistant, content: "..."}` | Agent |
| Agent 决定调用工具 | `{role: assistant, tool_calls: [...]}` | Agent |
| 工具执行完毕返回结果 | `{role: tool, content: "...", status: "completed"}` | Agent |

### 不应存入

- ASR 中间识别结果（interim results）
- Agent 流式输出的中间 chunk
- Agent 内部思考、中间状态
- VAD 事件、音频事件等非对话内容
- 系统提示词（system prompt）—— 由 Agent/Memory 在每次调用时动态附加

## Agent 集成方式

当前 Agent.Process() 内部用局部变量管理消息历史，调用结束即丢失。

### 改动

`Agent.Process()` 签名从：

```go
func (a *Agent) Process(ctx context.Context, input string) (<-chan AgentEvent, error)
```

变为（增加 session 参数，input 已通过 session.Add 提前写入）：

```go
func (a *Agent) Run(ctx context.Context, sess *session.Session) (<-chan AgentEvent, error)
```

### 调用流程

1. Orchestrator 创建/持有 Session
2. 用户输入 → `sess.Add(userMessage)`
3. `agent.Run(ctx, sess)` 读取 session 消息，调用 LLM
4. Agent 将 assistant 回复和 tool 结果写入 session
5. Run 返回后，session 包含完整的本轮对话历史
6. Orchestrator 将 turn 写入 Memory 供提炼

### Memory 交互

Memory.Service 不受影响，继续通过 `BuildContextMessages()` 提供摘要和长期记忆上下文。Agent.Run() 内部将 session 消息与 memory 上下文合并后发送给 LLM。

## 线程安全

Session 自身不加锁。由调用方（Orchestrator）保证串行访问。如需跨 goroutine 共享，使用 Clone() 获取快照。

## 测试要求

- 创建会话、添加消息、ToLLMMessages 转换
- 消息 ID 唯一性
- JSON 序列化/反序列化完整性
- Clone 深拷贝验证
- Pop/PopN 边界条件（空 session、超出长度、部分弹出后消息列表正确）
- LastN 边界条件
