# Pipeline Stages

业务相关的 Pipeline Stage 实现。

## 已实现的 Stage

### 1. AgentStage

**职责**：调用 LLM Agent 处理用户输入，**内部处理工具调用和总结**。

**输入**：
- `MessageTypeTextChunk`（用户输入文本）

**输出**：
- `MessageTypeTextChunk`（Agent 响应文本，包括工具调用的总结）
- `MessageTypeAudioData`（工具返回的音频资源，如果有）
- `MessageTypeFinished`（处理完成）
- `MessageTypeError`（错误）

**使用**：
```go
stage := stages.NewAgentStage(agent)
```

**特性**：
- Agent 内部自动执行工具调用
- Agent 内部自动总结工具结果
- 如果工具返回音频资源，输出 `MessageTypeAudioData`
- 对外只暴露最终结果，不暴露工具调用细节

---

### 2. TTSStage

**职责**：将文本转换为语音，管理 TTS session 生命周期。

**输入**：
- `MessageTypeTextChunk`（待合成文本）
- `MessageTypeFinished`（结束 TTS session）
- `MessageTypeInterrupt`（打断 TTS）

**输出**：
- `MessageTypeTTSStop`（TTS 停止通知）
- 透传所有输入消息

**使用**：
```go
stage := stages.NewTTSStage(audioOutPipe)
```

**特性**：
- 自动管理 TTS session（BeginTTSStream/EndTTSStream）
- 根据 `Metadata.Emotion` 选择音色
- 支持打断

---

### 3. ASRStage

**职责**：音频输入和语音识别（Source Stage）。

**输入**：
- 无（Source Stage）

**输出**：
- `MessageTypeTextPartial`（ASR interim result）
- `MessageTypeTextChunk`（ASR final result）
- `MessageTypeInterrupt`（检测到用户说话）
- `MessageTypeError`（启动错误）

**使用**：
```go
stage := stages.NewASRStage(audioInPipe)
```

**特性**：
- Source Stage（不需要输入）
- 有缓冲输出 channel（避免阻塞 ASR 回调）
- 支持 interim result 和用户打断检测

---

## 完整 Pipeline 示例

```go
import (
    "github.com/liuscraft/orion-x/internal/pipeline"
    "github.com/liuscraft/orion-x/internal/pipeline/stages"
)

func NewVoicebotPipeline(
    agent stages.AgentRunner,
    audioInPipe audio.AudioInPipe,
    audioOutPipe audio.AudioOutPipe,
) pipeline.Pipeline {
    return pipeline.NewBuilder().
        // 1. 音频输入 + ASR
        AddStage(stages.NewASRStage(audioInPipe)).
        
        // 2. Agent 处理（LLM + 工具调用 + 总结）
        AddStage(stages.NewAgentStage(agent)).
        
        // 3. 文本处理（可选）
        AddStage(pipeline.NewTextFilterStage()).
        AddStage(pipeline.NewEmotionExtractorStage()).
        
        // 4. TTS 合成
        AddStage(stages.NewTTSStage(audioOutPipe)).
        
        // 5. 可观测性
        SetObserver(pipeline.NewLoggingObserver(true)).
        Build()
}
```

## 数据流示例

```
用户说话 → ASR
  ↓
MessageTypeTextChunk("你好")
  ↓
AgentStage (内部：LLM → 识别工具 → 执行工具 → 总结)
  ↓
MessageTypeTextChunk("你好！") + MessageTypeAudioData(...) + MessageTypeTextChunk("搜索结果...")
  ↓
TextFilterStage
  ↓
TTSStage
  ↓
播放语音
  ↓
MessageTypeFinished
```

## 测试

```bash
go test ./internal/pipeline/stages/
```

## 扩展

创建自定义 Stage：

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
                // 处理逻辑
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

## 注意事项

1. **Source Stage**（如 ASRStage）忽略 input 参数
2. **Sink Stage**（如果有）返回 nil 或 empty channel
3. **所有 Stage** 必须响应 `ctx.Done()`，及时退出
4. **错误处理**：使用 `msg.WithError()` 而不是 panic
5. **Channel 操作**：使用 `select` 避免阻塞
