# 配置管理

## 目标

- 提供统一的配置文件，集中管理日志、ASR、TTS、LLM、音频与工具配置
- 保持与现有默认值与环境变量兼容
- 明确加载顺序与覆盖规则，避免隐藏行为

## 配置文件格式与位置

- 格式: JSON（不引入额外依赖）
- 默认路径: `config/voicebot.json`
- 通过 `-config` 参数覆盖默认路径
- 示例配置: `config/voicebot.example.json`

## 加载顺序

1. 代码默认值（由各模块 `Default*Config` 提供）
2. 配置文件（JSON）
3. 环境变量（覆盖关键字段）

## 环境变量

| 环境变量 | 说明 | 覆盖配置项 |
|---------|------|-----------|
| `LOG_LEVEL` | 日志级别 | `logging.level` |
| `LOG_FORMAT` | 日志格式 | `logging.format` |
| `DASHSCOPE_API_KEY` | 阿里云 API Key | `asr.api_key`, `tts.api_key` |
| `ZHIPU_API_KEY` | 智谱 AI API Key | `llm.api_key` |

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
    "endpoint": "wss://dashscope.aliyuncs.com/api-ws/v1/inference"
  },
  "tts": {
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
  },
  "llm": {
    "api_key": "",
    "base_url": "https://open.bigmodel.cn/api/coding/paas/v4",
    "model": "glm-4-flash"
  },
  "audio": {
    "mixer": {
      "tts_volume": 1.0,
      "resource_volume": 1.0,
      "sample_rate": 16000,
      "channels": 2
    },
    "tts_pipeline": {
      "max_tts_buffer": 3,
      "max_concurrent_tts": 2,
      "text_queue_size": 100
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
      "search": "query",
      "playMusic": "action",
      "setVolume": "action",
      "pauseMusic": "action"
    },
    "action_responses": {
      "playMusic": "正在为您播放{{song}}",
      "setVolume": "已将音量设置为{{level}}",
      "pauseMusic": "音乐已暂停"
    }
  }
}
```

## 配置项说明

### logging

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| level | string | info | 日志级别：debug, info, warn, error |
| format | string | console | 日志格式：console, json |

### asr

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| api_key | string | - | 阿里云 Dashscope API Key |
| model | string | fun-asr-realtime | ASR 模型 |
| endpoint | string | - | WebSocket 端点 |

### tts

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
| types | object | 工具类型映射（query/action） |
| action_responses | object | action 类工具的预设响应模板 |

## 校验规则

- LLM 的 `api_key` 不能为空（或由 `ZHIPU_API_KEY` 覆盖）
- ASR/TTS 的 `api_key` 不能为空（或由 `DASHSCOPE_API_KEY` 覆盖）
- `audio.in_pipe.sample_rate` 与 `tts.sample_rate` 必须是正数
- `tools.types` 仅接受 `query` 或 `action`

## 相关文档

- [快速开始](/guide/getting-started) - 环境配置和运行
- [工具开发](/guide/development) - 工具开发指南
