# AudioSink 设计文档

## 背景

当前 `AudioMixer` 直接依赖 PortAudio 进行播放，导致：
- 混音逻辑与播放实现耦合
- 服务端部署也需要引入客户端音频库

引入 `AudioSink` 作为音频输出目标抽象，实现混音与播放解耦。

## 设计目标

- `AudioMixer` 只负责混音，不感知具体播放实现
- 支持多种输出（本地播放 / WebSocket / 文件）
- 服务端部署不需要依赖 PortAudio
- 新增输出实现只需实现 `AudioSink`

## 核心接口

```go
// internal/audio/sink.go
type AudioFormat struct {
    SampleRate      int
    Channels        int
    FramesPerBuffer int
}

// AudioSink receives interleaved PCM16 samples.
type AudioSink interface {
    Start(ctx context.Context, format AudioFormat) error
    WritePCM(samples []int16) error
    Stop() error
}
```

## 数据格式

- **采样格式**：PCM 16-bit little-endian
- **声道排列**：interleaved（L,R,L,R,...）
- **采样率/声道数**：由 `AudioMixer` 配置决定

## 模块职责

- **AudioMixer**：混音、音量控制、输出 PCM16
- **AudioSink**：播放/写出 PCM16
- **Sink 实现**：位于 `internal/audio/sink/`
  - `PortAudioSink`：本地扬声器播放
  - 预留：`WebSocketSink`、`FileSink`

## 流程

```
AudioOutPipe → TTSPipeline → AudioMixer → AudioSink
```

## 依赖关系

- `internal/audio` 定义 `AudioSink` 接口
- `internal/audio/sink` 实现具体输出
- `AudioMixer` 只依赖 `AudioSink` 接口，不直接依赖 PortAudio

