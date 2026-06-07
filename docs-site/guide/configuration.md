# 配置管理

## 目标

- 提供统一的配置文件，集中管理日志、ASR、TTS、LLM、音频与工具配置
- 保持与现有默认值与环境变量兼容
- 明确加载顺序与覆盖规则，避免隐藏行为

## 配置文件格式与位置

- 格式: JSON（不引入额外依赖）
- 默认路径: `data/voicebot.json`
- 通过 `-config` 参数覆盖默认路径
- 示例配置: `voicebot.example.json`

## 加载顺序

1. 代码默认值（由各模块 `Default*Config` 提供）
2. 配置文件（JSON）
3. 环境变量（覆盖关键字段）

## 环境变量

| 环境变量 | 说明 | 覆盖配置项 |
|---------|------|-----------|
| `LOG_LEVEL` | 日志级别 | `logging.level` |
| `LOG_FORMAT` | 日志格式 | `logging.format` |
| `DASHSCOPE_API_KEY` | 阿里云 API Key | `provider.asr.aliyun.api_key`, `provider.tts.aliyun.api_key` |
| `ZHIPU_API_KEY` | 智谱 AI API Key | `provider.llm.openai.api_key` |

## 配置结构

本节描述本地 `cmd/voicebot` 使用的 `data/voicebot.json`。它不需要配置 `server` / `metrics`；
这两个顶层字段属于 `cmd/ws-server` 的运行配置，见 `ws-server.example.json`。

```json
{
  "logging": {
    "level": "info",
    "format": "console"
  },
  "provider": {
    "asr": {
      "type": "aliyun",
      "aliyun": {
        "api_key": "",
        "model": "fun-asr-realtime",
        "endpoint": "wss://dashscope.aliyuncs.com/api-ws/v1/inference"
      }
    },
    "tts": {
      "type": "aliyun",
      "aliyun": {
        "api_key": "",
        "endpoint": "wss://dashscope.aliyuncs.com/api-ws/v1/inference",
        "workspace": "",
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
    },
    "llm": {
      "type": "openai",
      "openai": {
        "api_key": "",
        "base_url": "https://open.bigmodel.cn/api/coding/paas/v4",
        "model": "glm-4-flash"
      }
    }
  },
  "audio": {
    "mixer": {
      "tts_volume": 1.0,
      "resource_volume": 1.0,
      "sample_rate": 16000,
      "channels": 2,
      "frames_per_buffer": 1024
    },
    "tts_pipeline": {
      "max_tts_buffer": 3,
      "max_concurrent_tts": 2,
      "text_queue_size": 100
    },
    "tts_scheduler": {
      "max_in_flight_sentences": 2,
      "max_cache_sentences": 0
    },
    "in_pipe": {
      "sample_rate": 16000,
      "channels": 1,
      "enable_vad": true,
      "vad_threshold": 0.5,
      "vad_type": "silero",
      "vad_model_path": "models/silero_vad.onnx",
      "vad_min_silence_ms": 500,
      "vad_speech_pad_ms": 300
    }
  },
  "tools": {
    "mcp": []
  },
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

## 配置项说明

### logging

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| level | string | info | 日志级别：debug, info, warn, error |
| format | string | console | 日志格式：console, json |

### provider.asr.aliyun

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| api_key | string | - | 阿里云 Dashscope API Key |
| model | string | fun-asr-realtime | ASR 模型 |
| endpoint | string | - | WebSocket 端点 |

### provider.tts.aliyun

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| api_key | string | - | 阿里云 Dashscope API Key |
| model | string | cosyvoice-v3-flash | TTS 模型 |
| voice | string | longanyang | 默认音色 |
| sample_rate | int | 16000 | 采样率（支持：16000, 22050, 24000, 48000） |
| voice_map | object | - | 情绪到音色的映射表 |

### audio.mixer

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| tts_volume | float | 1.0 | TTS 音量（0.0-1.0） |
| resource_volume | float | 1.0 | 资源音频音量（0.0-1.0） |
| sample_rate | int | 16000 | 系统采样率 |
| channels | int | 2 | 输出声道数 |

### audio.tts_pipeline

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| max_tts_buffer | int | 3 | TTS 音频缓冲区最大容量 |
| max_concurrent_tts | int | 2 | 最大并发 TTS 生成数 |
| text_queue_size | int | 100 | 文本队列大小 |

### audio.in_pipe

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| sample_rate | int | 16000 | 输入采样率 |
| channels | int | 1 | 输入声道数 |
| enable_vad | bool | true | 是否启用 VAD |
| vad_threshold | float | 0.5 | VAD 阈值（0.0-1.0） |

### tools

| 字段 | 类型 | 说明 |
|------|------|------|
| mcp | array | MCP 服务器配置 |

## 校验规则

- `provider.llm.openai.api_key` 不能为空（或由 `ZHIPU_API_KEY` 覆盖）
- `provider.asr.aliyun.api_key` / `provider.tts.aliyun.api_key` 不能为空（或由 `DASHSCOPE_API_KEY` 覆盖）
- `audio.in_pipe.sample_rate` 与 `provider.tts.aliyun.sample_rate` 必须是正数
- `audio.tts_scheduler.max_in_flight_sentences` 必须是正数
- `cmd/voicebot` 本地启动不会监听 `server.address`，也不会暴露 metrics；服务端监听与 metrics 配置在 `ws-server.example.json`

## 相关文档

- [快速开始](/guide/getting-started) - 环境配置和运行
- [工具开发](/guide/development) - 工具开发指南
