# 配置管理设计

## 目标

- 提供统一的配置文件，集中管理日志、ASR、TTS、LLM、音频与工具配置。
- 保持与现有默认值与环境变量兼容。
- 明确加载顺序与覆盖规则，避免隐藏行为。

## 配置文件格式与位置

- 格式: JSON（不引入额外依赖）
- 默认路径通过 `-config` 参数指定
- 通过 `-config` 参数覆盖默认路径

## 加载顺序

1. 代码默认值（由各模块 `Default*Config` 提供）
2. 配置文件（JSON）
3. 环境变量（覆盖关键字段）

环境变量覆盖项：

- `LOG_LEVEL`, `LOG_FORMAT`
- `DASHSCOPE_API_KEY`（ASR/TTS）
- `ZHIPU_API_KEY`（LLM，优先于配置文件）

## 配置结构

本节描述本地会话使用的配置结构。

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

## 校验规则

- `provider.llm.openai.api_key` 不能为空（或由 `ZHIPU_API_KEY` 覆盖）。
- `provider.asr.aliyun.api_key` / `provider.tts.aliyun.api_key` 不能为空（或由 `DASHSCOPE_API_KEY` 覆盖）。
- `audio.in_pipe.sample_rate` 与 `provider.tts.aliyun.sample_rate` 必须是正数。
- `audio.in_pipe.sample_rate` 同时用于 ASR 请求采样率。
- `tools.mcp.transport` 仅接受 `stdio` / `sse` / `streamable`，`stdio` 必须提供 `command`，其余必须提供 `endpoint`。
- `audio.tts_scheduler.max_in_flight_sentences` 必须是正数。

## 行为说明

- 未设置的字段将使用默认值，保持当前运行行为。
- 同名环境变量会覆盖配置文件值，便于部署时注入密钥。
- ASR / TTS / LLM 只通过 `provider` 结构配置。
- `tools.mcp` 支持 `stdio` / `sse` / `streamable` 三种连接方式，工具会以 `mcp.<id>.<tool>` 作为统一名称前缀。
- 未配置的工具类型默认为 `query`。

