# 日志更新说明

## 日志库

采用 `zap` 作为统一日志库，提供结构化日志与可配置日志级别/格式。

**环境变量**:
- `LOG_LEVEL`: `debug|info|warn|error`，默认 `info`
- `LOG_FORMAT`: `console|json`，默认 `console`

## 日志 ID 方案

当前单客户端运行模式下，整个进程使用一个固定 `traceId`，每一轮完整交互生成一个新的 `turnId`。

**完整周期**:
用户说话检测/ASR Final 触发处理 → LLM/工具处理 → TTS 播放 → 回到 Idle

日志中同时注入:
- `trace_id`
- `turn_id`
- `log_id`: `traceId-turnId`

## 添加的日志

### AudioInPipe

**启动/停止日志**
- AudioInPipe 启动
- 音频源启动
- 音频读取协程启动/停止
- 停止和清理过程

**音频处理日志**
- 读取音频错误
- 发送音频到 ASR 错误

### MicrophoneSource

**初始化日志**
- PortAudio 初始化
- 流参数（采样率、声道、缓冲区大小）
- 流启动

**关闭日志**
- 流停止/关闭错误
- PortAudio 终止

### Orchestrator

**Start/Stop 日志**
- 启动时的详细步骤（AudioInPipe、AudioOutPipe、事件处理器）
- 停止时的清理过程
- 状态变化日志

**事件处理日志**
- UserSpeakingDetected: 记录用户说话检测和当前状态
- ASRFinal: 记录识别到的文本
- ToolCallRequested: 记录工具名称和参数
- ToolAudioReady: 记录资源音频播放
- OutputEmotionChanged: 记录情绪变化
- AgentEvent: 记录每个句子、工具调用、完成事件

### Agent

**流程日志**
- 输入文本
- LLM 流开始/完成
- 文本块和情绪变化
- 工具调用请求
- 动作回复生成

**错误日志**
- LLM 流错误
- 流接收错误

### AudioOutPipe

**TTS 播放日志**
- 文本、情绪、音色
- TTS 流开始/写入/关闭
- 混音器操作

**资源播放日志**
- 资源流添加

### AudioInPipe

**现有日志**
- 启动/停止
- 状态变化

### ToolExecutor

**工具执行日志**
- 工具注册
- 工具执行（名称和参数）

### 工具实现

**GetWeatherTool**
- 查询城市
- 返回结果

**GetTimeTool**
- 获取当前时间
- 返回结果

## 日志格式示例

```
========================================
        VoiceBot Starting...
========================================
API key loaded successfully
Creating Agent...
Agent created successfully
Creating AudioMixer...
AudioMixer created successfully
Creating AudioOutPipe...
AudioOutPipe created successfully
Creating AudioInPipe...
AudioInPipe created successfully
Creating ToolExecutor and registering tools...
ToolExecutor: registered tool: getTime
ToolExecutor: registered tool: getWeather
Tools registered successfully
Creating Orchestrator...
Orchestrator created successfully
Starting Orchestrator...
Orchestrator: starting...
Orchestrator: event handlers registered
Orchestrator: starting AudioInPipe...
AudioInPipe: started, state: Listening
Orchestrator: AudioInPipe started
Orchestrator: starting AudioOutPipe...
AudioOutPipe: started
Orchestrator: AudioOutPipe started
Orchestrator: started successfully, current state: Idle
========================================
     VoiceBot is Running! 🎤
     Press Ctrl+C to stop.
========================================

[用户说话时]
Orchestrator: user speaking detected: 你好
Orchestrator: ASR final result: 你好
Orchestrator: ASR final event received: 你好
State changed: Idle -> Processing
Agent: processing input: 你好
Agent: starting LLM stream...
Agent: text chunk: 你好 (emotion: happy)
Orchestrator: playing TTS for sentence: 你好
AudioOutPipe: PlayTTS - text: 你好, emotion: happy, voice: longanyang
AudioOutPipe: starting TTS stream...
AudioOutPipe: writing text chunk to TTS...
AudioOutPipe: closing TTS stream...
AudioOutPipe: adding TTS stream to mixer...
AudioMixer: TTS started, reducing resource volume to 50%
AudioMixer: TTS finished, restoring resource volume to 100%
AudioMixer: failed to stop stream: Stream is stopped
AudioOutPipe: TTS stream removed from mixer
AudioOutPipe: PlayTTS completed
State changed: Processing -> Speaking
State changed: Speaking -> Idle
Agent: processing finished

[工具调用时]
Agent: tool call requested: getTime, args: map[]
ToolExecutor: executing tool: getTime, args: map[]
GetTimeTool: getting current time
GetTimeTool: time result: map[...]
ToolExecutor: executing tool: getTime, args: map[]
Orchestrator: ToolCallRequested event - tool: getTime, args: map[]
Orchestrator: Tool execution result: map[...]
```

## 日志级别

当前所有日志都使用 `log.Printf`，可以考虑在未来支持不同日志级别：
- DEBUG: 详细调试信息
- INFO: 一般信息
- WARN: 警告信息
- ERROR: 错误信息
