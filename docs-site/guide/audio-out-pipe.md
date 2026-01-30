# 音频输出管道 (AudioOutPipe)

## 概述

AudioOutPipe 是音频输出管道，负责 TTS 音频和资源音频的播放、混音和中断处理。

## 核心功能

1. **TTS 播放**：异步调用 TTS 服务生成音频并播放
2. **资源播放**：播放工具返回的资源音频（如音乐）
3. **音频混音**：双通道音频混音，TTS 播放时降低资源音量
4. **中断处理**：响应中断信号，停止所有播放

## 接口设计

```go
type AudioOutPipe interface {
    // Start 启动输出管道
    Start(ctx context.Context) error

    // Stop 停止输出管道
    Stop() error

    // PlayTTS 播放 TTS（异步，立即返回）
    PlayTTS(text string, emotion string) error

    // PlayResource 播放资源音频
    PlayResource(audio io.Reader) error

    // Interrupt 中断所有任务（清空队列、停止播放）
    Interrupt() error

    // SetMixer 设置音频混音器
    SetMixer(mixer AudioMixer)

    // Stats 获取 Pipeline 统计信息
    Stats() PipelineStats
}
```

## 架构设计

```
┌─────────────────────────────────────────────────────────┐
│                    AudioOutPipe                         │
│                                                         │
│  ┌──────────────┐         ┌──────────────┐             │
│  │ TTS Pipeline │         │ Resource Q   │             │
│  │ (异步处理)    │         │              │             │
│  └──────┬───────┘         └──────┬───────┘             │
│         │                        │                      │
│         └────────────┬───────────┘                      │
│                      ▼                                  │
│         ┌─────────────────────────┐                     │
│         │     AudioMixer          │                     │
│         │  · TTS Volume: 100%     │                     │
│         │  · Resource: 50%/100%   │                     │
│         └────────────┬────────────┘                     │
│                      │                                  │
│                      ▼                                  │
│         ┌─────────────────────────┐                     │
│         │    PortAudio Player     │                     │
│         └─────────────────────────┘                     │
└─────────────────────────────────────────────────────────┘
```

## 混音逻辑

### 音量控制规则

```go
if isTTSPlaying {
    resourceVolume = 0.5  // TTS 播放时，资源音频降为 50%
} else {
    resourceVolume = 1.0  // TTS 停止时，资源音频恢复 100%
}
```

### 播放优先级

1. **TTS 播放**：最高优先级，独占音频输出
2. **资源播放**：后台播放，TTS 启动时自动降低音量

## TTS 播放流程

```
PlayTTS(text, emotion)
    ↓
TTSPipeline.EnqueueText(text, emotion)  // 非阻塞入队
    ↓
TTS Worker 生成音频
    ↓
添加到 AudioMixer
    ↓
播放完成
```

## 音色映射

根据情绪自动切换 TTS 音色：

| 情绪 | 音色 | 说明 |
|------|------|------|
| happy | longanyang | 开朗男声 |
| sad | zhichu | 悲伤女声 |
| angry | zhimeng | 愤怒男声 |
| calm | longxiaochun | 平静男声 |
| default | longanyang | 默认 |

## 配置

### OutPipeConfig

```go
type OutPipeConfig struct {
    // TTSPipeline TTS 管道配置
    TTSPipeline TTSPipelineConfig `json:"tts_pipeline"`

    // Mixer 混音器配置
    Mixer MixerConfig `json:"mixer"`
}
```

### JSON 配置示例

```json
{
  "audio": {
    "out_pipe": {
      "tts_pipeline": {
        "max_tts_buffer": 3,
        "max_concurrent_tts": 2,
        "text_queue_size": 100
      },
      "mixer": {
        "tts_volume": 1.0,
        "resource_volume": 1.0,
        "sample_rate": 16000,
        "channels": 2
      }
    }
  }
}
```

## 中断处理

### Interrupt() 流程

1. 取消 TTS Pipeline（停止所有 TTS 生成和播放）
2. 从 Mixer 移除所有音频流
3. 清空播放队列

### 使用场景

- 用户说话时打断 AI 回复
- 工具调用需要立即播放资源音频
- 系统停止/关闭

## 文件结构

```
internal/audio/
├── outpipe.go              # AudioOutPipe 接口
├── outpipe_impl.go         # AudioOutPipe 实现（集成 TTSPipeline）
└── mixer.go                # AudioMixer 实现
```
