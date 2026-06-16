# 配置说明

Orion-X 使用 JSON 配置。默认路径是 `data/voicebot.json`，可以通过 `-config` 参数覆盖。

```bash
./bin/voicebot -config data/voicebot.json
```

示例配置文件在项目根目录：

```bash
voicebot.example.json
```

## 加载顺序

1. 代码默认值
2. JSON 配置文件
3. 环境变量覆盖
4. 启动时校验 API key

如果默认配置文件不存在，程序仍会使用代码默认值和环境变量，但 `cmd/voicebot` 启动时要求 ASR、TTS、LLM key 都存在。

## 环境变量

| 环境变量 | 覆盖项 |
|---|---|
| `LOG_LEVEL` | `logging.level` |
| `LOG_FORMAT` | `logging.format` |
| `DASHSCOPE_API_KEY` | `provider.asr.aliyun.api_key`、`provider.tts.aliyun.api_key`，并在 LLM key 为空时填充 `provider.llm.openai.api_key` |
| `ZHIPU_API_KEY` | `provider.llm.openai.api_key` |

## Provider

当前 provider 类型固定为：

| 配置项 | 当前支持值 |
|---|---|
| `provider.asr.type` | `aliyun` |
| `provider.tts.type` | `aliyun` |
| `provider.llm.type` | `openai` |

`openai` 表示 OpenAI-compatible Chat API，不限定必须访问 OpenAI 官方服务。默认 LLM base URL 指向智谱兼容接口。

## 音频输入

```json
{
  "audio": {
    "in_pipe": {
      "sample_rate": 16000,
      "channels": 1,
      "enable_vad": true,
      "vad_threshold": 0.5,
      "vad_type": "silero",
      "vad_model_path": "models/silero_vad.onnx",
      "vad_min_silence_ms": 500,
      "vad_speech_pad_ms": 300,
      "buffer_size": 3200,
      "high_latency": false,
      "input_device": ""
    }
  }
}
```

说明：

- `buffer_size` 默认 3200 samples，约等于 16 kHz 下 200 ms
- `high_latency` 适合蓝牙等高延迟输入设备
- `input_device` 为空时，启动时会尝试选择默认输入设备
- 当前 VAD 类型只支持 `silero`

## TTS 输出

`provider.tts.aliyun` 配置 DashScope TTS 参数：

```json
{
  "model": "cosyvoice-v3-flash",
  "voice": "longanyang",
  "format": "pcm",
  "sample_rate": 16000,
  "volume": 50,
  "rate": 1.0,
  "pitch": 1.0,
  "text_type": "PlainText",
  "enable_ssml": false,
  "enable_data_inspection": true,
  "voice_map": {
    "happy": "longanyang",
    "sad": "zhichu",
    "angry": "zhimeng",
    "calm": "longxiaochun",
    "excited": "longanyang",
    "default": "longanyang"
  }
}
```

`audio.tts_pipeline` 控制异步 TTS 队列：

| 字段 | 默认值 | 说明 |
|---|---:|---|
| `max_tts_buffer` | 3 | 已生成音频缓冲数量 |
| `max_concurrent_tts` | 2 | 并发 TTS 生成数 |
| `text_queue_size` | 100 | 文本队列容量 |

`audio.tts_scheduler` 当前保留为调度配置：

| 字段 | 默认值 | 校验 |
|---|---:|---|
| `max_in_flight_sentences` | 2 | 必须大于 0 |
| `max_cache_sentences` | 0 | 必须大于等于 0 |

## MCP 工具

工具配置位于 `tools.mcp`。每个 server 需要唯一 `id`。

```json
{
  "tools": {
    "mcp": [
      {
        "id": "example",
        "transport": "stdio",
        "command": "node",
        "args": ["server.js"],
        "tool_name_list": ["search"],
        "timeout_ms": 10000
      }
    ]
  }
}
```

支持的 transport：

| transport | 必填字段 |
|---|---|
| `stdio` | `command` |
| `sse` | `endpoint` |
| `streamable` | `endpoint` |

`tool_name_list` 为空时加载 server 暴露的全部工具。

`voicebot.example.json` 中如果仍看到 `tools.types` 或 `tools.action_responses`，它们是旧版工具设计残留字段；当前 `internal/config` 不读取这些字段，实际生效的是 `tools.mcp` 和代码里的本地 `LocalSpecs()`。

## 记忆

```json
{
  "memory": {
    "mode": "session",
    "session_max_turns": 10,
    "session_summary_every_n": 20,
    "long_term_db_path": "data/memory.db",
    "long_term_max_results": 6,
    "retention_days": 365,
    "fts_min_score": 0
  }
}
```

| mode | 说明 |
|---|---|
| `none` | 不启用 memory service 上下文 |
| `session` | 维护会话短期记忆和摘要 |
| `long_term` | 使用 SQLite 长期记忆，要求 `long_term_db_path` 非空 |

## 校验规则

启动前会校验：

- ASR、TTS、LLM provider type 必须是当前支持值
- `provider.*.api_key` 在 `cmd/voicebot` 中必须存在
- ASR、TTS、LLM model 不能为空
- `audio.in_pipe.sample_rate` 和 `provider.tts.aliyun.sample_rate` 必须大于 0
- VAD 启用时 `audio.in_pipe.vad_type` 必须为空或 `silero`
- MCP `id` 不能重复
- `stdio` MCP server 必须有 `command`
- `sse`、`streamable` MCP server 必须有 `endpoint`
- memory mode 必须是 `none`、`session` 或 `long_term`
