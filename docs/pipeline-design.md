# Pipeline 架构设计

## 概述

Pipeline 是一个通用的流式数据处理框架，用于替代现有的 EventBus + StateMachine 架构。它提供了：
- 统一的错误处理和传播机制
- 可组合的 Stage 处理节点
- 内置的打断（Interrupt）支持
- 元数据贯穿整个处理链

## 设计目标

1. **简化复杂度**：移除 EventBus，状态由数据流驱动
2. **错误透明**：错误作为 Message 的一部分流转，可被任何 Stage 处理
3. **易于测试**：每个 Stage 独立可测，Pipeline 可组合测试
4. **高扩展性**：新功能只需添加新 Stage，无需修改核心代码

## 核心概念

### 1. Message

Pipeline 中流转的数据单元，包含：
- `Type`: 消息类型（TextChunk, AudioData, ToolCall, Error 等）
- `Payload`: 实际数据
- `Metadata`: 上下文信息（TurnID, TraceID, Emotion 等）
- `Error`: 错误信息（如果有）

### 2. Stage

Pipeline 中的处理节点，接收输入流，产生输出流：
```
Input Stream → Stage.Process() → Output Stream
```

Stage 特性：
- 无状态（stateless）或自包含状态
- 可并发执行
- 支持 Context 取消

### 3. Pipeline

Stage 的编排器，负责：
- 连接多个 Stage
- 管理生命周期（Start/Stop/Interrupt）
- 提供可观测性（Observer）

## 接口设计

### Message

```go
type MessageType string

const (
    MessageTypeTextChunk    MessageType = "text_chunk"
    MessageTypeAudioData    MessageType = "audio_data"
    MessageTypeToolCall     MessageType = "tool_call"
    MessageTypeToolResult   MessageType = "tool_result"
    MessageTypeEmotion      MessageType = "emotion"
    MessageTypeFinished     MessageType = "finished"
    MessageTypeError        MessageType = "error"
)

type Message struct {
    Type     MessageType
    Payload  interface{}
    Metadata Metadata
}

type Metadata struct {
    TurnID    int64
    TraceID   string
    Emotion   string
    Timestamp time.Time
    Error     error  // 错误信息
    Extra     map[string]interface{} // 扩展字段
}
```

### Stage

```go
type Stage interface {
    // Name 返回 Stage 名称（用于日志/metrics）
    Name() string
    
    // Process 处理输入流，返回输出流
    // ctx 用于取消和超时控制
    // input 可能为 nil（对于源 Stage，如 AudioInput）
    Process(ctx context.Context, input <-chan Message) <-chan Message
}

// PassthroughStage 透传 Stage，用于插入 metrics/logging
type PassthroughStage interface {
    Stage
    OnMessage(msg Message) Message  // 可修改/过滤消息
}
```

### Pipeline

```go
type Pipeline interface {
    // Start 启动 Pipeline
    Start(ctx context.Context) error
    
    // Stop 停止 Pipeline（优雅关闭）
    Stop() error
    
    // Interrupt 立即打断当前处理
    Interrupt() error
    
    // Output 获取 Pipeline 输出流
    Output() <-chan Message
    
    // Send 向 Pipeline 发送输入（对于交互式场景）
    Send(msg Message) error
}

// PipelineBuilder 用于构建 Pipeline
type PipelineBuilder interface {
    AddStage(stage Stage) PipelineBuilder
    AddPassthrough(pt PassthroughStage) PipelineBuilder
    SetObserver(observer PipelineObserver) PipelineBuilder
    Build() Pipeline
}

// PipelineObserver 观察 Pipeline 事件
type PipelineObserver interface {
    OnMessage(stageName string, msg Message)
    OnError(stageName string, err error)
    OnStageStart(stageName string)
    OnStageStop(stageName string)
}
```

## 数据流

### 正常流程

```
用户输入
  ↓
AudioInputStage → Message{Type: AudioData}
  ↓
ASRStage → Message{Type: TextChunk, Payload: "你好"}
  ↓
AgentStage → Message{Type: TextChunk, Payload: "你好！"}
            → Message{Type: ToolCall, Payload: {...}}
  ↓
TTSStage → Message{Type: AudioData, Payload: []byte}
  ↓
AudioOutputStage → 播放
  ↓
Message{Type: Finished}
```

### 错误流程

```
任何 Stage 出错
  ↓
Message{Type: Error, Metadata.Error: err}
  ↓
下游 Stage 可选择：
  1. 处理错误（恢复/重试）
  2. 传播错误（继续传递）
  3. 忽略错误（继续处理）
```

### 打断流程

```
用户打断（Interrupt）
  ↓
Pipeline.Interrupt()
  ↓
取消所有 Stage 的 context
  ↓
各 Stage 停止处理，清理资源
  ↓
Pipeline 返回到空闲状态
```

## Stage 实现规范

### 基础 Stage

```go
type baseStage struct {
    name string
}

func (s *baseStage) Name() string {
    return s.name
}

// 示例：文本过滤 Stage
type TextFilterStage struct {
    baseStage
}

func (s *TextFilterStage) Process(ctx context.Context, input <-chan Message) <-chan Message {
    output := make(chan Message)
    
    go func() {
        defer close(output)
        
        for {
            select {
            case <-ctx.Done():
                return
            case msg, ok := <-input:
                if !ok {
                    return
                }
                
                // 只处理文本消息
                if msg.Type == MessageTypeTextChunk {
                    // 过滤逻辑
                    filtered := filterText(msg.Payload.(string))
                    msg.Payload = filtered
                }
                
                // 传递到下游
                select {
                case output <- msg:
                case <-ctx.Done():
                    return
                }
            }
        }
    }()
    
    return output
}
```

### Source Stage（无输入）

```go
type AudioInputStage struct {
    baseStage
    source audiosource.AudioSource
}

func (s *AudioInputStage) Process(ctx context.Context, input <-chan Message) <-chan Message {
    output := make(chan Message)
    
    go func() {
        defer close(output)
        
        // 从音频源读取数据
        for {
            select {
            case <-ctx.Done():
                return
            default:
                data, err := s.source.Read()
                if err != nil {
                    output <- Message{
                        Type: MessageTypeError,
                        Metadata: Metadata{Error: err},
                    }
                    return
                }
                
                output <- Message{
                    Type:    MessageTypeAudioData,
                    Payload: data,
                }
            }
        }
    }()
    
    return output
}
```

### Sink Stage（无输出）

```go
type AudioOutputStage struct {
    baseStage
    sink audiosink.AudioSink
}

func (s *AudioOutputStage) Process(ctx context.Context, input <-chan Message) <-chan Message {
    // Sink 不产生输出，返回 nil 或 empty channel
    go func() {
        for {
            select {
            case <-ctx.Done():
                return
            case msg, ok := <-input:
                if !ok {
                    return
                }
                
                if msg.Type == MessageTypeAudioData {
                    s.sink.Write(msg.Payload.([]byte))
                }
            }
        }
    }()
    
    return nil
}
```

## Pipeline 实现

### 简单线性 Pipeline

```go
type linearPipeline struct {
    stages   []Stage
    input    chan Message
    output   chan Message
    ctx      context.Context
    cancel   context.CancelFunc
    observer PipelineObserver
    wg       sync.WaitGroup
}

func (p *linearPipeline) Start(ctx context.Context) error {
    p.ctx, p.cancel = context.WithCancel(ctx)
    
    // 连接各 Stage
    var currentOutput <-chan Message = p.input
    
    for _, stage := range p.stages {
        if p.observer != nil {
            p.observer.OnStageStart(stage.Name())
        }
        
        stageOutput := stage.Process(p.ctx, currentOutput)
        currentOutput = stageOutput
    }
    
    // 最后一个 Stage 的输出作为 Pipeline 输出
    p.output = make(chan Message)
    p.wg.Add(1)
    go func() {
        defer p.wg.Done()
        defer close(p.output)
        
        for msg := range currentOutput {
            p.output <- msg
        }
    }()
    
    return nil
}

func (p *linearPipeline) Interrupt() error {
    if p.cancel != nil {
        p.cancel()
    }
    return nil
}

func (p *linearPipeline) Output() <-chan Message {
    return p.output
}

func (p *linearPipeline) Send(msg Message) error {
    select {
    case p.input <- msg:
        return nil
    case <-p.ctx.Done():
        return p.ctx.Err()
    }
}
```

## 与 Orchestrator 集成

### 新旧架构对比

| 旧架构（Orchestrator） | 新架构（Pipeline） |
|------------------------|-------------------|
| EventBus.Publish() | Send(Message) |
| EventBus.Subscribe() | Stage.Process() |
| StateMachine.Transition() | 隐式（由数据流驱动） |
| handleASRFinal() | ASRStage 输出 |
| handleAgentEvent() | AgentStage 输出 |
| OnTTSPlaybackFinished() | TTSStage 输出 Finished |

### 集成方式

```go
// Voicebot Pipeline 组装
func NewVoicebotPipeline(cfg config.Config) Pipeline {
    builder := NewPipelineBuilder()
    
    // 1. 音频输入 + ASR
    builder.AddStage(NewAudioInputStage(cfg.Audio))
    builder.AddStage(NewASRStage(cfg.ASR))
    
    // 2. Agent 处理
    builder.AddStage(NewAgentStage(cfg.Agent, cfg.Tools))
    
    // 3. 文本处理
    builder.AddStage(NewTextFilterStage())
    builder.AddStage(NewEmotionExtractorStage())
    
    // 4. TTS + 音频输出
    builder.AddStage(NewTTSStage(cfg.TTS))
    builder.AddStage(NewAudioOutputStage(cfg.Audio))
    
    // 5. 可观测性
    builder.SetObserver(NewLoggingObserver())
    
    return builder.Build()
}
```

## 文件结构

```
internal/pipeline/
├── pipeline.go           # Pipeline 接口和实现
├── stage.go              # Stage 接口和基础实现
├── message.go            # Message 和 Metadata 定义
├── builder.go            # PipelineBuilder 实现
├── observer.go           # PipelineObserver 接口
├── linear_pipeline.go    # 线性 Pipeline 实现
├── pipeline_test.go
├── stage_test.go
└── examples/
    └── simple_pipeline_test.go
```

## 测试策略

### 单元测试

```go
func TestTextFilterStage(t *testing.T) {
    stage := NewTextFilterStage()
    input := make(chan Message, 1)
    ctx := context.Background()
    
    output := stage.Process(ctx, input)
    
    input <- Message{
        Type:    MessageTypeTextChunk,
        Payload: "hello <metadata>world</metadata>",
    }
    close(input)
    
    msg := <-output
    assert.Equal(t, "hello world", msg.Payload)
}
```

### 集成测试

```go
func TestVoicebotPipeline(t *testing.T) {
    pipeline := NewVoicebotPipeline(testConfig)
    
    ctx := context.Background()
    pipeline.Start(ctx)
    
    // 发送音频输入
    pipeline.Send(Message{
        Type:    MessageTypeAudioData,
        Payload: testAudioData,
    })
    
    // 验证输出
    msg := <-pipeline.Output()
    assert.Equal(t, MessageTypeAudioData, msg.Type)
}
```

## 性能考虑

### Channel 缓冲

- Source Stage 使用有缓冲 channel，避免阻塞
- 中间 Stage 使用无缓冲 channel，保持背压（backpressure）
- Sink Stage 使用有缓冲 channel，避免丢数据

### 并发控制

- 每个 Stage 在独立 goroutine 中运行
- 使用 context 统一取消
- 使用 WaitGroup 等待所有 goroutine 退出

### 内存管理

- Message 复用对象池（如果需要）
- 大 Payload（音频数据）使用引用传递
- 及时关闭 channel，释放资源

## 迁移计划

### 阶段 1：核心框架（本次实现）
- [ ] 定义 Message/Stage/Pipeline 接口
- [ ] 实现 LinearPipeline
- [ ] 实现 PipelineBuilder
- [ ] 单元测试

### 阶段 2：Stage 实现（后续 PR）
- [ ] ASRStage（包装现有 ASR）
- [ ] AgentStage（包装现有 Agent）
- [ ] TTSStage（包装现有 TTS）
- [ ] TextFilterStage
- [ ] EmotionExtractorStage

### 阶段 3：集成（后续 PR）
- [ ] 创建 VoicebotPipeline
- [ ] 替换 Orchestrator
- [ ] 性能测试和优化

## 设计决策

### 为什么使用 Channel 而非接口回调？

**优势**：
- 天然支持背压（backpressure）
- 取消和超时控制简单（context）
- 并发安全，无需显式锁
- 符合 Go 的并发模型

**劣势**：
- 额外的 goroutine 开销（可接受）
- 调试稍复杂（可用 pprof）

### 为什么不使用 Actor 模型？

Actor 模型更适合：
- 有大量独立实体（entity）
- 复杂的状态管理
- 分布式系统

我们的场景：
- 流式数据处理
- 线性或树形拓扑
- 单机部署

Pipeline 更简单、更直观。

### 为什么 Metadata 包含 Error？

统一错误处理方式：
- 错误作为数据流的一部分
- 下游 Stage 可选择处理或忽略
- 避免单独的错误 channel

## 参考资料

- [Go Concurrency Patterns: Pipelines and cancellation](https://go.dev/blog/pipelines)
- [Reactive Streams Specification](https://www.reactive-streams.org/)
- CloudWeGo Eino 的 Compose 模式
