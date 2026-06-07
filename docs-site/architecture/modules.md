# 语音机器人模块拆分文档

## 已创建的模块结构

```
internal/
├── voicebot/          # 对话编排模块
│   ├── orchestrator.go # 对话编排器接口和状态定义
│   ├── state.go       # 状态机实现
│   ├── events.go      # 事件定义和实现
│   └── eventbus.go    # 事件总线实现
├── agent/             # 通用 Agent 模块
│   ├── agent.go # Agent接口
│   ├── runtime.go     # Agent运行主流程
│   ├── output_metadata_node.go # 输出元数据提取
│   └── events.go      # Agent事件定义
├── audio/             # 音频处理模块
│   ├── mixer.go       # AudioMixer接口
│   ├── outpipe.go     # AudioOutPipe接口
│   └── inpipe.go      # AudioInPipe接口
├── text/              # 文本处理模块
│   ├── segmenter.go   # 分句器（已存在）
│   └── markdown_filter.go # Markdown过滤器
├── tools/             # 工具执行模块
│   ├── executor.go    # ToolExecutor接口
│   ├── music.go       # 音乐工具示例
│   └── weather.go     # 天气工具示例
├── config/            # 配置管理模块
│   ├── config.go       # 配置结构与加载
├── asr/               # ASR模块（已存在）
│   ├── recognizer.go
│   └── dashscope.go
└── tts/               # TTS模块（已存在）
    ├── tts.go
    └── dashscope.go
```

## 各模块接口概览

### 1. voicebot 包

#### Orchestrator (接口)
- `Start(ctx context.Context) error`
- `Stop() error`
- `GetState() State`
- `OnASRFinal(text string)`
- `OnUserSpeakingDetected()`
- `OnToolCall(tool string, args map[string]interface{})`
- `OnToolAudioReady(audio io.Reader)`
- `OnLLMTextChunk(chunk string)`
- `OnLLMFinished()`

**实现细节**：
- 集成 `text.Segmenter` 进行流式文本分句
- 接收 `Agent` 的 `TextChunkEvent` 进行分句处理
- 对每个完整句子调用 `AudioOutPipe.PlayTTS()` 生成和播放音频
- 在 `FinishedEvent` 时调用 `Segmenter.Flush()` 处理剩余文本
- 状态转换：`Processing` → `Speaking`（开始播放时）→ `Idle`（完成时）

#### EventBus (接口)
- `Publish(event Event)`
- `Subscribe(eventType EventType, handler EventHandler)`

#### 状态机
- State: `Idle`, `Listening`, `Processing`, `Speaking`
- 支持状态转换检查和自动转换

#### 事件类型
- `UserSpeakingDetected` - 用户说话事件
- `ASRFinal` - ASR识别完成事件
- `ToolCallRequested` - 工具调用请求事件
- `ToolAudioReady` - 工具返回音频事件
- `OutputEmotionChanged` - 输出情绪变化事件
- `TTSInterrupt` - TTS播放中断事件
- `StateChanged` - 状态变化事件

### 2. agent 包

#### Agent（具体类型）
- `Process(ctx context.Context, text string) (<-chan AgentEvent, error)`

#### Agent Runtime
- `Process(ctx context.Context, text string) (<-chan AgentEvent, error)`
- 负责 LLM 流式输出、工具调用请求事件、工具结果总结

#### OutputMetadataNode (接口)
- `Process(text string) OutputMetadata`
- 在 Orchestrator 输出链路中从文本提取 `[EMO:xxx]` 标签

#### 文本清洗
- `text.FilterMarkdown(text string) string`
- `voicebot.TextFilterNode.Process(text string) string`
- `TextFilterNode` 在 Orchestrator 进入 TTS 前编排 Markdown 清洗与语音情绪标签移除

### 3. audio 包

#### AudioMixer (接口)
- `AddTTSStream(audio io.Reader)`
- `AddResourceStream(audio io.Reader)`
- `RemoveTTSStream()`
- `RemoveResourceStream()`
- `SetTTSVolume(volume float64)`
- `SetResourceVolume(volume float64)`
- `OnTTSStarted()` - 资源音频自动降为50%
- `OnTTSFinished()` - 资源音频恢复正常
- `SetSink(sink AudioSink)`
- `Start() error`, `Stop() error`

#### AudioSink (接口)
- `Start(ctx context.Context, format AudioFormat) error`
- `WritePCM(samples []int16) error`
- `Stop() error`

#### AudioOutPipe (接口)
- `Start(ctx context.Context) error`
- `Stop() error`
- `PlayTTS(text string, emotion string) error`
- `PlayResource(audio io.Reader) error`
- `Interrupt() error`
- `SetMixer(mixer AudioMixer)`

**实现细节**：
- 集成 `tts.DashScopeProvider` 进行文本到音频的转换
- 根据情绪映射到不同的音色
- 音色映射表：
  - `happy` → `longanyang`
  - `sad` → `zhichu`
  - `angry` → `zhimeng`
  - `calm` → `longxiaochun`
  - `excited` → `longanyang`
  - `default` → `longanyang`
- 使用 `AudioMixer` 管理音频流播放
- 支持中断功能（清空 TTS 流和资源音频）
- 并发安全（使用 `sync.Mutex`）

#### AudioInPipe (接口)
- `Start(ctx context.Context) error`
- `Stop() error`
- `SendAudio(audio []byte) error`
- `OnASRResult(handler func(text string, isFinal bool))`
- `OnUserSpeakingDetected(handler func())`

### 4. text 包

#### 文本清洗函数
- `FilterMarkdown(text string) string`
- `RemoveEmotionTags(text string) string`

### 5. tools 包

#### ToolExecutor (接口)
- `Execute(tool string, args map[string]interface{}) (result interface{}, audio io.Reader, error)`
- `RegisterTool(name string, executor ToolExecutorFunc)`

#### 工具示例
- `PlayMusicTool` - 播放音乐，返回音频流
- `PauseMusicTool` - 暂停音乐
- `SetVolumeTool` - 设置音量
- `GetWeatherTool` - 获取天气信息
- `GetTimeTool` - 获取当前时间
- `SearchTool` - 搜索

### 6. config 包

#### AppConfig
- 统一管理日志、ASR、TTS、LLM、音频与工具配置
- 支持从配置文件加载并与环境变量合并

## 关键设计点

### 1. 工具调用流程

#### 直接播放类工具（如播放音乐）
```
用户输入 → 意图识别 → 识别工具 → 生成固定回复
  → 调用工具 → 工具返回资源音频 → TTS播报回复
```

#### 工具结果总结（如查询天气）
```
用户输入 → 意图识别 → 识别工具 → 调用工具获取数据
  → LLM总结数据 → 情绪标注 → Markdown过滤 → TTS播放
```

### 2. 音频混音逻辑

```
TTS音频 ────┐
           ├─→ AudioMixer → 播放
资源音频 ──┘

混音规则:
- TTS播放时: 资源音频降为50%音量
- TTS停止时: 资源音频恢复100%音量
```

### 3. 情绪标注格式

```
回答内容 [EMO:emotion]
示例:
"北京今天天气不错，晴朗，温度25度 [EMO:happy]"
"很抱歉，我没有找到相关信息 [EMO:sad]"
```

### 4. Markdown过滤

支持过滤:
- 加粗: `**text**`
- 代码块: ````code````
- 行内代码: `` `code` ``
- 链接: `[text](url)`
- 图片: `![alt](url)`
- 标题: `## Heading`

### 5. 状态转换

```
Idle ─────→ Processing ───→ Speaking
  ↑            ↓               ↑
  └────Listening←───────────────┘
```

## 下一步工作

### 已完成功能

1. **voicebot 包**
    - [x] `Orchestrator` 完整实现
    - [x] 事件总线和状态机集成
    - [x] 集成 `text.Segmenter` 分句器和 TTS 生成

2. **agent 包**
    - [x] `Agent` 完整实现
    - [x] 集成 `internal/agent`、`internal/tools` 与 `internal/provider/llm` 的工具调用逻辑
    - [x] 启用流式LLM输出
    - [x] 集成 `OutputMetadataNode` 和 `internal/text` 文本清洗

3. **audio 包**
    - [x] `AudioMixer` 实现（音频混音）
    - [x] `AudioOutPipe` 实现（基于现有TTS）
    - [x] `AudioInPipe` 实现（基于现有ASR）

4. **text 包**
    - [x] Markdown 与语音文本清洗完整实现

5. **tools 包**
    - 实现真实工具（如天气API调用）
    - 工具注册和发现机制

### 需要集成的现有模块

- `internal/agent/*` - Agent 与 LLM 工具调用编排
- `internal/tools/*` - 工具加载、注册与执行
- `internal/provider/llm/*` - LLM provider 模块
- `internal/provider/asr/*` - ASR provider 模块
- `internal/provider/tts/*` - TTS provider 模块
- `internal/text/segmenter.go` - 分句器

### 扩展功能

- 音色映射表（情绪→音色）
- VAD监听实现
- 音频播放器集成
- 配置文件支持
- 日志和监控
