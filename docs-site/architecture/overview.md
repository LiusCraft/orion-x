# 语音机器人架构设计

## 架构概述

采用 **Orchestrator + Pipeline + Audio Mixing** 模式，支持工具调用、音频混音、情绪标注和用户打断。

```
用户说话 → ASR → LLM流式处理
                      ↓
                 [工具调用] → 工具执行 → 返回资源音频/数据
                      ↓
            [LLM总结] → [情绪标注] → [Markdown过滤] → [分句]
                      ↓
         TTSPipeline.EnqueueText() ← 非阻塞入队
                      ↓
              [TTS异步生成] → [音频预缓冲]
                      ↓
                   AudioMixer → 播放
                      ↑
               资源音频 ─────┘
```

## 核心模块

### 1. ConversationOrchestrator（对话编排器）
**职责**: 状态管理、事件路由、组件协调

**状态机**: `Idle` → `Listening` → `Processing` → `Speaking` → `Idle`

**接口**:
```go
type Orchestrator interface {
    Start(ctx context.Context) error
    Stop() error
    OnASRFinal(text string)
    OnUserSpeakingDetected()
    OnToolCall(tool string, args map[string]interface{})
    OnToolAudioReady(audio io.Reader)
    OnLLMTextChunk(chunk string)
}
```

### 2. VoiceAgent（语音Agent）
**职责**: LLM流式调用、工具调用、情绪标注、Markdown过滤

**流式约定**:
- LLM流式输出必须只发送新增的增量文本（delta）
- `text.Segmenter` 只消费增量文本并在成句时 flush 缓冲

**两种工具调用流程**:

#### 流程1: 直接播放类工具（如播放音乐）
```
用户输入 → 意图识别 → 识别工具 → 生成固定回复
  → 调用工具 → 工具返回资源音频 → TTS播报回复
```

#### 流程2: 查询类工具（如查询天气）
```
用户输入 → 意图识别 → 识别工具 → 调用工具获取数据
  → LLM总结数据 → 情绪标注 → Markdown过滤 → TTS播放
```

**工具分类**:
- `ToolTypeQuery`: 查询类（需要LLM总结）
- `ToolTypeAction`: 动作类（直接执行+播报）

**接口**:
```go
type VoiceAgent interface {
    Process(ctx context.Context, text string) (<-chan AgentEvent, error)
}

type AgentEvent interface {
    Type() EventType
}
```

### 3. ToolExecutor（工具执行器）
**职责**: 执行工具、管理资源音频

**接口**:
```go
type ToolExecutor interface {
    Execute(tool string, args map[string]interface{}) (result interface{}, audio io.Reader, err error)
}
```

**输出**:
- `result`: 工具返回的数据（用于LLM总结）
- `audio`: 资源音频流（如音乐文件）
- `err`: 错误信息

### 4. AudioOutPipe（音频输出管道）
**职责**: 音频混合播放、异步 TTS 处理、队列管理、中断处理

**架构**:
```
┌─────────────┐    ┌─────────────┐
│ TTSPipeline │    │ 资源音频源   │
│ (异步处理)   │    └──────┬──────┘
└──────┬──────┘           │
       │                  │
       └────────┬─────────┘
                ▼
         ┌─────────────┐
         │ AudioMixer  │  ← 音量控制逻辑
         └──────┬──────┘
                ▼
            音频播放器
```

**混音逻辑**:
- TTS音频: 正常音量播放
- 资源音频:
  - 无TTS时: 正常音量 (100%)
  - 有TTS时: 降低音量 (50%)

**TTS 播放流程（异步）**:
- `AudioOutPipe.PlayTTS()` 将文本入队到 `TTSPipeline`，立即返回
- TTSPipeline 内部维护文本队列、TTS Worker Pool、音频缓冲区
- 支持预缓冲最多 N 句已生成的音频（默认 3 句）
- 用户打断时，调用 `Interrupt()` 清空所有队列

### 5. TTSPipeline（TTS 异步管道）
**职责**: 异步 TTS 生成、音频预缓冲、播放协调

**核心组件**:
- **Text Queue**: 待处理文本队列（容量 100）
- **TTS Worker Pool**: 并发生成 TTS（默认 2 个 worker）
- **TTS Buffer**: 已生成音频缓冲区（默认 3 句）
- **Audio Player**: 从缓冲区取出并播放

**优势**:
- Agent 输出句子后立即继续，不阻塞在 TTS 播放上
- 提前生成多个 TTS，避免播放间隙
- 播放间隙 < 100ms，打断响应 < 200ms

### 6. TTSManager（TTS管理器）
**职责**: TTS连接管理、音色切换

**特性**:
- 音色映射: `map[string]string` (情绪 → 音色)
- 流式文本 → 音频转换
- 支持动态音色切换
- 支持多采样率输出（16k/22k/24k/48k）

### 7. AudioMixer（音频混音器）
**职责**: 音频混合、音量控制、采样率转换

**接口**:
```go
type AudioMixer interface {
    AddTTSStream(audio io.Reader)
    AddResourceStream(audio io.Reader)
    SetTTSVolume(volume float64)
    SetResourceVolume(volume float64)
    Start()
    Stop()
}
```

**启动约束**:
- `Start()` 采用异步启动，避免底层设备初始化阻塞主流程
- 启动失败会记录日志，不阻塞 Orchestrator 启动

**音量控制**:
```go
if isTTSPlaying {
    resourceVolume = 0.5
} else {
    resourceVolume = 1.0
}
```

### 8. EventBus（事件总线）
**职责**: 组件间异步通信

**事件类型**:
- `UserSpeakingDetected` (用户说话，触发中断)
- `ASRFinal` (识别完成)
- `ToolCallRequested` (工具调用请求)
- `ToolAudioReady` (工具返回音频)
- `LLMEmotionChanged` (情绪变化)
- `TTSInterrupt` (播放中断)

## 数据流设计

### 流程1: 直接播放类工具（播放音乐）
```
用户: "播放周杰伦的晴天"
  ↓
[ASR识别] → "播放周杰伦的晴天"
  ↓
[VoiceAgent意图识别] → 识别为播放音乐工具
  ↓
[生成回复] → "正在为您播放周杰伦的晴天" [EMO:calm]
  ↓
[TTS处理] → 生成音频 → AudioMixer → 播放
  ↓
[ToolExecutor调用音乐工具] → 返回音乐文件
  ↓
[资源音频] → AudioMixer → 降低音量播放（TTS播报时）
  ↓
[TTS播报完成] → 资源音频恢复正常音量
```

### 流程2: 查询类工具（查询天气）
```
用户: "今天北京天气怎么样"
  ↓
[ASR识别] → "今天北京天气怎么样"
  ↓
[VoiceAgent意图识别] → 识别为天气工具（查询类）
  ↓
[调用天气工具] → 返回: "北京，晴，25°C"
  ↓
[LLM总结] → "北京今天天气不错，晴朗，温度25度" [EMO:happy]
  ↓
[Markdown过滤] → 过滤特殊符号
  ↓
[分句] → "北京今天天气不错，晴朗，温度25度"
  ↓
[TTS处理] → 选择happy音色 → 生成音频 → 播放
```

### 中断流程
```
[TTS正在播放回答]
  ↓
用户: "等一下"
  ↓
[AudioInPipe检测到说话] → 发送UserSpeakingDetected事件
  ↓
[Orchestrator] → 转换状态为Listening
  ↓
[AudioOutPipe] → 停止播放 → 清空队列
  ↓
[ASR开始识别新输入]
```

## 关键实现细节

### 0. 配置管理

- 配置文件统一管理日志、ASR、TTS、LLM、音频与工具参数。
- 默认路径 `config/voicebot.json`，支持命令行 `-config` 覆盖。
- 加载顺序：默认值 → 配置文件 → 环境变量（`LOG_LEVEL`/`LOG_FORMAT`/`DASHSCOPE_API_KEY`/`ZHIPU_API_KEY`）。

### 1. 工具分类
```go
type ToolType int

const (
    ToolTypeQuery ToolType = iota  // 查询类：需要LLM总结
    ToolTypeAction                 // 动作类：直接执行+播报
)

var toolTypes = map[string]ToolType{
    "getWeather":  ToolTypeQuery,
    "getTime":     ToolTypeQuery,
    "playMusic":   ToolTypeAction,
    "setVolume":   ToolTypeAction,
}
```

### 2. 直接播放类工具的回复生成
```go
func generateActionResponse(tool, args string) string {
    switch tool {
    case "playMusic":
        return fmt.Sprintf("正在为您播放%s", args["song"])
    case "setVolume":
        return fmt.Sprintf("已将音量设置为%s", args["level"])
    default:
        return "好的，正在为您处理"
    }
}
```

### 3. 音频混音逻辑
```go
type AudioMixer struct {
    ttsVolume      float64
    resourceVolume float64
    ttsPlaying     bool
}

func (m *AudioMixer) OnTTSStarted() {
    m.ttsPlaying = true
    m.resourceVolume = 0.5  // 资源音频降低到50%
}

func (m *AudioMixer) OnTTSFinished() {
    m.ttsPlaying = false
    m.resourceVolume = 1.0  // 资源音频恢复正常
}
```

### 4. 情绪标注格式
```go
// LLM输出格式
"回答内容 [EMO:emotion]"
// 示例
"北京今天天气不错，晴朗，温度25度 [EMO:happy]"
"很抱歉，我没有找到相关信息 [EMO:sad]"
```

### 5. Markdown过滤正则
```go
markdownPatterns = []string{
    `\*\*.*?\*\*`,        // 加粗
    `__.*?__`,            // 下划线
    "```.*?```",          // 代码块
    `\[.*?\]\(.*?\)`,     // 链接
    `!\[.*?\]\(.*?\)`,    // 图片
}
```

### 6. 音色映射
```go
emotionToVoice = map[string]string{
    "happy":   "longanyang",
    "sad":     "zhichu",
    "angry":   "zhimeng",
    "calm":    "longxiaochun",
    "default": "longanyang",
}
```

## 目录结构

```
internal/
├── voicebot/
│   ├── orchestrator.go      # ConversationOrchestrator
│   ├── state.go             # 状态机定义
│   ├── events.go            # 事件定义
│   └── eventbus.go          # 事件总线
├── agent/
│   ├── voice_agent.go       # VoiceAgent
│   ├── tools.go             # 工具分类器
│   ├── processor.go         # LLM流处理器
│   └── events.go            # Agent事件定义
├── audio/
│   ├── mixer.go             # AudioMixer
│   ├── outpipe.go           # AudioOutPipe
│   ├── tts_pipeline.go      # TTSPipeline
│   ├── tts_pipeline_impl.go # TTSPipeline 实现
│   ├── resampler.go         # 重采样器
│   └── inpipe.go            # AudioInPipe (基于ASR)
├── text/
│   ├── segmenter.go         # 已有
│   └── markdown_filter.go   # Markdown过滤器
├── asr/
│   ├── recognizer.go        # 已有
│   └── dashscope.go         # 已有
├── tts/
│   ├── tts.go               # 已有
│   └── dashscope.go         # 已有
└── tools/
    ├── executor.go          # ToolExecutor
    ├── music.go             # 音乐播放工具示例
    └── weather.go           # 天气查询工具示例

cmd/voicebot/
└── main.go                  # 主程序入口
```

## 扩展点

1. **音频效果器**: 添加淡入淡出、混响等效果
2. **多轮对话记忆**: 添加上下文管理
3. **WebSocket服务**: 支持远程客户端
4. **更精细的情绪识别**: 替换LLM标注为独立模型

## 依赖关系

- `internal/ai/llm.go`: 现有LLM工具调用框架
- `internal/asr/*`: 现有ASR模块
- `internal/tts/*`: 现有TTS模块
- `internal/text/segmenter.go`: 现有分句器

## 实现步骤

1. 创建核心包结构（voicebot、agent、audio、tools）
2. 实现 ConversationOrchestrator 和状态机
3. 实现 VoiceAgent（集成现有工具调用逻辑）
4. 实现 AudioMixer（双通道混音）
5. 实现 TTSPipeline（异步 TTS 处理）
6. 实现 AudioOutPipe（集成 TTSPipeline）
7. 实现 Resampler（多采样率支持）
8. 实现 ToolExecutor（工具执行器）
9. 实现事件总线
10. 集成测试和调优
