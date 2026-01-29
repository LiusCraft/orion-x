# AudioSource 设计文档

## 模块概述

`AudioSource` 是音频输入源的抽象接口，定义了统一的音频数据读取规范。具体的音频源实现（如本地麦克风、WebSocket、文件等）位于独立的 `internal/audio/source/` 包中。

## 设计目标

1. **解耦音频输入与处理**：AudioInPipe 通过 AudioSource 接口接收音频，不依赖具体的采集实现
2. **支持多种部署模式**：客户端（本地麦克风）、服务端（WebSocket）、测试（文件）
3. **依赖隔离**：服务端部署不需要依赖 PortAudio 等客户端库
4. **易于扩展**：新增音频源只需实现 AudioSource 接口

## 架构设计

### 模块层次

```
┌─────────────────────────────────────────┐
│         AudioInPipe (核心处理)           │
│   - VAD 检测                             │
│   - ASR 调用                             │
│   - 事件发布                             │
└─────────────────┬───────────────────────┘
                  │ 依赖
                  ↓
┌─────────────────────────────────────────┐
│      AudioSource (接口抽象)              │
│   Read(ctx) ([]byte, error)             │
│   Close() error                         │
└─────────────────┬───────────────────────┘
                  │ 实现
        ┌─────────┴─────────┬──────────────┐
        ↓                   ↓              ↓
┌───────────────┐  ┌─────────────────┐  ┌──────────┐
│ Microphone    │  │ WebSocket       │  │ File     │
│ Source        │  │ Source          │  │ Source   │
│ (PortAudio)   │  │ (服务端)         │  │ (测试)    │
└───────────────┘  └─────────────────┘  └──────────┘
```

### 目录结构

```
internal/audio/
  ├── source.go              # AudioSource 接口定义
  ├── inpipe.go              # AudioInPipe 接口
  ├── inpipe_impl.go         # AudioInPipe 实现
  ├── outpipe.go
  ├── mixer.go
  └── ...

internal/audio/source/       # 音频源实现（独立包）
  ├── microphone.go          # 本地麦克风源
  ├── microphone_test.go
  ├── websocket.go           # WebSocket 源（待实现）
  ├── file.go                # 文件源（待实现）
  └── README.md              # 音频源使用说明
```

## 接口定义

### AudioSource 接口

**位置**：`internal/audio/source.go`

```go
package audio

import "context"

// AudioSource 音频输入源接口
type AudioSource interface {
    // Read 读取音频数据
    // 返回 PCM 音频数据（16-bit, little-endian）
    // 支持 context 取消
    Read(ctx context.Context) ([]byte, error)
    
    // Close 关闭音频源，释放资源
    Close() error
}
```

### 接口约定

1. **音频格式**：
   - 返回 16-bit PCM 音频数据
   - Little-endian 字节序
   - 采样率和声道数由具体实现定义

2. **上下文支持**：
   - `Read()` 必须支持 `context.Context` 取消
   - 当 `ctx.Done()` 时，应立即返回 `ctx.Err()`

3. **错误处理**：
   - 返回 `io.EOF` 表示音频流结束
   - 返回 `context.Canceled` 表示被取消
   - 其他错误表示读取失败

4. **并发安全**：
   - `Read()` 和 `Close()` 可能从不同 goroutine 调用
   - 实现必须保证并发安全

## 音频源实现

### 1. MicrophoneSource（本地麦克风）

**位置**：`internal/audio/source/microphone.go`

**用途**：客户端本地运行，从系统麦克风采集音频

**依赖**：`github.com/gordonklaus/portaudio`

**构造函数**：
```go
func NewMicrophoneSource(sampleRate, channels, bufferSize int) (*MicrophoneSource, error)
```

**参数**：
- `sampleRate`：采样率（如 16000）
- `channels`：声道数（1=单声道，2=立体声）
- `bufferSize`：缓冲区大小（samples 数量）

**特性**：
- 使用 PortAudio 采集系统默认麦克风
- 支持 context 取消，中断阻塞的 Read()
- 关闭流程：`Stop()` → `Close()`，优雅退出
- 线程安全：使用 `sync.Once` 保证只关闭一次

**示例**：
```go
import "github.com/liuscraft/orion-x/internal/audio/source"

// 创建麦克风源：16kHz, 单声道, 3200 samples
micSource, err := source.NewMicrophoneSource(16000, 1, 3200)
if err != nil {
    return err
}
defer micSource.Close()

// 读取音频
ctx := context.Background()
audioData, err := micSource.Read(ctx)
```

**注意事项**：
- PortAudio 需要系统音频驱动支持
- macOS/Linux 需要安装 PortAudio 库
- 服务端部署不应使用此实现

---

### 2. WebSocketSource（WebSocket 音频流）

**位置**：`internal/audio/source/websocket.go`（待实现）

**用途**：服务端从 WebSocket 接收浏览器传来的音频流

**依赖**：`github.com/gorilla/websocket` 或类似库

**构造函数**：
```go
func NewWebSocketSource(conn *websocket.Conn, config *WebSocketSourceConfig) (*WebSocketSource, error)
```

**特性**：
- 从 WebSocket 连接读取 PCM 音频数据
- 支持音频格式协商（采样率、声道数）
- 处理网络断连和重连
- 支持背压控制（buffering）

**数据格式**：
```json
{
  "type": "audio",
  "format": "pcm",
  "sampleRate": 16000,
  "channels": 1,
  "data": "base64-encoded-pcm-data"
}
```

**示例**（伪代码）：
```go
import "github.com/liuscraft/orion-x/internal/audio/source"

// 从 WebSocket 创建音频源
wsSource, err := source.NewWebSocketSource(conn, nil)
if err != nil {
    return err
}
defer wsSource.Close()

// 读取音频
audioData, err := wsSource.Read(ctx)
```

---

### 3. FileSource（文件音频）

**位置**：`internal/audio/source/file.go`（待实现）

**用途**：测试和调试，从文件读取预录制的音频

**构造函数**：
```go
func NewFileSource(filePath string, sampleRate, channels int) (*FileSource, error)
```

**特性**：
- 读取 WAV 或 RAW PCM 文件
- 支持循环播放（用于长时间测试）
- 可模拟实时音频流（按帧率延迟）

**示例**：
```go
import "github.com/liuscraft/orion-x/internal/audio/source"

fileSource, err := source.NewFileSource("test_audio.pcm", 16000, 1)
if err != nil {
    return err
}
defer fileSource.Close()

// 读取音频
audioData, err := fileSource.Read(ctx)
```

---

### 4. StreamSource（通用流包装器）

**位置**：`internal/audio/source/stream.go`（待实现）

**用途**：将任意 `io.Reader` 包装为 AudioSource

**构造函数**：
```go
func NewStreamSource(reader io.Reader, frameSize int) *StreamSource
```

**示例**：
```go
file, _ := os.Open("audio.pcm")
streamSource := source.NewStreamSource(file, 3200)
defer streamSource.Close()
```

## 使用场景

### 场景 1：客户端本地运行

```go
import (
    "github.com/liuscraft/orion-x/internal/audio"
    "github.com/liuscraft/orion-x/internal/audio/source"
)

func main() {
    // 创建本地麦克风源
    micSource, err := source.NewMicrophoneSource(16000, 1, 3200)
    if err != nil {
        log.Fatal(err)
    }
    defer micSource.Close()
    
    // 创建 AudioInPipe
    config := audio.DefaultInPipeConfig()
    audioInPipe, err := audio.NewInPipeWithAudioSource(apiKey, config, micSource)
    if err != nil {
        log.Fatal(err)
    }
    
    // 启动处理
    audioInPipe.Start(ctx)
}
```

### 场景 2：服务端 WebSocket

```go
import "github.com/liuscraft/orion-x/internal/audio"

func handleWebSocket(conn *websocket.Conn) {
    // 创建 AudioInPipe（不使用 AudioSource）
    config := audio.DefaultInPipeConfig()
    audioInPipe, err := audio.NewInPipe(apiKey, config)
    if err != nil {
        return
    }
    
    // 从 WebSocket 接收音频并发送
    go func() {
        for {
            var msg AudioMessage
            if err := conn.ReadJSON(&msg); err != nil {
                break
            }
            
            // 解码 Base64 音频数据
            audioData, _ := base64.StdEncoding.DecodeString(msg.Data)
            
            // 发送到 AudioInPipe
            audioInPipe.SendAudio(audioData)
        }
    }()
}
```

### 场景 3：单元测试

```go
import "github.com/liuscraft/orion-x/internal/audio/source"

func TestAudioProcessing(t *testing.T) {
    // 使用文件源进行测试
    fileSource, _ := source.NewFileSource("testdata/sample.pcm", 16000, 1)
    defer fileSource.Close()
    
    audioInPipe, _ := audio.NewInPipeWithAudioSource(apiKey, config, fileSource)
    
    // 测试处理逻辑
    // ...
}
```

## 实现规范

### 必须实现的功能

1. **Read() 方法**：
   - 支持 context 取消
   - 返回固定大小的音频帧（或小于帧大小的最后一块）
   - 阻塞式读取，直到数据可用或发生错误

2. **Close() 方法**：
   - 幂等性：多次调用不报错
   - 释放所有资源（文件句柄、网络连接、音频设备等）
   - 中断阻塞的 Read() 调用

3. **错误处理**：
   - 明确区分 EOF、Canceled 和其他错误
   - 错误信息包含足够的上下文

### 推荐实现的功能

1. **并发安全**：使用 `sync.Mutex` 保护共享状态
2. **单次关闭**：使用 `sync.Once` 保证 Close() 只执行一次
3. **资源清理**：使用 `defer` 确保资源释放
4. **日志记录**：使用 `internal/logging` 记录关键事件

### 测试要求

每个音频源实现必须包含以下测试：

1. **基本读取测试**：验证能正常读取音频数据
2. **Context 取消测试**：验证 Read() 响应 context 取消
3. **Close 测试**：验证 Close() 能中断 Read() 并释放资源
4. **并发测试**：验证并发 Read() 和 Close() 的安全性
5. **错误处理测试**：验证各种错误场景

### 示例测试

```go
func TestMicrophoneSourceRead(t *testing.T) {
    source, err := NewMicrophoneSource(16000, 1, 3200)
    require.NoError(t, err)
    defer source.Close()
    
    ctx := context.Background()
    data, err := source.Read(ctx)
    require.NoError(t, err)
    assert.NotEmpty(t, data)
}

func TestMicrophoneSourceCancel(t *testing.T) {
    source, err := NewMicrophoneSource(16000, 1, 3200)
    require.NoError(t, err)
    defer source.Close()
    
    ctx, cancel := context.WithCancel(context.Background())
    cancel() // 立即取消
    
    _, err = source.Read(ctx)
    assert.ErrorIs(t, err, context.Canceled)
}
```

## 性能考虑

### 缓冲区大小

- **过小**：频繁的系统调用，增加 CPU 开销
- **过大**：增加延迟，影响实时性
- **推荐**：3200 samples（16kHz 下约 200ms）

### 零拷贝

- 尽量避免不必要的内存拷贝
- 复用缓冲区（如 PortAudio 的 buffer）

### 背压控制

- WebSocketSource 应实现背压机制
- 当处理速度慢于接收速度时，避免无限缓冲

## 迁移指南

### 从旧代码迁移

**之前**：
```go
import "github.com/liuscraft/orion-x/internal/audio"

micSource, _ := audio.NewMicrophoneSource(16000, 1, 3200)
```

**现在**：
```go
import "github.com/liuscraft/orion-x/internal/audio/source"

micSource, _ := source.NewMicrophoneSource(16000, 1, 3200)
```

**原因**：将音频源实现移至独立的 `source` 包，实现模块解耦。

### 代码搜索与替换

```bash
# 查找所有使用旧 import 的文件
grep -r "audio.NewMicrophoneSource" .

# 替换 import 路径
sed -i 's|internal/audio"|internal/audio/source"|g' *.go
```

## 未来扩展

### 计划实现的音频源

1. ✅ **MicrophoneSource** - 本地麦克风（已实现）
2. ⏳ **WebSocketSource** - WebSocket 音频流（待实现）
3. ⏳ **FileSource** - 文件音频读取（待实现）
4. ⏳ **RTSPSource** - RTSP 网络音频流
5. ⏳ **StreamSource** - 通用 io.Reader 包装器

### 高级特性

1. **音频格式自动转换**：
   - 采样率转换（resampling）
   - 声道转换（mono ↔ stereo）
   - 格式解码（Opus, AAC → PCM）

2. **音频增强**：
   - 噪声抑制
   - 自动增益控制（AGC）
   - 回声消除（AEC）

3. **多源混音**：
   - 支持多个 AudioSource 混音
   - 动态添加/移除音频源

## 常见问题

### Q: 为什么要把音频源分离到独立的包？

A: 主要原因：
1. **依赖隔离**：服务端部署不需要 PortAudio
2. **职责分明**：`audio` 包负责核心抽象，`source` 包负责具体实现
3. **易于扩展**：新增音频源不影响核心模块

### Q: 服务端应该用哪种方式接收音频？

A: 推荐使用 `AudioInPipe.SendAudio()` 方法，直接发送来自 WebSocket 的音频数据，无需实现 AudioSource。

### Q: 如何测试音频处理逻辑？

A: 使用 `FileSource` 从测试文件读取音频，或实现 Mock AudioSource。

### Q: Read() 应该返回多大的数据块？

A: 建议返回固定大小的帧（如 3200 samples），便于下游处理。最后一帧可以小于帧大小。

## 参考资料

- [PortAudio 官方文档](http://www.portaudio.com/)
- [WebSocket 音频流最佳实践](https://developer.mozilla.org/en-US/docs/Web/API/MediaStream_Recording_API)
- [Go Context 使用指南](https://go.dev/blog/context)