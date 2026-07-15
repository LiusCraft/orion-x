# Pipeline 快速开始

## 5 分钟上手 Pipeline

### 1. 理解核心概念

```
Message → Stage → Message → Stage → Message
```

- **Message**：数据单元（文本、音频、工具调用等）
- **Stage**：处理节点（转换、过滤、调用外部服务）
- **Pipeline**：Stage 的组合

### 2. 创建第一个 Pipeline

```go
package main

import (
    "context"
    "fmt"
    
    "github.com/liuscraft/orion-x/internal/pipeline"
)

func main() {
    // 构建 Pipeline
    p := pipeline.NewBuilder().
        AddStage(pipeline.NewTextFilterStage()).
        Build()
    
    ctx := context.Background()
    p.Start(ctx)
    defer p.Stop()
    
    // 发送消息
    go func() {
        p.Input() <- pipeline.NewMessage(
            pipeline.MessageTypeTextChunk,
            "Hello <metadata>world</metadata>!",
        )
    }()
    
    // 接收输出
    msg := <-p.Output()
    fmt.Println(msg.Payload) // "Hello !"
}
```

### 3. 使用业务 Stage

```go
import (
    "github.com/liuscraft/orion-x/internal/pipeline"
    "github.com/liuscraft/orion-x/internal/pipeline/stages"
)

// 完整的 Voicebot Pipeline
p := pipeline.NewBuilder().
    // 1. 音频输入 + ASR
    AddStage(stages.NewASRStage(audioInPipe)).
    
    // 2. LLM Agent
    AddStage(stages.NewAgentStage(agent)).
    
    // 3. 工具执行
    AddStage(stages.NewToolExecutorStage(toolExecutor, agent)).
    
    // 4. 文本处理
    AddStage(pipeline.NewTextFilterStage()).
    AddStage(pipeline.NewEmotionExtractorStage()).
    
    // 5. TTS
    AddStage(stages.NewTTSStage(audioOutPipe)).
    
    // 6. 观察者（可选）
    SetObserver(pipeline.NewLoggingObserver(true)).
    Build()

p.Start(ctx)
defer p.Stop()

// Pipeline 自动运行，从 ASR 接收输入，输出到 TTS
```

### 4. 创建自定义 Stage

```go
type UppercaseStage struct {
    *pipeline.BaseStage
}

func NewUppercaseStage() pipeline.Stage {
    return &UppercaseStage{
        BaseStage: pipeline.NewBaseStage("uppercase"),
    }
}

func (s *UppercaseStage) Process(ctx context.Context, input <-chan pipeline.Message) <-chan pipeline.Message {
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
                
                // 转换文本为大写
                if msg.Type == pipeline.MessageTypeTextChunk {
                    if text, ok := msg.Payload.(string); ok {
                        msg.Payload = strings.ToUpper(text)
                    }
                }
                
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

// 使用
p := pipeline.NewBuilder().
    AddStage(NewUppercaseStage()).
    Build()
```

### 5. 错误处理

```go
// Pipeline 会自动传播错误
for msg := range p.Output() {
    if msg.IsError() {
        fmt.Printf("Error: %v\n", msg.Metadata.Error)
        continue
    }
    
    // 处理正常消息
    handleMessage(msg)
}
```

### 6. 打断机制

```go
// 发送打断消息
p.Input() <- pipeline.Message{
    Type: pipeline.MessageTypeInterrupt,
}

// 或者直接取消 Context
cancel()  // 所有 Stage 立即停止
```

### 7. 可观测性

```go
// 自定义观察者
type MyObserver struct{}

func (o *MyObserver) OnMessage(stageName string, msg pipeline.Message) {
    fmt.Printf("[%s] Message: %s\n", stageName, msg.Type)
}

func (o *MyObserver) OnError(stageName string, err error) {
    fmt.Printf("[%s] Error: %v\n", stageName, err)
}

func (o *MyObserver) OnStageStart(stageName string) {
    fmt.Printf("[%s] Started\n", stageName)
}

func (o *MyObserver) OnStageStop(stageName string) {
    fmt.Printf("[%s] Stopped\n", stageName)
}

// 使用
p := pipeline.NewBuilder().
    AddStage(myStage).
    SetObserver(&MyObserver{}).
    Build()
```

## 常见场景

### 场景 1：文本处理管道

```go
p := pipeline.NewBuilder().
    AddStage(pipeline.NewTextFilterStage()).
    AddStage(pipeline.NewEmotionExtractorStage()).
    AddStage(NewUppercaseStage()).
    Build()
```

### 场景 2：AI 对话

```go
p := pipeline.NewBuilder().
    AddStage(stages.NewAgentStage(agent)).
    AddStage(stages.NewTTSStage(audioOutPipe)).
    Build()

// 发送用户输入
p.Input() <- pipeline.NewMessage(
    pipeline.MessageTypeTextChunk,
    "你好",
)

// 接收 AI 响应
for msg := range p.Output() {
    if msg.Type == pipeline.MessageTypeTTSStop {
        break // TTS 播放完成
    }
}
```

### 场景 3：工具调用

```go
p := pipeline.NewBuilder().
    AddStage(stages.NewAgentStage(agent)).
    AddStage(stages.NewToolExecutorStage(toolExecutor, agent)).
    Build()

// Agent 会自动识别工具调用，ToolExecutorStage 会自动执行
p.Input() <- pipeline.NewMessage(
    pipeline.MessageTypeTextChunk,
    "帮我搜索今天的天气",
)
```

## 调试技巧

### 1. 使用 Logging Observer

```go
p := pipeline.NewBuilder().
    AddStage(myStage).
    SetObserver(pipeline.NewLoggingObserver(true)).  // verbose=true
    Build()
```

### 2. 打印消息流

```go
for msg := range p.Output() {
    fmt.Printf("Type: %s, Payload: %v, Error: %v\n", 
        msg.Type, msg.Payload, msg.Metadata.Error)
}
```

### 3. 使用 Context Timeout

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

p.Start(ctx)
```

## 最佳实践

### ✅ DO

1. **使用 select 防止阻塞**
   ```go
   select {
   case output <- msg:
   case <-ctx.Done():
       return
   }
   ```

2. **defer close(output)**
   ```go
   go func() {
       defer close(output)
       // ...
   }()
   ```

3. **检查 Context**
   ```go
   for {
       select {
       case <-ctx.Done():
           return
       case msg := <-input:
           // ...
       }
   }
   ```

### ❌ DON'T

1. **不要忘记处理 ctx.Done()**
   ```go
   // ❌ 错误
   for msg := range input {
       output <- msg  // 可能永久阻塞
   }
   
   // ✅ 正确
   for {
       select {
       case <-ctx.Done():
           return
       case msg, ok := <-input:
           if !ok {
               return
           }
           select {
           case output <- msg:
           case <-ctx.Done():
               return
           }
       }
   }
   ```

2. **不要在 Stage 中存储可变状态**
   ```go
   // ❌ 错误
   type MyStage struct {
       counter int  // 可变状态
   }
   
   // ✅ 正确：使用 Metadata 传递状态
   msg.Metadata.Extra["counter"] = counter
   ```

3. **不要 panic**
   ```go
   // ❌ 错误
   panic("error")
   
   // ✅ 正确：返回错误消息
   output <- msg.WithError(fmt.Errorf("error"))
   ```

## 下一步

- 📖 阅读 [Pipeline 设计文档](../../docs/pipeline-design.md)
- 📖 阅读 [Stage 实现文档](stages/README.md)
- 🧪 查看 [测试示例](pipeline_test.go)
- 🏗️ 查看 [完整示例](stages/integration_test.go)
