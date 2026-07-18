# Orion-X 重构方案: DAG Pipeline 编排 + 薄 Channel Gateway

## Context

`cmd/wsserver` 当前每个 xiaozhi WS 连接都独立创建 ASR Processor/Agent/TTS Processor 并构建完整的 DAG pipeline。`internal/channels/` 目前只是生命周期管理器，不是真正的薄网关层。`cmd/voicebot` 只是 CLI 测试版，忽略。

目标：DAG pipeline 是核心编排引擎，通道是薄网关只做协议转换。不引入多余编排层。

---

## 架构总览

```
                        ┌── 输入 ──┐
                        │ 网关消息  │
                        └────┬─────┘
                             │ session.Manager (session/ 包)
                             ▼
    ┌──────────────────────────────────────────────────┐
    │               DAG Pipeline (编排引擎)              │
    │        每个 session 一个，channel 自行组装         │
    │                                                   │
    │  ┌──────┐   ┌───────┐   ┌──────┐   ┌──────────┐ │
    │  │ ASR  │→  │ Agent │→  │ TTS  │→  │Output    │ │
    │  │Stage │   │Stage  │   │Stage │   │Stage     │ │
    │  └──┬───┘   └───┬───┘   └──────┘   └─────┬────┘ │
    │     │           │                         │      │
    │     │  SubAgent │  (Mount 挂载)            │      │
    │     └───────────┴─────────────────────────┘      │
    └──────────────────────┬───────────────────────────┘
                           │ pipeline.Message
                           ▼
                     ┌──────────┐
                     │ 网关转发  │ (channel 负责编码 + pacing + 发送)
                     │ 给客户端  │
                     └──────────┘

    包职责:
    ┌────────┐  ┌─────────┐  ┌──────────┐  ┌────────┐  ┌─────────┐
    │session/│  │ agent/  │  │ audio/   │  │ task/  │  │provider/│
    │Session │  │Agent    │  │ASR Stage │  │Task    │  │Pool     │
    │Manager │  │SubAgent │  │TTS Stage │  │Registry│  │(复用)   │
    │        │  │AgentStage│ │OutputStg │  │        │  │         │
    └────────┘  └─────────┘  └──────────┘  └────────┘  └─────────┘

    session/ 不涉及 DAG 构建，只对话状态 + 生命周期管理。
    DAG pipeline 由 channel 自行组装（pkg/pipeline/ DAGBuilder）。
    session.Session.Pipeline 字段由 channel 赋值，供 sub-agent 挂载查询。
```

---

## 核心设计原则

| 原则 | 说明 |
| --- | --- |
| **DAG 为核心编排** | 所有处理节点编排在 DAG pipeline 中，pkg/pipeline/ 提供框架 |
| **无 core 编排层** | session/agent/audio/task/provider 各司其职，自然组合 |
| **通道极薄** | 只做协议转换（Opus编解码 / TG消息收发），不做业务逻辑 |
| **控制流隔离** | 各通道 LLM turn 完全独立，互不阻塞 |
| **Sub-agent 独立运行** | 后台独立 LLM turn，不阻塞任何主 Agent |
| **通道挂载** | 主 Agent 可以把 sub-agent 的输出挂载到当前 session 的 DAG |

---

## 目录结构

```
internal/
├── channels/                            # 薄网关层
│   ├── channel.go                       # Channel interface（不变）
│   ├── manager.go                       # Manager（不变）
│   ├── xiaozhi/                         # WS 协议转换
│   │   ├── channel.go                   # 协议处理 + 音频编解码 + pacing
│   │   ├── config.go                    # 配置加载
│   │   ├── audiopacer.go                # 【移动】从 stages/ 移出
│   │   ├── wsaudiosource.go             # 【移动】从 stages/ 移出
│   │   └── wsproto/
│   │       └── protocol.go
│   └── tg/
│       └── channel.go
│
├── session/                             # 对话数据 + 生命周期管理
│   ├── session.go                       # Session 结构体（已有）
│   ├── manager.go                       # SessionManager（新增，加/删/查）
│   └── types.go                         # 消息类型（已有）
│
├── agent/                               # Agent + SubAgent
│   ├── agent.go                         # Agent（已有）
│   ├── agent_stage.go                   # DAG stage（已有）
│   ├── config.go                        # 配置（已有）
│   └── sub_agent.go                     # SubAgent（新增）
│
├── audio/                               # ASR/TTS + DAG stages
│   ├── asr_stage.go                     # ASR DAG stage（已有）
│   ├── tts_stage.go                     # TTS DAG stage（已有）
│   ├── output_stage.go                  # 【新增】通用 DAG output stage
│   └── ...
│
├── task/                                # Task Registry（新增）
│   ├── task.go
│   └── registry.go
│
├── provider/                            # Provider 池化（新增）
│   ├── pool.go                          # ProviderPool
│   ├── asr/                             # 已有 factory（不变）
│   └── tts/                             # 已有 factory（不变）
│
├── tools/                               # Tool Registry + MCP（不变）
├── memory/                              # Memory（不变）
└── ...
```

**移除**：`internal/channels/xiaozhi/stages/`（内容分散到 channel 和 audio）

> 不做 `internal/core/` 包 — session/agent/audio/task/provider 自然组合，无需额外编排层。

---

## 1. DAG Pipeline — 核心编排引擎

### 重构后的 DAG 拓扑

```
网关(xiaozhi/tg) ──→ session.Manager (session/ 包)
                       │
                 DAG pipeline (per session, channel 自行组装):
                       │
           asr ──┬──→ agent ──→ tts ──┐
                 │                     │
                 └──→ (STT 回显) ──────┤
                                       │
                      agent ──→ (LLM 文本) ─┤
                                       │
                              session_output
                                       │
                       sub-agent 输出 ──┤  (通过 Mount 注入)
                                       │
                              pipeline.Message
                                       │
                             网关转发 (channel 处理)
                        Opus 编码 + pacing + WS send
```

**DAG pipeline 的价值：**

- **流程编排** — ASR 流式输入、VAD 事件、打断传播、LLM 流式输出、TTS 边合成边播
- **可扩展** — 新增 stage（NLP 预处理、翻译、过滤）+ 连边即可
- **解耦** — 每个 stage 只关心自己，消息通过 pipeline channel 传递
- **打断传播** — `MessageTypeInterrupt` 广播到所有 stage

---

## 2. 包设计

### session/ — 对话数据 + 生命周期管理

**session 是对话上下文的边界**，不关心 DAG 拓扑，不关心 pipeline 组装。
DAG 组装在 channel 层，组装好后的 pipeline 引用挂在 `Session.Pipeline` 上。

```go
// session/session.go — 不变，仅新增 Pipeline 可选字段
type Session struct {
    ID        string
    Meta      SessionMeta
    Messages  []Message
    UpdatedAt time.Time

    Pipeline  pipeline.Pipeline    // 可选，channel 组装后赋值
}

// session/manager.go — 新增，纯生命周期管理
type Manager struct {
    sessions map[string]*Session  // key: sessionID
    mu        sync.RWMutex
}

// Manager 提供机制，生命周期策略由 channel 决策：
func NewManager() *Manager
func (m *Manager) Add(sess *Session)
func (m *Manager) Get(id string) (*Session, bool)
func (m *Manager) Remove(id string)
// CloseSession 从 map 移除 + 停 pipeline（如果有）
func (m *Manager) CloseSession(id string)
```

### agent/ — Agent + SubAgent

```go
// agent/agent.go — 已有，不变
// agent/agent_stage.go — 已有，不变
// agent/config.go — 已有，不变

// agent/sub_agent.go — 新增
type SubAgent struct {
    ID        string
    TaskID    string
    agent     *Agent
    cancel    context.CancelFunc
    OutputCh  chan pipeline.Message
}

func NewSubAgent(ctx context.Context, cfg Config, registry *tools.Registry, memSvc *memory.Service) (*SubAgent, error)
func (sa *SubAgent) Start()
func (sa *SubAgent) Stop()
```

### audio/ — ASR/TTS Stages + Output Stage

```go
// audio/asr_stage.go — 已有，不变
// audio/tts_stage.go — 已有，不变

// audio/output_stage.go — 新增
// DAG pipeline 的输出汇合点，将各 stage 输出统一为 pipeline.Message
// 同时接收 sub-agent 挂载过来的输出
```

### task/ — Task Registry

```go
// task/task.go
type Task struct {
    ID              string
    Title           string
    Status          TaskStatus     // Active | Completed | Cancelled
    SubAgentID      string
    MountedSessions []string
    Progress        string
    CreatedAt       time.Time
}

// task/registry.go
type Registry struct {
    tasks     map[string]*Task
    subAgents map[string]*agent.SubAgent
    mu        sync.RWMutex
}

func (r *Registry) Create(req CreateTaskRequest) (*Task, error)
func (r *Registry) Mount(taskID, sessionID string)
func (r *Registry) Dismount(taskID, sessionID string)
func (r *Registry) Search(query string) []*Task
```

### provider/ — Provider Pool

```go
// provider/pool.go
type Pool struct {
    llmClients    map[string]llm.Client       // key: api_key+base_url
    ttsProviders  map[string]tts.Provider
    asrProviders  map[string]asr.Recognizer
    mu            sync.RWMutex
}

func (p *Pool) GetOrCreateTTS(cfg config.TTSConfig) (tts.Provider, error)
func (p *Pool) GetOrCreateASR(cfg config.ASRConfig) (asr.Recognizer, error)
```

---

## 3. Sub-Agent 模型

### 主 Agent vs Sub-Agent

```
主 Agent（每个 Session 一个）
  ├── 日常对话、简单问答
  ├── 检测到复杂任务 → 孵化 sub-agent
  └── 保持轻量

Sub-Agent（后台独立运行）
  ├── 执行复杂任务的完整流程
  ├── 独立 LLM turn，不阻塞任何主 Agent
  ├── 输出通过 Mount 挂载到 session 的 DAG
  └── 完成后自动结束
```

### 主 Agent 工具集

| 工具 | 用途 |
| --- | --- |
| `CreateTask` | 孵化新任务（自动创建 sub-agent） |
| `SearchTasks` | 搜索现有任务 |
| `MountTask` | 挂载任务输出到当前 session |
| `DismountTask` | 取消挂载 |
| `GetTaskProgress` | 查看任务进度 |

### 完整场景流

```
小智: "帮我规划国庆八天去北京的游玩方案"
  → session_xiaozhi 的 DAG 处理
  → 主 Agent: CreateTask("国庆北京旅游规划")
  → task.Registry 创建 task + 孵化 sub-agent (agent 包)
  → sub-agent 后台独立 LLM turn 开始规划
  → 输出默认挂载到 session_xiaozhi
  → 主 Agent 回复"好的，我来帮你规划"

TG: "北京天气怎么样？"
  → session_tg 的 DAG 处理
  → 主 Agent 简单回答，完事

TG: "我那个去北京的旅游方案怎么样了？"
  → 主 Agent: SearchTasks("北京 旅游") → 找到 task
  → MountTask(taskID, "session_tg")
  → sub-agent 输出同时注入两个 session 的 DAG
  → 小智和 TG 都能看到规划进度

期间小智: "帮我查下明天的天气"
  → 主 Agent 正常处理（sub-agent 不受影响）
```

---

## 4. Xiaozhi Channel 重构

### 重构前

```
handleWS → handleConnection
  ├── 加载设备配置
  ├── newRecognizer()          → 独立 ASR
  ├── newTTSProvider()         → 独立 TTS
  ├── audio.NewASRProcessor()
  ├── audio.NewTTSProcessor()
  ├── agent.New()              → 独立 Agent
  ├── pipeline.NewDAGBuilder() → DAG (asr→agent→tts→ws_output)
  │   └── stages: asr_stage, agent_stage, tts_stage, wsoutput_stage
  ├── pipeline.Start()
  └── readLoop
```

### 重构后

```
handleWS → handleConnection
  ├── 加载设备配置（不变）
  ├── session.Manager.Add(sess)            # 先建 session
  ├── provider.Pool.GetOrCreateASR()       # 从池拿 provider
  ├── provider.Pool.GetOrCreateTTS()
  ├── 创建 ASR/TTS Processor（不变）
  ├── 创建 Agent（略有简化）
  ├── pipeline.NewDAGBuilder() → channel 自建 DAG
  │   (ASR → Agent → TTS → WSOutput)
  ├── sess.Pipeline = pl                   # 挂到 session
  ├── pl.Start()
  │
  ├── readLoop
  │   ├── 音频帧 → asrProc.Write()
  │   ├── abort  → pl.Input() ← interrupt
  │   ├── listen → pl.Input() ← text
  │   └── iot/mcp → 协议层处理
  │
  └── close:
      ├── pl.Stop()
      └── session.Manager.CloseSession(sess.ID)
```

**变化总结**：
- `newRecognizer` / `newTTSProvider` 移到 `provider.Pool`（不再对每个连接重复创建）
- `session.Manager` 只做 session 增删查，不涉及 DAG 构建
- `xiaozhi/stages/` 移除 — 其中 `wsoutput.go` 的编码+ pacing 逻辑移到 channel 本身
- WS 协议特有的 `wsproto`、`wsaudiosource`、`audiopacer` 保留在 channel 层

---

## 5. TG Channel 重构

### 重构前

```
handleText:
  ├── 构建 agent.Config
  ├── toolsMgr.Clone()
  ├── agent.New() + agent.Run()
  └── 收集 eventChan → 回复
```

### 重构后

```
handleText:
  ├── sess, ok := session.Manager.Get(sessionID)
  ├── if !ok: sess = session.New(meta); session.Manager.Add(sess)
  ├── sess.Add(RoleUser, text)
  ├── agent.Run(ctx, sess) → 收集文本回复
  └── session.Manager.CloseSession(sess.ID)  # 当前策略：回复完即关

**变化总结**：
- 不再重复创建 agent.Config + agent.New — 通过 session 复用 agent 实例（可选）
- session 生命周期策略由通道决定：当前"用完即关"，未来可改为 idle timeout
```

---

## 实施步骤

### Phase 1: 基础设施

1. `session/manager.go` — SessionManager（Add/Get/Remove/CloseSession）
2. `Session.Pipeline` 可选字段
3. `agent/sub_agent.go` — SubAgent struct + 启动/停止
4. `task/` — Task + Registry
5. `provider/pool.go` — ProviderPool

### Phase 2: 集成

6. `audio/output_stage.go` — 通用 DAG output stage
7. 重构 `xiaozhi/channel.go`: `newRecognizer`/`newTTSProvider` → `provider.Pool`，DAG 组装不变
8. 将 `xiaozhi/stages/wsoutput.go` 中的编码 + pacing 逻辑移到 channel 层
9. 重构 `tg/channel.go` — 通过 `session.Manager` 管理 session 生命周期
10. 主 Agent 工具集注入 CreateTask/SearchTasks/MountTask/DismountTask

### Phase 3: 清理

11. 移除 `xiaozhi/stages/` 目录
12. 移除 `newRecognizer` / `newTTSProvider`（已在 provider pool 中）
13. 更新测试
14. `golangci-lint run ./...`

---

## 边界情况

| 场景 | 处理 |
| --- | --- |
| 两通道同时输入 | 各自主 Agent 独立 turn，sub-agent 不受影响 |
| Sub-agent 输出时主 Agent 忙 | Sub-agent 输出不打断，当前 turn 结束后显示 |
| 用户断开重连 | Session 数据保留在 Manager，pipeline 需重建（channel 负责）。sub-agent 独立运行不受影响 |
| Provider 连接失败 | ProviderPool 重试/降级 |

---

## 验证

1. 现有 xiaozhi WS 协议保持兼容
2. 多设备并发各自独立
3. TG 创建任务 → TG 自己可以挂载查看
4. 小智继续日常对话，sub-agent 不受影响
5. 跨通道查任务 + 挂载输出
6. `golangci-lint run ./...`
