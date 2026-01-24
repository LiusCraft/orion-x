# 问题修复总结

## 问题 1: Ctrl+C 退出卡住

### 现象
按 Ctrl+C 后程序卡住无法退出，只能强制终止。

### 根因
1. `AudioInPipe` 的 `Stop()` 方法等待音频读取协程退出
2. 音频读取协程卡在 `stream.Read()` 上（PortAudio 阻塞等待音频数据）
3. 即使 context 被取消，`stream.Read()` 也不会立即返回
4. 主 goroutine 和音频读取协程形成死锁

### 修复方案

**修改文件：** `internal/audio/inpipe_impl.go`

```go
func (p *inPipeImpl) Stop() error {
    // 1. 先设置停止状态
    p.state = InPipeStateStopping

    // 2. 取消 context（让 SendAudio 返回错误）
    if p.cancel != nil {
        p.cancel()
    }

    // 3. 关闭音频源（让 Read() 返回错误，解除阻塞）
    if p.audioSource != nil {
        p.audioSource.Close()
    }

    // 4. 关闭 ASR
    if p.recognizer != nil {
        p.recognizer.Finish(p.ctx)
        p.recognizer.Close()
    }

    // 5. 等待 goroutine 退出
    p.wg.Wait()

    return nil
}
```

**修改文件：** `cmd/voicebot/main.go`

添加信号处理器，收到中断后立即停止并退出：

```go
go func() {
    <-sigCh
    orchestrator.Stop()
    cancel()
    os.Exit(0)
}()
```

## 问题 2: TTS 报错 418

### 现象
```
AudioOutPipe: TTS close error: tts bad request: [tts:]Engine return error code: 418
```

### 根因
HTTP 418 "I'm a teapot" 错误码表示：
- API 请求参数不正确
- API key 无效或无权限
- TTS 模型不可用
- 速率限制

### 修复方案

**修改文件：** `internal/tts/dashscope.go`

添加详细日志，帮助调试：

```go
func mapDashScopeError(code, message string) error {
    log.Printf("TTS error: code=%s, message=%s", code, message)
    // ... 其余代码
}
```

**可能的原因：**
1. DASHSCOPE_API_KEY 不正确
2. TTS 模型 `cosyvoice-v3-flash` 可能需要特殊权限
3. 参数配置问题

**建议：**
- 检查 API key 是否正确
- 尝试使用其他 TTS 模型
- 查看阿里云控制台的 API 配额和限制

## 问题 3: 情绪标签没有被过滤

### 现象
TTS 收到了包含情绪标签的文本：
```
Orchestrator: playing TTS for sentence: [EMO:calm] 你好！
```

### 根因
情绪标签在流式输出中被分成多个 chunk：
- `[`
- `E`
- `M`
- `O`
- `:`
- `c`...
- `]`

每个 chunk 单独过滤时都不包含完整的 `[EMO:calm]` 模式，所以过滤器无法正确移除。

### 修复方案

**修改文件：** `internal/agent/processor.go`

添加 `RemoveEmotionTags` 方法：

```go
func (f *markdownFilter) RemoveEmotionTags(text string) string {
    return removeEmotionTags(text)
}
```

**修改文件：** `internal/agent/voice_agent_impl.go`

改进流式处理逻辑：

```go
currentEmotion := ""
fullText := ""
bufferedContent := ""
lastFilteredLength := 0

for {
    // ... 接收消息

    if msg.Content != "" {
        bufferedContent += msg.Content

        // 提取情绪
        emotion := v.emotionExtractor.Extract(bufferedContent)
        if emotion != "" && emotion != currentEmotion {
            currentEmotion = emotion
            eventChan <- &EmotionChangedEvent{Emotion: emotion}
        }

        // 移除缓冲内容中的情绪标签
        cleanBufferedContent := v.markdownFilter.RemoveEmotionTags(bufferedContent)

        // 只发送新增的内容
        if len(cleanBufferedContent) > lastFilteredLength {
            newContent := cleanBufferedContent[lastFilteredLength:]
            if newContent != "" {
                eventChan <- &TextChunkEvent{Chunk: newContent, Emotion: currentEmotion}
                fullText += newContent
                lastFilteredLength = len(cleanBufferedContent)
            }
        }
    }
}
```

## 问题 4: LLM 没有调用工具

### 现象
用户问"现在几点了？"，LLM 自己回答而不是调用 `getTime` 工具。

### 根因
LLM 不知道有 `getTime` 和 `getWeather` 工具可用，因为系统提示词中没有描述工具。

### 修复方案

**修改文件：** `internal/agent/voice_agent_impl.go`

添加工具描述到系统提示词：

```go
messages := []*schema.Message{
    schema.SystemMessage(`你是一个语音助手。

规则：
1. 在每个句子的开头包含情绪标签，格式为 [EMO:emotion]，可选值：happy, sad, angry, calm, excited。
   例如：[EMO:happy] 你好啊！[EMO:calm] 今天有什么可以帮你？

2. 当用户询问时间时，请使用 getTime 工具获取准确时间。

3. 当用户询问天气时，请使用 getWeather 工具。

工具定义：
- getTime: 获取当前时间，返回日期、时间、星期、时区等信息
- getWeather: 获取指定城市的天气信息，需要参数 city（城市名称）

回答示例：
[EMO:calm] 现在是2026年1月24日 星期六，23点35分。
[EMO:happy] 北京今天天气不错，晴天，温度25度。`),
    schema.UserMessage(input),
}
```

## 相关文件

### 修改的文件
- `internal/audio/inpipe_impl.go` - 修复停止流程
- `internal/audio/microphone.go` - 修复 PortAudio 终止
- `cmd/voicebot/main.go` - 改进信号处理
- `internal/tts/dashscope.go` - 添加错误日志
- `internal/agent/processor.go` - 添加 RemoveEmotionTags 方法
- `internal/agent/voice_agent_impl.go` - 改进流式处理和系统提示词

### 新增的文件
- `docs/ctrl-c-fix.md` - Ctrl+C 修复说明文档

## 测试

所有修改已通过单元测试：
```bash
go test ./internal/audio/ -v
go test ./internal/agent/ -v
go test ./internal/tts/ -v
```

## 待解决的问题

1. **TTS 错误 418** - 需要检查 API key 和模型配置
2. **LLM 工具调用** - 需要验证 LLM 是否能正确调用工具（需要真实 API key 测试）
3. **音频输入** - 麦克风输入已经可以工作，但可能需要调整 VAD 阈值
