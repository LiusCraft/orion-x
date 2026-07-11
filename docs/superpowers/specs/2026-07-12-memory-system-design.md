# 记忆系统设计文档

## 概述

对标 Hermes Agent 记忆架构，在 Orion-X 中实现完整的记忆系统。每个 Device 拥有独立记忆空间，Agent 通过 memory tool 主动管理，后台自省引擎被动补全，结构化摘要压缩长对话。

## 架构

```
┌──────────────────────────────────────────────────────────────┐
│                        Manager (Gin/GORM)                    │
│                                                              │
│  PostgreSQL:                                                 │
│    memory_entries    ← 设备梳理记忆                         │
│    session_turns     ← 对话历史 + FTS 检索                  │
│                                                              │
│  Internal API (RESTful):                                     │
│    GET/PUT /internal/devices/{id}/memory                     │
│    POST /internal/devices/{id}/turns                         │
│    GET  /internal/devices/{id}/turns?q=xxx                   │
│    GET  /internal/devices/{id}/sessions                      │
│    GET  /internal/devices/{id}/sessions/{sid}                │
└──────────────────────────┬───────────────────────────────────┘
                           │ HTTP (内网)
┌──────────────────────────▼───────────────────────────────────┐
│                    wsserver (每连接)                          │
│                                                              │
│  ┌──────────────┐   ┌─────────────────┐   ┌──────────────┐  │
│  │ CuratedStore  │   │ BackgroundReview │   │ Compressor   │  │
│  │ (内存缓存+)   │   │ (后台 goroutine) │   │ (结构化摘要)  │  │
│  │ 冻结快照)     │   │                 │   │              │  │
│  └──────┬───────┘   └────────┬────────┘   └──────┬───────┘  │
│         │                    │                    │          │
│  ┌──────▼────────────────────▼────────────────────▼───────┐  │
│  │                    Agent (LLM)                          │  │
│  │                                                         │  │
│  │  Built-in tools:  memory / session_search               │  │
│  └─────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘
```

## 组件设计

### 1. Manager 数据模型

#### memory_entries 表

| 字段 | 类型 | 说明 |
| ------ | ------ | ------ |
| id | varchar(36) PK | UUID |
| device_id | varchar(128) | 设备 ID |
| target | varchar(16) | `memory` \| `user` |
| content | text | 条目内容 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 更新时间 |

索引：`(device_id, target)`

#### session_turns 表

| 字段 | 类型 | 说明 |
| ------ | ------ | ------ |
| id | bigserial PK | 自增 |
| device_id | varchar(128) | 设备 ID |
| session_id | varchar(64) | 会话 ID |
| turn_id | bigint | 轮次 ID |
| user_text | text | 用户文本 |
| assistant_text | text | 助手文本 |
| started_at | timestamptz | 开始时间 |
| ended_at | timestamptz | 结束时间 |
| aborted | boolean | 是否打断 |
| created_at | timestamptz | 记录时间 |

索引：`(device_id, created_at DESC)`, `(device_id, session_id)`

FTS 索引：`gin(to_tsvector('simple', coalesce(user_text,'') || ' ' || coalesce(assistant_text,'')))`

### 2. Manager Internal API

#### 记忆

```
GET  /internal/devices/{device_id}/memory
  → 200 { entries: { memory: [{ content, created_at }, ...],
                     user: [{ content, created_at }, ...] },
           usage: { memory: { used: int, limit: 2200 },
                    user: { used: int, limit: 1375 } } }

PUT  /internal/devices/{device_id}/memory
  Body: { entries: [{ target: "memory"|"user", content: string }, ...] }
  → 204 No Content（原子替换全部）
```

#### 对话历史

```
POST /internal/devices/{device_id}/turns
  Body: { session_id, turn_id, user_text, assistant_text, started_at, ended_at, aborted }
  → 201 Created

GET  /internal/devices/{device_id}/turns?q=关键字&limit=3
  → FTS 搜索（discover 模式）
  → 200 { results: [{ session_id, snippet, matched_role,
                       messages: [{ id, role, content, timestamp }, ...],
                       bookend_start: [...], bookend_end: [...] }, ...] }

GET  /internal/devices/{device_id}/turns?session_id=xxx&around_message_id=123&window=5
  → Scroll 模式
  → 200 { messages: [...], messages_before: N, messages_after: N }

GET  /internal/devices/{device_id}/sessions
  → Browse 模式
  → 200 { sessions: [{ session_id, started_at, message_count, preview }, ...] }
```

### 3. CuratedStore（wsserver 端）

每个连接独立实例，生命周期：

1. **Load()** — 连接建立时 `GET /internal/devices/{device_id}/memory`，加载到内存
2. **冻结快照** — 编译为 system prompt 块，整会话不变
3. **Tool 操作** — add/replace/remove/batch → 更新内存 + 同步 `PUT` Manager
4. **FormatForSystemPrompt()** — 返回冻结快照（system prompt 注入）
5. **Close()** — 连接断开时确保同步

#### 冻结快照格式

```
════════════════════ 记忆(环境/项目笔记) [67% — 1,474/2,200] ════════════════════
用户运行 macOS，使用 Homebrew
§
项目 ~/code/api 使用 Go 1.22
§
测试通过 'make test' 运行

════════════════════ 用户画像 [58% — 800/1,375] ════════════════════
偏好简洁回复
§
中文交流
```

#### Memory Tool Schema

- `action`: add | replace | remove
- `target`: memory | user
- `content`: 条目文本（add/replace）
- `old_text`: 子串匹配（replace/remove）

工具描述中详细说明：**什么该存、什么不该存、容量怎么管理**。成功响应带 `done:true` 防止重复。超额报错 + 返回当前列表 → 迫使 Agent 同轮次合并。

### 4. Background Review

每轮对话完成后启动 goroutine：

1. 构建 review prompt（Hermes 1:1）："用户是否透露了偏好/需求？"
2. 调用 LLM（可配便宜模型）
3. LLM 可能调用 memory tool → 写入 Manager
4. 记录通知：`💾 Memory updated`

### 5. Context Compressor

上下文窗口超限时触发（默认 70%）：

1. 分离 head（system + 记忆快照） + middle（可压缩历史） + tail（最近约 8K tokens）
2. 结构化 LLM 摘要生成：
   - `## Historical Task Snapshot`
   - `## Historical In-Progress State`
   - `## Historical Pending User Asks`
   - `## Historical Remaining Work`
3. restrictive prefix："仅作参考，以最新用户消息为准"
4. 迭代更新：旧摘要喂回 LLM，信息累加
5. 工具输出修剪 + 尾保护

### 6. wsserver 集成

```go
// 连接建立流程
deviceID := hello.DeviceID
curatedStore := memory.NewCuratedStore(managerURL, deviceID, memoryCharLimit, userCharLimit)

// 注册 Agent built-in tool
agent.RegisterBuiltinTool(memory.NewMemoryTool(curatedStore))
agent.RegisterBuiltinTool(session_search.NewSessionSearchTool(managerURL, deviceID))

// 记忆快照注入 system prompt
func buildContextMessages(...) {
    messages = append(systemPrompt)
    if memoryBlock := curatedStore.FormatForSystemPrompt("memory"); memoryBlock != "" {
        messages = append(memoryBlock)
    }
    if userBlock := curatedStore.FormatForSystemPrompt("user"); userBlock != "" {
        messages = append(userBlock)
    }
    messages = append(history...)
}

// 每轮对话完成后
go backgroundReview.Spawn(ctx, snapshot)  // 后台自省
recordTurn(ctx, turn)  // POST /internal/devices/{id}/turns
compressor.CheckAndCompress(ctx, sess)   // 检查并压缩
```

## 文件变更清单

### 新增

| 文件 | 职责 |
| ------ | ------ |
| `internal/store/memory.go` | MemoryEntry Store（GORM CRUD） |
| `internal/store/turn.go` | SessionTurn Store（GORM + FTS） |
| `cmd/manager/handler/memory.go` | Memory API handler |
| `cmd/manager/handler/turn.go` | Turn API handler |
| `internal/memory/curated_store.go` | CuratedStore（缓存+快照+HTTP同步） |
| `internal/memory/compressor.go` | ContextCompressor（结构化摘要） |
| `internal/memory/background_review.go` | BackgroundReview（后台自省） |
| `internal/tools/memory_tool.go` | Memory Tool（Schema + Handler） |
| `internal/tools/session_search_tool.go` | SessionSearch Tool（Schema + Handler） |

### 修改

| 文件 | 改动 |
| ------ | ------ |
| `internal/store/db.go` | 新增 `memory_entries`、`session_turns` 表 AutoMigrate |
| `cmd/manager/server.go` | 注册新路由 |
| `internal/memory/service.go` | 改为 CuratedStore + BackgroundReview + Compressor 组合 |
| `internal/memory/types.go` | 移除 Store 接口，保留 Turn/MemoryItem |
| `internal/config/config.go` | 新增 MemoryReviewConfig、CompressionConfig |
| `cmd/wsserver/main.go` | 改为 HTTP client 模式 |
| `cmd/wsserver/connection.go` | 每个连接独立 CuratedStore |
| `internal/agent/agent.go` | 注册 built-in tool 接口 |

### 删除

| 文件 | 理由 |
| ------ | ------ |
| `internal/memory/sqlite_store.go` | 本地 SQLite 不再使用 |
| `internal/memory/llm.go` | `llmExtractor` 由 BackgroundReview 替代 |
| `internal/memory/session_buffer.go` | 由 ContextCompressor 替代 |
| `internal/memory/context.go` | DeviceID 在 CuratedStore 中直接持有 |

## 遗留问题与后续

- Background Review 的 write_approval（写入审批）暂不实现，后续按需添加
- 外部 MemoryProvider（Honcho/Mem0 等）预留 Provider 接口，后续扩展
- session_turns FTS 查询在数据量大时需要分区或分表
