# 配置管理设计

## 目标

- 提供统一的配置文件，集中管理日志、ASR、TTS、LLM、音频与工具配置。
- 保持与现有默认值与环境变量兼容。
- 明确加载顺序与覆盖规则，避免隐藏行为。

## 配置文件格式与位置

- 格式: JSON（不引入额外依赖）
- 默认路径: `config/voicebot.json`
- 通过 `-config` 参数覆盖默认路径
- 示例配置: `config/voicebot.example.json`

## 加载顺序

1. 代码默认值（由各模块 `Default*Config` 提供）
2. 配置文件（JSON）
3. 环境变量（覆盖关键字段）

环境变量覆盖项：

- `LOG_LEVEL`, `LOG_FORMAT`
- `DASHSCOPE_API_KEY`（ASR/TTS）
- `ZHIPU_API_KEY`（LLM，优先于配置文件）

## 配置结构

```json
{
  "logging": {
    "level": "info",
    "format": "console"
  },
  "asr": {
    "api_key": "",
    "model": "fun-asr-realtime",
    "endpoint": ""
  },
  "tts": {
    "api_key": "",
    "endpoint": "",
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
    "enable_data_inspection": true
  },
  "llm": {
    "api_key": "",
    "base_url": "https://open.bigmodel.cn/api/coding/paas/v4",
    "model": "glm-4-flash"
  },
  "audio": {
    "mixer": {
      "tts_volume": 1.0,
      "resource_volume": 1.0
    },
    "in_pipe": {
      "sample_rate": 16000,
      "channels": 1,
      "enable_vad": true,
      "vad_threshold": 0.5
    }
  },
  "tools": {
    "types": {
      "getTime": "query",
      "getWeather": "query",
      "playMusic": "action"
    },
    "action_responses": {
      "playMusic": "正在为您播放{{song}}",
      "setVolume": "已将音量设置为{{level}}"
    }
  }
}
```

## 校验规则

- LLM 的 `api_key` 不能为空（或由 `ZHIPU_API_KEY` 覆盖）。
- ASR/TTS 的 `api_key` 不能为空（或由 `DASHSCOPE_API_KEY` 覆盖）。
- `audio.in_pipe.sample_rate` 与 `tts.sample_rate` 必须是正数。
- `audio.in_pipe.sample_rate` 同时用于 ASR 请求采样率。
- `tools.types` 仅接受 `query` 或 `action`。

## 行为说明

- 未设置的字段将使用默认值，保持当前运行行为。
- 同名环境变量会覆盖配置文件值，便于部署时注入密钥。
- `tools.action_responses` 支持 `{{key}}` 形式的简单模板替换。
