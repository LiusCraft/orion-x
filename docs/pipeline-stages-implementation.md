# Pipeline Stages 实现总结

## 完成内容

### ✅ 核心 Stage 实现

**文件结构**：
```
internal/pipeline/stages/
├── agent.go              # AgentStage - LLM 处理
├── agent_test.go         # AgentStage 测试
├── asr.go                # ASRStage - 语音识别
├── tts.go                # TTSStage - 语音合成
├── tool.go               # ToolExecutorStage - 工具执行
├── integration_test.go   # 集成测试
└── README.md             # 使用文档
```

### 1. AgentStage ✅

**职责**：调用 LLM Agent，转换 AgentEvent 到 Pipeline Message

**实现要点**：
- 只处理 `MessageTypeTextChunk` 输入
- 调用 `agent.Process()` 获取事件流
- 转换 3 种事件类型：
  - `TextChunkEvent` → `MessageTypeTextChunk`
  - `ToolCallRequestedEvent` → `MessageTypeToolCall`
  - `FinishedEvent` → `MessageTypeFinished`
- 错误通过 `Message.Metadata.Error` 传播
- 非文本消息透传

**测试覆盖**：
- ✅ 文本响应
- ✅ 工具调用
- ✅ 错误处理
- ✅ 消息透传

---

### 2. TTSStage ✅

**职责**：管理 TTS session 生命周期，将文本转语音

**实现要点**：
- 第一个文本 chunk 调用 `BeginTTSStream(emotion)`
- 后续 chunk 调用 `WriteTTSChunk(text)`
- `MessageTypeFinished` 触发 `EndTTSStream()`
- `MessageTypeInterrupt` 触发 `Interrupt()`
- 根据 `Metadata.Emotion` 选择音色
- 发送 `MessageTypeTTSStop` 通知

**状态管理**：
- `ttsActive` 标记 session 是否活跃
- defer 确保资源清理

---

### 3. ToolExecutorStage ✅

**职责**：执行工具调用，总结结果

**实现要点**：
- 只处理 `MessageTypeToolCall`
- 调用 `executor.Execute(tool, args)`
- 如果有音频资源，输出 `MessageTypeAudioData`
- 调用 `agent.SummarizeToolResult()` 总结文本结果
- 异步执行（独立 goroutine）

**复用**：
- 复用 `AgentStage.convertAgentEvent` 转换总结事件

---

### 4. ASRStage ✅

**职责**：音频输入和语音识别（Source Stage）

**实现要点**：
- Source Stage（忽略 input 参数）
- 设置 `OnASRResult` 回调：
  - `isFinal=true` → `MessageTypeTextChunk`
  - `isFinal=false` → `MessageTypeTextPartial`
- 设置 `OnUserSpeakingDetected` 回调 → `MessageTypeInterrupt`
- 使用缓冲 channel（16）避免阻塞回调
- 启动 `audioInPipe.Start(ctx)`
- ctx 取消时自动 Stop

---

## Message 类型扩展

新增了 4 个消息类型：

```go
MessageTypeTextPartial  // ASR interim result
MessageTypeInterrupt    // 用户打断
MessageTypeTTSStart     // TTS 开始
MessageTypeTTSStop      // TTS 停止
```

## 完整 Pipeline 示例

```go
func NewVoicebotPipeline(
    agent AgentRunner,
    audioInPipe audio.AudioInPipe,
    audioOutPipe audio.AudioOutPipe,
    toolExecutor tools.ToolExecutor,
) pipeline.Pipeline {
    return pipeline.NewBuilder().
        AddStage(stages.NewASRStage(audioInPipe)).
        AddStage(stages.NewAgentStage(agent)).
        AddStage(stages.NewToolExecutorStage(toolExecutor, agent)).
        AddStage(pipeline.NewTextFilterStage()).
        AddStage(pipeline.NewEmotionExtractorStage()).
        AddStage(stages.NewTTSStage(audioOutPipe)).
        SetObserver(pipeline.NewLoggingObserver(true)).
        Build()
}
```

## 数据流

### 正常流程

```
用户语音
  ↓
ASRStage: MessageTypeTextChunk("你好")
  ↓
AgentStage: MessageTypeTextChunk("你好！我可以帮你什么？")
            MessageTypeFinished
  ↓
TextFilterStage: 过滤标签
  ↓
EmotionExtractorStage: 提取情感
  ↓
TTSStage: BeginTTSStream → WriteTTSChunk → EndTTSStream
          MessageTypeTTSStop
  ↓
播放完成
```

### 工具调用流程

```
用户: "帮我搜索天气"
  ↓
ASRStage: MessageTypeTextChunk("帮我搜索天气")
  ↓
AgentStage: MessageTypeToolCall{tool: "search", args: {...}}
            MessageTypeFinished
  ↓
ToolExecutorStage: 
  - Execute tool
  - MessageTypeAudioData (如果有)
  - MessageTypeTextChunk("搜索结果是...")
  - MessageTypeFinished
  ↓
TTSStage: 播放总结语音
```

### 打断流程

```
用户在 AI 播放时说话
  ↓
ASRStage: MessageTypeInterrupt
  ↓
TTSStage: 检测到 Interrupt → audioOutPipe.Interrupt()
  ↓
AgentStage: context 被取消 → 停止处理
  ↓
新的用户输入开始
```

## 设计亮点

### 1. 适配器模式

Stage 不重写现有组件，只做协议转换：
- `AgentEvent` ↔ `Pipeline.Message`
- 复用所有现有逻辑

### 2. 统一错误处理

```go
// Agent 错误
eventChan, err := s.agent.Process(ctx, text)
if err != nil {
    output <- msg.WithError(err)  // 错误变成 Message
}

// FinishedEvent 错误
case *agent.FinishedEvent:
    if e.Error != nil {
        msg.Metadata.Error = e.Error  // 错误嵌入 Metadata
    }
```

### 3. 异步工具执行

ToolExecutorStage 不阻塞 Pipeline：
```go
go s.executeToolAndSummarize(ctx, msg, output)
```

### 4. 资源清理

TTSStage 使用 defer 确保 session 关闭：
```go
defer func() {
    if s.ttsActive {
        s.audioOutPipe.EndTTSStream()
    }
}()
```

## 与 Orchestrator 对比

| 功能 | Orchestrator | Pipeline Stages |
|------|--------------|-----------------|
| Agent 调用 | `handleASRFinal()` | `AgentStage.Process()` |
| 工具执行 | `handleToolCallRequested()` | `ToolExecutorStage.Process()` |
| TTS 管理 | `handleAgentEvent()` 中分散 | `TTSStage.Process()` 集中管理 |
| 错误处理 | 分散在各 handler | 统一通过 Message.Error |
| 打断逻辑 | 手动取消多个组件 | Context 取消传播 |
| 测试 | 需 mock EventBus | 每个 Stage 独立测试 |

## 测试策略

### 单元测试

每个 Stage 独立测试：
```go
func TestAgentStage_TextChunk(t *testing.T) {
    mockAgent := &mockAgentRunner{...}
    stage := NewAgentStage(mockAgent)
    
    input := make(chan pipeline.Message, 1)
    output := stage.Process(ctx, input)
    
    input <- pipeline.NewMessage(...)
    msg := <-output
    // assert...
}
```

### 集成测试

组合多个 Stage 测试：
```go
func TestAgentStage_Integration(t *testing.T) {
    p := pipeline.NewBuilder().
        AddStage(stages.NewAgentStage(agent)).
        AddStage(pipeline.NewTextFilterStage()).
        Build()
    
    p.Start(ctx)
    p.Input() <- msg
    result := <-p.Output()
    // assert...
}
```

## 性能考虑

1. **ASRStage 缓冲**：使用 16 大小缓冲，避免阻塞 ASR 回调
2. **异步工具执行**：工具在独立 goroutine 中执行，不阻塞主流
3. **Channel 选择**：所有发送都用 `select` 检查 ctx.Done()
4. **资源清理**：defer 确保组件正确停止

## 下一步

### 阶段 3：集成到 Voicebot

1. **创建 VoicebotPipeline 工厂**
   - 组装所有 Stage

2. **集成使用**
   - 替换现有 Orchestrator
   - 保持现有 WebSocket 协议

3. **对比测试**
   - 功能完整性
   - 性能指标（延迟、内存）
   - 稳定性

4. **渐进式迁移**
   - Feature flag 控制
   - 部分用户灰度
   - 监控指标对比

### 预计时间

- Pipeline 工厂实现：2 小时
- 集成：3 小时
- 测试和调试：4 小时
- 文档和总结：1 小时

**总计**：1-1.5 天完成完整集成

## 总结

✅ **Stage 实现已完成**：
- 4 个核心 Stage（Agent, TTS, Tool, ASR）
- 扩展 Message 类型支持
- 单元测试和集成测试
- 完整文档

🎯 **架构优势**：
- 适配器模式，不修改现有代码
- 统一错误处理和传播
- 易于测试和扩展
- 清晰的职责分离

📊 **代码量**：
- 核心实现：~500 行
- 测试代码：~300 行
- 文档：~400 行

**Pipeline 框架现在已经可以投入使用！**
