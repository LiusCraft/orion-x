# 模块说明

本文按当前代码结构说明各模块职责。`cmd/voicebot` 是当前唯一可运行产品入口，`internal/*` 包提供可复用能力。

## 入口层

### `cmd/voicebot`

职责：

- 读取 `data/voicebot.json` 或 `-config` 指定的配置文件
- 初始化 zap logging、PortAudio、ToolManager、Memory Service、Agent
- 创建本地麦克风输入和 PortAudio 播放 sink
- 组装 `ASRStage -> AgentStage -> TTSStage`
- 处理 `SIGINT`、`SIGTERM` 并关闭 pipeline

当前入口是本地 CLI harness。真实产品可以在同一批 internal 包上扩展 WebSocket、GUI 或其他交互通道。

## Pipeline

### `internal/pipeline`

核心类型：

```go
type Stage interface {
    Name() string
    Process(ctx context.Context, input <-chan Message) <-chan Message
}
```

`pipeline.Message` 是 Stage 间传递的数据单元，主要类型包括：

- `text_partial`：ASR 中间结果
- `text_chunk`：ASR 最终文本或 Agent 文本增量
- `interrupt`：用户插话
- `finished`：Agent 完成
- `error`：错误消息
- `tts_start` / `tts_stop`：TTS 播放状态

`NewBuilder()` 当前构建线性 pipeline。包内也保留 DAG 相关实现和测试，供后续复杂拓扑使用。

### `internal/pipeline/stages`

| Stage | 依赖 | 说明 |
|---|---|---|
| `ASRStage` | `audio.InPipe` | source stage，启动音频输入和 ASR，输出文本与打断消息 |
| `AgentStage` | `agent.Agent`、`session.Session` | 维护对话 session，运行 Agent，支持被上游新输入取消 |
| `TTSStage` | `audio.AudioOutPipe` | 接收 Agent 文本流，管理 TTS stream 的 begin/write/end/interrupt |

## Agent 与 LLM

### `internal/agent`

Agent 负责：

- 组装 session messages 和 memory context
- 调用 LLM provider 的 `Chat`
- 流式输出新增文本 delta
- 收集并执行 tool calls
- 将 assistant/tool message 写回 session
- 控制多 step 工具调用循环

当前事件只有两类：

```go
type TextChunkEvent struct { Chunk string }
type FinishedEvent struct { Error error }
```

### `internal/llm`

定义 provider 无关的 LLM 类型，包括 `Message`、`Request`、`ToolDefinition`、`ToolCall`、`Stream`。当前 provider 是 OpenAI-compatible，注册在 `internal/llm/provider/openai`。

## 工具

### `internal/tools`

`Manager` 会先注册本地工具，再加载配置里的 MCP server。

当前本地工具：

| 工具 | 说明 |
|---|---|
| `getTime` | 返回当前时间、星期和 Unix timestamp |

核心注册结构：

```go
type Spec struct {
    Name        string
    Description string
    Parameters  map[string]any
    Execute     func(ctx context.Context, arguments json.RawMessage) (Result, error)
}
```

MCP server 支持 `stdio`、`sse`、`streamable` transport。可以用 `tool_name_list` 限制加载的工具集合。

## 音频

### `internal/audio`

主要边界：

- `AudioSource`：抽象音频输入源，当前由 `cmd/voicebot` 的 microphone 实现
- `InPipe`：读取音频、VAD 切段、调用 ASR recognizer
- `AudioOutPipe`：管理 TTS 输出、播放 sink 和中断
- `TTSPipeline`：异步 TTS 队列、buffer、播放协调
- `AudioSink`：抽象播放目标，当前由 PortAudio sink 实现
- `resampler`：线性插值重采样
- `vad`：Silero VAD ONNX 模型封装

当前没有单独的全局 AudioMixer 模块；播放链路以 TTS 输出为主，资源音频接口保留在 `AudioOutPipe` 边界上。

## Provider

### `internal/provider/asr`

ASR 使用 factory registration。当前实际实现是阿里云 DashScope realtime ASR。

### `internal/provider/tts`

TTS 使用 factory registration。当前实际实现是阿里云 DashScope CosyVoice，支持 voice、sample rate、volume、rate、pitch、SSML、data inspection 等配置。

## 会话与记忆

### `internal/session`

保存当前对话历史，支持 user、assistant、tool 角色和 tool call 元数据。`AgentStage` 将 ASR 最终文本写入 session，Agent 将 LLM 回复和工具结果继续追加。

### `internal/memory`

提供三种模式：

- `none`：不构建额外 memory context
- `session`：基于最近轮次和摘要构建上下文
- `long_term`：使用 SQLite 存储 turn 和抽取出的长期记忆，并通过 FTS 查询注入 prompt

## 文本处理

### `internal/text`

包含：

- `segmenter.go`：文本分句
- `markdown_filter.go`：过滤 Markdown 结构，避免 TTS 读出格式符
- `emotion_tags.go`：处理 `[EMO:xxx]` 语音情绪标签

当前主 pipeline 走流式 TTS chunk，文本处理能力主要供 Agent 输出和 TTS 前处理扩展复用。

## 配置与日志

### `internal/config`

负责 JSON 配置加载、默认值、环境变量覆盖、provider 类型校验、MCP 配置校验和 memory 配置校验。

默认路径是 `data/voicebot.json`，示例文件是项目根目录的 `voicebot.example.json`。

### `internal/logging`

zap wrapper。业务代码应使用 `logging.Infof/Errorf/Warnf/Debugf` 等封装，不直接使用标准库 `log`。
