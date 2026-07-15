# Pipeline - 流式数据处理框架

## 概述

`internal/pipeline` 是一个通用的流式数据处理框架，用于构建可组合、可测试、可观测的数据处理管道。

## 核心概念

### Message

Pipeline 中流转的数据单元：

```go
msg := pipeline.NewMessage(pipeline.MessageTypeTextChunk, "hello")
msg = msg.WithMetadata(pipeline.Metadata{
    TurnID:  123,
    TraceID: "trace-abc",
    Emotion: "happy",
})
```

### Stage

Pipeline 中的处理节点，接收输入流，产生输出流：

```go
type Stage interface {
    Name() string
    Process(ctx context.Context, input <-chan Message) <-chan Message
}
```

### Pipeline

Stage 的编排器：

```go
pipeline := pipeline.NewBuilder().
    AddStage(stage1).
    AddStage(stage2).
    SetObserver(observer).
    Build()

ctx := context.Background()
pipeline.Start(ctx)
defer pipeline.Stop()
```

## 快速开始

### 创建简单 Pipeline

```go
package main

import (
    "context"
    "fmt"
    
    "github.com/liuscraft/orion-x/internal/pipeline"
)

func main() {
    // 创建 Pipeline
    p := pipeline.NewBuilder().
        AddStage(pipeline.NewTextFilterStage()).
        AddStage(pipeline.NewEmotionExtractorStage()).
        Build()
    
    ctx := context.Background()
    p.Start(ctx)
    defer p.Stop()
    
    // 发送消息
    go func() {
        p.Input() <- pipeline.NewMessage(
            pipeline.MessageTypeTextChunk,
            "I'm <emotion>happy</emotion> today <metadata>extra</metadata>",
        )
    }()
    
    // 接收输出
    msg := <-p.Output()
    fmt.Printf("Emotion: %s, Text: %s\n", msg.Metadata.Emotion, msg.Payload)
}
```

### 实现自定义 Stage

```go
type MyStage struct {
    *pipeline.BaseStage
}

func NewMyStage() pipeline.Stage {
    return &MyStage{
        BaseStage: pipeline.NewBaseStage("my_stage"),
    }
}

func (s *MyStage) Process(ctx context.Context, input <-chan pipeline.Message) <-chan pipeline.Message {
    output := make(chan pipeline.Message)
    
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
                
                // 处理消息
                // ...
                
                // 发送到下游
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

## 特性

### 统一错误处理

错误作为 Message 的一部分流转：

```go
msg := msg.WithError(fmt.Errorf("something went wrong"))

// 下游 Stage 可以检查错误
if msg.IsError() {
    // 处理错误
}
```

### 打断支持

```go
pipeline.Interrupt()  // 立即取消所有 Stage
```

### 可观测性

```go
observer := pipeline.NewLoggingObserver(true)
pipeline := pipeline.NewBuilder().
    SetObserver(observer).
    Build()

// 自动记录：
// - Stage 启动/停止
// - 消息流转
// - 错误发生
```

### Context 传播

所有 Stage 共享同一个 context，支持：
- 超时控制
- 取消传播
- 值传递

## 内置 Stage

### 框架自带（示例）

#### TextFilterStage

过滤文本中的特殊标签：

```go
stage := pipeline.NewTextFilterStage()
// "hello <metadata>world</metadata>" -> "hello "
```

#### EmotionExtractorStage

提取情感标签到 Metadata：

```go
stage := pipeline.NewEmotionExtractorStage()
// "I'm <emotion>happy</emotion>" -> Metadata.Emotion = "happy"
```

### 业务 Stage（`stages/` 包）

#### AgentStage

LLM Agent 处理：

```go
import "github.com/liuscraft/orion-x/internal/pipeline/stages"

stage := stages.NewAgentStage(agent)
```

#### TTSStage

文本转语音：

```go
stage := stages.NewTTSStage(audioOutPipe)
```

#### ToolExecutorStage

工具执行：

```go
stage := stages.NewToolExecutorStage(toolExecutor, agent)
```

#### ASRStage

语音识别（Source Stage）：

```go
stage := stages.NewASRStage(audioInPipe)
```

详见 [`stages/README.md`](stages/README.md)

## 测试

```bash
go test ./internal/pipeline/
```

### 测试自定义 Stage

```go
func TestMyStage(t *testing.T) {
    stage := NewMyStage()
    input := make(chan pipeline.Message, 1)
    ctx := context.Background()
    
    output := stage.Process(ctx, input)
    
    input <- pipeline.NewMessage(pipeline.MessageTypeTextChunk, "test")
    
    msg := <-output
    // assert...
}
```

## 最佳实践

### 1. Stage 应该是无状态的

避免在 Stage 中存储可变状态，使用 Message.Metadata 传递上下文信息。

### 2. 及时响应 Context 取消

```go
select {
case <-ctx.Done():
    return
case output <- msg:
}
```

### 3. 使用 defer close(output)

确保输出 channel 被关闭，避免下游 Stage 阻塞。

### 4. 错误传播而非中断

遇到可恢复的错误，使用 `msg.WithError()` 传播给下游，而不是直接 return。

## 性能考虑

- **Channel 缓冲**：Source Stage 使用有缓冲 channel，中间 Stage 使用无缓冲 channel
- **背压（Backpressure）**：当下游处理慢时，上游自动阻塞
- **并发**：每个 Stage 在独立 goroutine 中运行
- **内存**：大 Payload（如音频数据）使用引用传递

## 架构对比

| 特性 | EventBus | Pipeline |
|------|----------|----------|
| 数据流 | 发布-订阅 | 线性流 |
| 错误处理 | 分散 | 统一 |
| 打断 | 手动取消多个组件 | 取消 context |
| 扩展性 | 需改多处代码 | 添加新 Stage |
| 测试 | 需 mock EventBus | 每个 Stage 独立测试 |

## 下一步

- [ ] 实现 ASRStage（包装现有 ASR）
- [ ] 实现 AgentStage（包装现有 Agent）
- [ ] 实现 TTSStage（包装现有 TTS）
- [ ] 创建 VoicebotPipeline（替代 Orchestrator）
- [ ] 性能基准测试和优化

## 相关文档

- [Pipeline 设计文档](../../docs/pipeline-design.md)
