# 异步 TTS Pipeline 架构设计

## 文档版本
- **版本**: v1.0
- **日期**: 2024-01
- **状态**: 已实现

## 1. 背景和问题

### 1.1 原有架构问题

**现状**：
- `AudioOutPipe.PlayTTS()` 是**同步阻塞**的调用
- Orchestrator 在处理 LLM TextChunk 时会被阻塞
- 每句话必须等前一句 TTS **完全播放完成**才能开始下一句
- 导致用户感知延迟增加，存在明显卡顿

**影响**：
1. **响应延迟高**：第一句话播放时（假设 3 秒），第二句话还在等待，无法提前生成 TTS
2. **卡顿明显**：第一句播放完后，第二句才开始调用 TTS，中间有明显停顿
3. **打断不及时**：用户说话时，可能正在等待 `stream.Close()`，无法立即响应

### 1.2 设计目标

1. **Agent 输出不等待播放**：LLM 输出句子后立即继续，不阻塞在 TTS 播放上
2. **TTS 预缓冲**：提前生成多个 TTS，避免播放间隙
3. **可配置缓冲数量**：最多缓冲 N 句已生成的音频（如 3 句）
4. **文本队列缓冲**：超出 TTS 缓冲的句子放在文本队列中排队
5. **快速打断**：用户说话时，立即停止 Agent 生成 + 清空所有缓冲 + 停止播放

## 2. 架构设计

### 2.1 整体架构图

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
│  │  ["句子1", "句子2", "句子3", ...]                     │   │
│  └────────────┬─────────────────────────────────────────┘   │
│               │ Text Consumer (Goroutine)                    │
│               ↓                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  TTS Worker Pool (并发度可配置，默认 2)               │   │
│  │  Worker-1: 生成 TTS Stream-1                          │   │
│  │  Worker-2: 生成 TTS Stream-2                          │   │
│  └────────────┬─────────────────────────────────────────┘   │
│               │ Generated TTS Streams                        │
│               ↓                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  TTS Buffer (有限，默认 3 句)                          │   │
│  │  [Stream-1, Stream-2, Stream-3]                       │   │
│  │  超出则阻塞 Worker，等待播放器消费                     │   │
│  └────────────┬─────────────────────────────────────────┘   │
│               │ Audio Player (Goroutine)                     │
│               ↓                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Playing Stream                                       │   │
│  │  当前正在播放的 Stream                                 │   │
│  └────────────┬─────────────────────────────────────────┘   │
└───────────────┼──────────────────────────────────────────────┘
                │
                ↓
        ┌───────────────┐
        │  AudioMixer   │
        │  混音 + 播放   │
        └───────────────┘
```

### 2.2 核心组件

#### TTSPipeline

**职责**：
- 管理文本队列、TTS 生成队列、播放队列
- 协调多个 goroutine：Text Consumer、TTS Workers、Audio Player
- 支持快速中断（清空所有队列）

**接口**：
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
    
    // Stats 获取统计信息（用于调试和监控）
    Stats() PipelineStats
    
    // SetMixer 设置音频混音器
    SetMixer(mixer AudioMixer)
    
    // SetReferenceSink 设置参考音频输出（用于 AEC）
    SetReferenceSink(sink ReferenceSink)
}
```

### 2.3 队列设计

**1. Text Queue（文本队列）**：
- 类型：带缓冲的 channel
- 容量：可配置（默认 100）
- 阻塞策略：入队时如果满了则阻塞（保护内存）

**2. TTS Buffer（TTS 缓冲区）**：
- 类型：带缓冲的 channel
- 容量：有限（默认 3）
- 阻塞策略：TTS Worker 生成完后入队，如果满了则阻塞等待

**3. Playing Stream（正在播放的流）**：
- 类型：单个变量（受 mutex 保护）
- 职责：Audio Player 从 ttsBuffer 取出后设置

### 2.4 Goroutine 设计

#### Text Consumer
从 textQueue 取出文本，启动 TTS Worker 生成音频。

#### TTS Worker Pool
- 使用 semaphore 控制并发数
- 生成 TTS 音频流，放入 ttsBuffer
- 如果 ttsBuffer 满了会阻塞，等待播放器消费

#### Audio Player
从 ttsBuffer 取出 TTS Stream，添加到 Mixer 播放。

### 2.5 打断机制

**触发时机**：
- 用户说话（VAD 检测或 ASR 部分识别）
- Orchestrator 调用 `TTSPipeline.Interrupt()`

**打断流程**：
1. 取消当前 context（通知所有 worker 停止）
2. 立即停止当前播放（从 Mixer 移除）
3. 等待所有 worker 退出
4. 清空所有队列
5. 重新创建 context 和 workers，准备接收新任务

### 2.6 Agent 取消机制

**Orchestrator 改动**：
- 为每次 `Agent.Process()` 创建独立的 context
- 用户打断时取消 Agent context，停止 LLM 生成
- 同时调用 `TTSPipeline.Interrupt()` 清空队列

## 3. 配置

### 3.1 TTSPipelineConfig

```go
type TTSPipelineConfig struct {
    // MaxTTSBuffer TTS 缓冲区最大容量
    // 已生成但未播放的 TTS 流数量上限
    // 默认: 3
    MaxTTSBuffer int `json:"max_tts_buffer"`

    // MaxConcurrentTTS 最大并发 TTS 生成数
    // 控制同时调用 TTS 服务的数量，避免过多并发
    // 默认: 2
    MaxConcurrentTTS int `json:"max_concurrent_tts"`

    // TextQueueSize 文本队列大小
    // 待处理的文本数量上限，防止内存爆炸
    // 默认: 100
    TextQueueSize int `json:"text_queue_size"`
}
```

### 3.2 JSON 配置示例

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

## 4. 文件结构

```
internal/audio/
├── tts_pipeline.go           # 接口和类型定义
├── tts_pipeline_impl.go      # 核心实现
├── tts_pipeline_test.go      # 单元测试
├── outpipe.go                # AudioOutPipe 接口（已更新）
├── outpipe_impl.go           # AudioOutPipe 实现（集成 TTSPipeline）
└── mocks_test.go             # 测试用 mock

internal/voicebot/
└── orchestrator.go           # 已更新，支持 Agent context 取消
```

## 5. 关键改动

### 5.1 AudioOutPipe 接口

```go
type AudioOutPipe interface {
    Start(ctx context.Context) error
    Stop() error
    // PlayTTS 播放 TTS（异步，立即返回）
    PlayTTS(text string, emotion string) error
    PlayResource(audio io.Reader) error
    // Interrupt 中断所有任务（清空队列、停止播放）
    Interrupt() error
    SetMixer(mixer AudioMixer)
    SetReferenceSink(sink ReferenceSink)
    // Stats 获取 Pipeline 统计信息
    Stats() PipelineStats
}
```

### 5.2 Orchestrator 改动

- 新增 `agentCtx` 和 `agentCancel` 字段
- `handleASRFinal()` 为新的 Agent 调用创建独立 context
- `handleUserSpeakingDetected()` 取消 Agent 并调用 `Interrupt()`

## 6. 性能预期

- **播放间隙**：< 100ms（第一句播放时，第二句已生成完毕）
- **打断响应**：< 200ms（立即停止生成和播放）
- **内存占用**：稳定（队列有限制，超出则阻塞）

## 7. 测试覆盖

- `TestTTSPipelineCreate` - 创建 Pipeline
- `TestTTSPipelineStartStop` - 启动和停止
- `TestTTSPipelineDoubleStart` - 重复启动
- `TestTTSPipelineEnqueueText` - 入队文本
- `TestTTSPipelineEnqueueEmpty` - 入队空文本
- `TestTTSPipelineEnqueueBeforeStart` - 启动前入队
- `TestTTSPipelineInterrupt` - 打断
- `TestTTSPipelineConcurrentEnqueue` - 并发入队
- `TestTTSPipelineStats` - 统计信息
- `TestTTSPipelineVoiceMap` - 音色映射
- `TestTTSPipelineContextCancellation` - context 取消
- `TestTTSPipelineTTSError` - TTS 错误处理
- `TestTTSPipelineMaxConcurrentTTS` - 最大并发数
- `TestTTSPipelineResetAfterInterrupt` - 打断后重置
- `TestTTSPipelineSetMixerDynamic` - 动态设置 Mixer
- `TestTTSPipelineRaceCondition` - 竞态条件

## 8. 相关文件

- `internal/audio/tts_pipeline.go` - TTSPipeline 接口
- `internal/audio/tts_pipeline_impl.go` - TTSPipeline 实现
- `internal/audio/outpipe_impl.go` - AudioOutPipe 集成
- `internal/voicebot/orchestrator.go` - Orchestrator 更新
- `internal/config/config.go` - 配置结构
- `config/voicebot.example.json` - 配置示例