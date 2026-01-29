# Audio Source Package

音频源实现包，提供各种音频输入源的具体实现。

## 概述

本包包含 `AudioSource` 接口的各种实现，用于从不同来源采集音频数据。核心接口定义在 `internal/audio/source.go` 中。

## 可用的音频源

### 1. MicrophoneSource

从本地系统麦克风采集音频数据（基于 PortAudio）。

**用途**: 客户端本地运行

**依赖**: `github.com/gordonklaus/portaudio`

**示例**:
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

**参数说明**:
- `sampleRate`: 采样率（如 16000 Hz）
- `channels`: 声道数（1=单声道，2=立体声）
- `bufferSize`: 缓冲区大小（samples 数量，推荐 3200）

**注意事项**:
- 需要系统安装 PortAudio 库
- macOS: `brew install portaudio`
- Linux: `apt-get install portaudio19-dev`
- 服务端部署不应使用此实现

### 2. WebSocketSource (待实现)

从 WebSocket 连接接收浏览器传来的音频流。

**用途**: 服务端部署

### 3. FileSource (待实现)

从文件读取预录制的音频数据。

**用途**: 测试和调试

## 接口规范

所有音频源必须实现 `audio.AudioSource` 接口：

```go
type AudioSource interface {
    // Read 读取音频数据
    // 返回 16-bit PCM, little-endian 格式
    // 支持 context 取消
    Read(ctx context.Context) ([]byte, error)
    
    // Close 关闭音频源，释放资源
    Close() error
}
```

## 使用场景

### 客户端模式（本地麦克风）

```go
import (
    "github.com/liuscraft/orion-x/internal/audio"
    "github.com/liuscraft/orion-x/internal/audio/source"
)

// 创建麦克风源
micSource, err := source.NewMicrophoneSource(16000, 1, 3200)
if err != nil {
    log.Fatal(err)
}
defer micSource.Close()

// 创建 AudioInPipe 并关联音频源
config := audio.DefaultInPipeConfig()
audioInPipe, err := audio.NewInPipeWithAudioSource(apiKey, config, micSource)
```

### 服务端模式（WebSocket）

服务端不使用 AudioSource，而是通过 `SendAudio()` 方法发送音频：

```go
import "github.com/liuscraft/orion-x/internal/audio"

// 创建 AudioInPipe（不关联 AudioSource）
config := audio.DefaultInPipeConfig()
audioInPipe, err := audio.NewInPipe(apiKey, config)

// 从 WebSocket 接收并发送音频
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

## 实现新的音频源

如需实现新的音频源，请遵循以下规范：

### 1. 实现接口

```go
type MyCustomSource struct {
    // 字段定义
}

func NewMyCustomSource(params...) (*MyCustomSource, error) {
    // 初始化
    return &MyCustomSource{}, nil
}

func (s *MyCustomSource) Read(ctx context.Context) ([]byte, error) {
    // 读取音频数据
    // 必须支持 context 取消
    // 返回 16-bit PCM, little-endian
}

func (s *MyCustomSource) Close() error {
    // 释放资源
    // 幂等性：多次调用不报错
}
```

### 2. 编写测试

```go
func TestMyCustomSourceRead(t *testing.T) {
    source, err := NewMyCustomSource(...)
    require.NoError(t, err)
    defer source.Close()
    
    ctx := context.Background()
    data, err := source.Read(ctx)
    require.NoError(t, err)
    assert.NotEmpty(t, data)
}

func TestMyCustomSourceCancel(t *testing.T) {
    source, err := NewMyCustomSource(...)
    require.NoError(t, err)
    defer source.Close()
    
    ctx, cancel := context.WithCancel(context.Background())
    cancel() // 立即取消
    
    _, err = source.Read(ctx)
    assert.ErrorIs(t, err, context.Canceled)
}
```

### 3. 关键要求

- ✅ 支持 context 取消
- ✅ 返回 16-bit PCM, little-endian
- ✅ Close() 幂等性
- ✅ 并发安全
- ✅ 包含单元测试

## 性能建议

### 缓冲区大小

- **推荐**: 3200 samples（16kHz 下约 200ms）
- **过小**: 频繁系统调用，CPU 开销大
- **过大**: 增加延迟，影响实时性

### 内存优化

- 复用缓冲区，减少 GC 压力
- 避免不必要的内存拷贝

### 错误处理

- 明确区分 EOF、Canceled 和其他错误
- 使用 `internal/logging` 记录关键事件

## 迁移指南

从旧版本迁移：

```go
// 旧版本
import "github.com/liuscraft/orion-x/internal/audio"
micSource, _ := audio.NewMicrophoneSource(16000, 1, 3200)

// 新版本
import "github.com/liuscraft/orion-x/internal/audio/source"
micSource, _ := source.NewMicrophoneSource(16000, 1, 3200)
```

## 参考文档

- [AudioSource 设计文档](../../../docs/audio-source-design.md)
- [AudioInPipe 设计文档](../../../docs/audio-in-pipe-design.md)
- [PortAudio 官方文档](http://www.portaudio.com/)