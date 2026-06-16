# 日志系统

项目使用 `internal/logging` 封装 zap。业务代码应使用 `logging.Infof`、`logging.Warnf`、`logging.Errorf`、`logging.Debugf`，不要直接使用标准库 `log`。

## 配置

```json
{
  "logging": {
    "level": "info",
    "format": "console"
  }
}
```

环境变量可以覆盖配置：

```bash
LOG_LEVEL=debug LOG_FORMAT=console make run-voicebot
```

支持的值：

| 字段 | 值 |
|---|---|
| `level` | `debug`、`info`、`warn`、`error` |
| `format` | `console`、`json` |

## 启动日志

`cmd/voicebot` 启动时会按顺序记录：

```text
Config loaded successfully
Creating ToolManager...
ToolManager created successfully
Creating Agent...
Agent created successfully
Initializing PortAudio...
Creating AudioOutPipe...
Creating AudioInPipe...
Building pipeline: ASR -> Agent -> TTS...
Starting pipeline...
VoiceBot is Running!
```

工具加载会输出已注册工具数量和名称：

```text
[Tools] Total tools loaded: 1
[Tools]   - getTime
```

## Pipeline 日志

当前主链路使用 `pipeline.NewLoggingObserver(true)`。Pipeline observer 会记录 stage 启停、消息流转和错误。

关键 stage：

- `asr`
- `agent`
- `tts`

排查时可以用 `LOG_LEVEL=debug` 查看更细粒度的消息流。

## Agent 日志

Agent 会记录：

- 当前 step：`Agent: step 1/2`
- LLM stream 建立耗时
- 首个 chunk 到达耗时
- 工具执行名称
- 无工具调用时的总耗时
- LLM stream 和 tool execution 错误

工具调用示例：

```text
Agent: executing tool: getTime
[Tool] getTime 执行完成，结果: ...
Agent: step 2/2
```

## 音频日志

`AudioInPipe` 会记录：

- 启动和停止状态
- 音频源关闭
- ASR 启动、Finish、Close 过程
- VAD segmenter 创建失败时的降级警告

`AudioOutPipe` 和 `TTSStage` 会记录：

- TTS stream 开始
- TTS 写入失败
- EndTTSStream 失败
- Interrupt

## 常见排查

### 启动后马上退出

检查 API key：

- `provider.asr.aliyun.api_key`
- `provider.tts.aliyun.api_key`
- `provider.llm.openai.api_key`

`cmd/voicebot` 会调用 `ValidateKeys(true, true, true)`，三类 key 都必填。

### 没有麦克风输入

打开 debug 日志，检查 input device、buffer size 和 PortAudio 初始化日志：

```bash
LOG_LEVEL=debug make run-voicebot
```

配置项：

- `audio.in_pipe.input_device`
- `audio.in_pipe.high_latency`
- `audio.in_pipe.buffer_size`

### VAD 不工作

确认模型路径存在：

```text
models/silero_vad.onnx
```

如果模型加载失败，日志会警告并禁用 VAD。

### MCP 工具未加载

检查：

- `tools.mcp[].id` 是否唯一
- stdio server 是否有 `command`
- sse/streamable server 是否有 `endpoint`
- `tool_name_list` 是否过滤掉了目标工具
- `timeout_ms` 是否过短
