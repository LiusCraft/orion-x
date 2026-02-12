# Agent 记忆能力设计文档

## 1. 目标
在现有语音对话链路中引入记忆能力，支持三种模式：
- **none**：不保存、不注入上下文
- **session**：仅当前会话内有效
- **long_term**：持久化存储，跨会话可检索

## 2. 设计原则
- 自顶向下，优先保证流程可用
- 与现有 `voicebot/orchestrator` 事件流最小耦合
- 记忆能力可按配置开关
- 避免“记忆错误”扩散，注入内容需标注“仅供参考”

## 3. 接口定义

### 3.1 Memory 模式
```go
// internal/memory/types.go

type Mode string
const (
    ModeNone     Mode = "none"
    ModeSession  Mode = "session"
    ModeLongTerm Mode = "long_term"
)
```

### 3.2 Context / Turn / MemoryItem
```go
type Context struct {
    UserID    string
    SessionID string
    DeviceID  string
}

type Turn struct {
    TurnID        int64
    UserText      string
    AssistantText string
    StartedAt     time.Time
    EndedAt       time.Time
    Aborted       bool
    UserID        string
    SessionID     string
    DeviceID      string
}

type MemoryItem struct {
    ID         int64
    UserID     string
    Content    string
    Type       string
    Importance int
    CreatedAt  time.Time
    ExpiresAt  *time.Time
}
```

### 3.3 Store / Service 接口
```go
type Store interface {
    SaveTurn(turn Turn) error
    SaveItems(items []MemoryItem) error
    Query(userID, query string, limit int, minScore float64) ([]MemoryItem, error)
    Purge(now time.Time, retentionDays int) error
    Close() error
}

type Service interface {
    BuildContextMessages(ctx context.Context, userText string) ([]*schema.Message, error)
    RecordTurn(ctx context.Context, turn Turn) error
    Close() error
}
```

## 4. 数据流
```
ASRFinal → Orchestrator
         ├─ 初始化 Turn（UserText + StartedAt）
         ├─ 调用 VoiceAgent.Process
         │      ├─ MemoryService.BuildContextMessages
         │      │      ├─ LongTerm Query (SQLite FTS)
         │      │      └─ Session History
         │      └─ LLM 输出
         ├─ TextChunkEvent 累积 AssistantText
         └─ TTS 完成后触发 RecordTurn
                 ├─ SessionBuffer.Add
                 ├─ SaveTurn
                 └─ Extract + SaveItems
```

## 5. 状态与边界条件
- **用户打断**：标记 `turn.Aborted=true`，长期记忆不落盘。
- **无 TTS 输出**：不会进入 Speaking；RecordTurn 仍可在 Agent 结束后触发（但当前实现以 TTS 完成为主）。
- **无 LLM APIKey**：摘要/抽取逻辑自动跳过。

## 6. 持久化模型
### 表结构
- `turns`：保存完整对话（调试用途）
- `memories`：长期记忆实体
- `memories_fts`：FTS5 索引

### 检索逻辑
- `FTS5` + `bm25` 排序
- 二级排序：`importance`、`created_at`

## 7. 模块依赖
```
voicebot/orchestrator ──> memory.Service
agent/voice_agent_impl ──> memory.Service
wsserver/session ──> memory.Context + memory.Service
```

## 8. 风险与后续迭代
- 记忆抽取存在误差 → 需保留“仅供参考”提示
- SQLite FTS 在数据量增长后需要分区或升级向量检索
- 后续可增加：向量检索、跨设备账户级记忆
