# TTS 异步管道

## 概述

TTSPipeline 是一个异步的 TTS（文本转语音）处理管道，解决了原有同步阻塞调用带来的高延迟和卡顿问题。

### 原有问题

- `AudioOutPipe.PlayTTS()` 是同步阻塞调用
- Orchestrator 处理 LLM TextChunk 时被阻塞
- 每句话必须等前一句完全播放才能开始下一句
- 用户感知延迟高，存在明显卡顿

### 设计目标

1. **非阻塞输出**：LLM 输出句子后立即继续，不等待 TTS 播放
2. **TTS 预缓冲**：提前生成多个 TTS，避免播放间隙
3. **可配置缓冲**：最多缓冲 N 句已生成的音频（默认 3 句）
4. **快速打断**：用户说话时立即停止生成和播放

## 架构设计

```
┌──────────────────────────────────────────────────────────────┐
│                     Orchestrator                              │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  Agent.Process(ctx) - LLM 流式输出                      │  │
│  │  ↓ TextChunkEvent                                       │  │
│  │  Segmenter.Feed() → 完整句子                            │  │
│  │  ↓                                                       │  │
│  │  TTSPipeline.EnqueueText(sentence, emotion) ← 非阻塞!   │  │
│  └────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘
                            ↓
┌──────────────────────────────────────────────────────────────┐
│                     TTSPipeline                               │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Text Queue (可配置大小，默认 100)                     │   │
│  └────────────┬─────────────────────────────────────────┘   │
│               │ Text Consumer (Goroutine)                    │
│               ↓                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  TTS Worker Pool (并发度可配置，默认 2)               │   │
│  └────────────┬─────────────────────────────────────────┘   │
│               │ Generated TTS Streams                        │
│               ↓                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  TTS Buffer (有限，默认 3 句)                          │   │
│  └────────────┬─────────────────────────────────────────┘   │
│               │ Audio Player (Goroutine)                     │
│               ↓                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Playing Stream                                       │   │
│  └────────────┬─────────────────────────────────────────┘   │
└───────────────┼──────────────────────────────────────────────┘
                │
                ↓
        ┌───────────────┐
        │  AudioMixer   │
        │  混音 + 播放   │
        └───────────────┘
```

## 核心组件

### TTSPipeline 接口

```go
type TTSPipeline interface {
    // EnqueueText 入队文本（非阻塞，立即返回）
    EnqueueText(text string, emotion string) error

    // Interrupt 中断所有任务（清空队列、停止播放）
    Interrupt() error

    // Start 启动 Pipeline
    Start(ctx context.Context) error

    // Stop 停止 Pipeline
    Stop() error

    // Stats 获取统计信息
    Stats() PipelineStats

    // SetMixer 设置音频混音器
    SetMixer(mixer AudioMixer)
}
```

### 队列设计

| 队列 | 类型 | 容量 | 作用 |
|------|------|------|------|
| Text Queue | 带缓冲 channel | 100 (可配置) | 待处理文本队列 |
| TTS Buffer | 带缓冲 channel | 3 (可配置) | 已生成音频缓冲 |
| Playing Stream | 单个变量 | 1 | 当前播放的流 |

### Goroutine 设计

- **Text Consumer**：从文本队列取出，启动 TTS Worker
- **TTS Worker Pool**：并发生成 TTS 音频，使用 semaphore 控制并发数
- **Audio Player**：从 TTS Buffer 取出并播放

## 配置

### TTSPipelineConfig

```go
type TTSPipelineConfig struct {
    // MaxTTSBuffer TTS 缓冲区最大容量
    MaxTTSBuffer int `json:"max_tts_buffer"`

    // MaxConcurrentTTS 最大并发 TTS 生成数
    MaxConcurrentTTS int `json:"max_concurrent_tts"`

    // TextQueueSize 文本队列大小
    TextQueueSize int `json:"text_queue_size"`
}
```

### JSON 配置示例

```json
{
  "audio": {
    "tts_pipeline": {
      "max_tts_buffer": 3,
      "max_concurrent_tts": 2,
      "text_queue_size": 100
    }
  }
}
```

## 打断机制

### 触发时机

- 用户说话（VAD 检测或 ASR 部分识别）
- Orchestrator 调用 `TTSPipeline.Interrupt()`

### 打断流程

1. 取消当前 context（通知所有 worker 停止）
2. 立即停止当前播放（从 Mixer 移除）
3. 等待所有 worker 退出
4. 清空所有队列
5. 重新创建 context 和 workers

### Agent 取消

Orchestrator 为每次 `Agent.Process()` 创建独立 context：
- 用户打断时取消 Agent context，停止 LLM 生成
- 同时调用 `TTSPipeline.Interrupt()` 清空队列

## 性能预期

| 指标 | 目标值 |
|------|--------|
| 播放间隙 | < 100ms |
| 打断响应 | < 200ms |
| 内存占用 | 稳定（队列有限制） |

## 文件结构

```
internal/audio/
├── tts_pipeline.go           # 接口和类型定义
├── tts_pipeline_impl.go      # 核心实现
└── tts_pipeline_test.go      # 单元测试
```
