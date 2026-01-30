# AudioInPipe 设计文档

## 模块职责

AudioInPipe 是音频输入管道，负责音频数据接收、VAD监听、ASR调用。

**重要说明**：AudioInPipe 本身不负责音频采集，而是通过 `AudioSource` 接口抽象音频输入源。具体的音频采集实现（如本地麦克风、WebSocket、文件等）位于 `internal/audio/source/` 包中。

## 核心功能

1. **音频数据接收**：通过 `AudioSource` 接口或 `SendAudio()` 方法接收音频数据
2. **VAD检测**：检测用户说话活动
3. **ASR调用**：调用 ASR 服务进行语音识别
4. **事件发布**：发布识别结果和用户说话检测事件
5. **中断处理**：响应中断信号，停止接收和识别

## 状态机设计

```
Idle (初始状态)
  ↓
Listening (监听中) - 正在采集音频和识别
  ↓
Stopping (停止中) - 清理资源中
  ↓
Idle (停止完成)
```

### 状态转换

| 当前状态 | 事件 | 目标状态 | 说明 |
|---------|------|---------|------|
| Idle | Start | Listening | 开始采集 |
| Listening | Stop | Stopping | 停止请求 |
| Stopping | Cleanup | Idle | 清理完成 |

## 接口设计

### AudioInPipe 接口

```go
type AudioInPipe interface {
    Start(ctx context.Context) error
    Stop() error
    SendAudio(audio []byte) error
    OnASRResult(handler func(text string, isFinal bool))
    OnUserSpeakingDetected(handler func())
}
```

### InPipeConfig 配置

```go
type InPipeConfig struct {
    SampleRate   int     // 采样率（默认 16000）
    Channels     int     // 声道数（默认 1）
    EnableVAD    bool    // 是否启用VAD（默认 true）
    VADThreshold float64 // VAD阈值（默认 0.5）
    ASREnabled   bool    // 是否启用ASR（默认 true）
}
```

## 数据流

```
麦克风采集
    ↓
VAD检测（可选）
    ↓
ASR识别
    ↓
结果回调
    ├─ partial（中间结果）
    └─ final（最终结果）
        ↓
    发布事件到 EventBus
    ├─ UserSpeakingDetectedEvent
    └─ ASRFinalEvent
```

## 事件发布

| 事件 | 触发条件 | 说明 |
|------|---------|------|
| UserSpeakingDetectedEvent | ASR 返回非空结果 | 检测到用户说话，触发 AudioOutPipe 中断 |
| ASRFinalEvent | ASR 返回 final 结果 | 识别完成，传递文本给 Orchestrator |

## 依赖模块

- `internal/asr/recognizer.go` - ASR 服务
- `AudioSource` 接口 - 音频源抽象

## 部署场景

### 客户端模式（本地运行）

```
本地麦克风 → AudioSource → AudioInPipe → ASR → 后续处理
```

使用 `internal/audio/source/MicrophoneSource`（基于 PortAudio）采集本地麦克风音频。

### 服务端模式（WebSocket）

```
浏览器麦克风 → WebSocket → AudioInPipe.SendAudio() → ASR → 后续处理
```

服务端不使用 AudioSource，而是通过 `SendAudio()` 方法接收来自 WebSocket 的音频流。

## 音频源集成

AudioInPipe 通过 `AudioSource` 接口抽象音频输入源，具体的音频源实现（如本地麦克风、WebSocket、文件等）位于 `internal/audio/source/` 包中。

**AudioSource 接口**：
```go
type AudioSource interface {
    Read(ctx context.Context) ([]byte, error)
    Close() error
}
```

**可用的音频源**：
- `MicrophoneSource` - 本地麦克风（已实现）
- `WebSocketSource` - WebSocket 音频流（待实现）
- `FileSource` - 文件音频（待实现）

## VAD 检测（可选）

### 触发逻辑

- 在 `AudioInPipe.readAudioFromSource()` 中对 PCM 数据计算能量（RMS）。
- 当 RMS >= `VADThreshold` 且距离上次触发超过最小间隔时，触发 `OnUserSpeakingDetected()`。
- 通过 `EnableVAD` 开关控制，默认启用。

### 参数与策略

- `VADThreshold`：0~1，基于 16-bit PCM 归一化 RMS。
- 最小触发间隔：300ms（防止频繁触发）。
- 若音频读取返回 `context.Canceled`/`io.EOF`，直接退出，不触发 VAD。

## 使用方式

### 客户端模式：使用 MicrophoneSource

```go
import (
    "github.com/liuscraft/orion-x/internal/audio"
    "github.com/liuscraft/orion-x/internal/audio/source"
)

// 1. 创建麦克风音频源
micSource, err := source.NewMicrophoneSource(16000, 1, 3200)
if err != nil {
    return err
}
defer micSource.Close()

// 2. 创建 AudioInPipe 并关联音频源
config := audio.DefaultInPipeConfig()
audioInPipe, err := audio.NewInPipeWithAudioSource(apiKey, config, micSource)
if err != nil {
    return err
}

// 3. 启动音频处理
audioInPipe.Start(ctx)
```

### 服务端模式：使用 SendAudio()

```go
import "github.com/liuscraft/orion-x/internal/audio"

// 1. 创建 AudioInPipe（不关联 AudioSource）
config := audio.DefaultInPipeConfig()
audioInPipe, err := audio.NewInPipe(apiKey, config)
if err != nil {
    return err
}

// 2. 从 WebSocket 接收音频并发送到 AudioInPipe
go func() {
    for {
        audioData, err := websocket.ReadMessage()
        if err != nil {
            break
        }
        audioInPipe.SendAudio(audioData)
    }
}()
```

### 测试模式：使用自定义 AudioSource

```go
// 实现自定义音频源（如文件、Mock）
type CustomSource struct { ... }

func (c *CustomSource) Read(ctx context.Context) ([]byte, error) { ... }
func (c *CustomSource) Close() error { ... }

// 使用自定义源
customSource := &CustomSource{}
audioInPipe, err := audio.NewInPipeWithAudioSource(apiKey, config, customSource)
```

### MicrophoneSource 参数

使用 PortAudio 从麦克风采集 PCM 音频：
- 格式：16-bit PCM, little-endian
- 采样率：16000 Hz
- 声道数：1（单声道）
- 帧长：3200 samples（约 200ms）
- 读取支持 `context` 取消：当 `ctx.Done()` 或 `Close()` 触发时，主动 `Abort()` 打断 `Read()` 的阻塞
- 关闭流程：先 `Abort()` 强制中断阻塞读，再 `Stop()`/`Close()`，避免退出时卡住

## ASR 集成

```go
import "github.com/liuscraft/orion-x/internal/asr"

cfg := asr.Config{
    APIKey:      os.Getenv("DASHSCOPE_API_KEY"),
    Model:       "fun-asr-realtime",
    Format:      "pcm",
    SampleRate:  16000,
}

recognizer, _ := asr.NewDashScopeRecognizer(cfg)
```

### VAD 方案选择

**方案 1**：使用 ASR 的 VAD 能力（推荐）
- DashScope ASR 内置 VAD
- 通过 `IsFinal=true` 判断句子结束

**方案 2**：自实现 VAD
- 计算音频能量
- 超过阈值判定为说话
- 连续静音超过阈值判定为静音

### 发送取消

- `SendAudio` 使用 `context` 进行取消控制
- 在 `Stop()` 触发取消时，ASR 写入会被打断，避免阻塞退出

### 事件发布

```go
recognizer.OnResult(func(result asr.Result) {
    if result.Text != "" {
        eventBus.Publish(NewUserSpeakingDetectedEvent())
    }

    if result.IsFinal {
        eventBus.Publish(NewASRFinalEvent(result.Text))
    }
})
```

## 中断处理

收到 Stop 请求时：
1. 停止音频采集
2. 关闭 ASR 连接
3. 清理资源
4. 状态转换回 Idle

## 并发安全

- 使用 `sync.Mutex` 保护状态转换
- 使用 context.Context 处理取消
- 使用 `sync.WaitGroup` 等待 goroutine 退出

## 错误处理

| 错误类型 | 处理方式 |
|---------|---------|
| 麦克风打开失败 | 返回 error，保持 Idle 状态 |
| ASR 连接失败 | 返回 error，保持 Idle 状态 |
| 音频发送失败 | 记录日志，继续运行 |
| Context 取消 | 清理资源，返回 Idle |

## 测试要点

1. 状态转换测试
2. ASR 结果回调测试
3. 事件发布测试
4. 中断处理测试
5. 并发安全测试

## 配置示例

```go
config := audio.DefaultInPipeConfig()
config.SampleRate = 16000
config.Channels = 1
config.EnableVAD = true
config.VADThreshold = 0.5
```

## 相关文档

- [ASR 语音识别](/guide/asr) - ASR 服务详细说明
- [音频输出管道](/guide/audio-out-pipe) - AudioOutPipe 设计文档
