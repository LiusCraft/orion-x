# 配置管理设计

## 目标

- 提供统一的配置文件，集中管理日志、ASR、TTS、LLM、音频与工具配置。
- 保持与现有默认值与环境变量兼容。
- 明确加载顺序与覆盖规则，避免隐藏行为。

## 配置文件格式与位置

- 格式: JSON（不引入额外依赖）
- 默认路径: `data/voicebot.json`
- 通过 `-config` 参数覆盖默认路径
- 示例配置: `voicebot.example.json`

> `cmd/ws-server` 使用独立配置：默认路径 `data/ws-server.json`，示例 `ws-server.example.json`。

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
      "playMusic": "action",
      "mcp.demo.get_device_status": "query"
    },
    "action_responses": {
      "playMusic": "正在为您播放{{song}}",
      "setVolume": "已将音量设置为{{level}}",
      "mcp.demo.play_music": "开始播放: {{song}}"
    },
    "mcp": [
      {
        "id": "demo",
        "transport": "sse",
        "endpoint": "http://localhost:12345/mcp/sse",
        "tool_name_list": ["get_device_status"],
        "timeout_ms": 30000
      }
    ]
  },
  "metrics": {
    "enabled": true,
    "address": "127.0.0.1:9100",
    "path": "/metrics",
    "enable_open_metrics": true,
    "max_requests_in_flight": 5,
    "bearer_token": ""
  }
}
```

## 校验规则

- LLM 的 `api_key` 不能为空（或由 `ZHIPU_API_KEY` 覆盖）。
- ASR/TTS 的 `api_key` 不能为空（或由 `DASHSCOPE_API_KEY` 覆盖）。
- `audio.in_pipe.sample_rate` 与 `tts.sample_rate` 必须是正数。
- `audio.in_pipe.sample_rate` 同时用于 ASR 请求采样率。
- `tools.types` 仅接受 `query` 或 `action`。
- `tools.mcp.transport` 仅接受 `stdio` / `sse` / `streamable`，`stdio` 必须提供 `command`，其余必须提供 `endpoint`。
- `metrics.enabled=true` 时必须设置 `metrics.address`（host:port）与非空 `metrics.path`。

## 行为说明

- 未设置的字段将使用默认值，保持当前运行行为。
- 同名环境变量会覆盖配置文件值，便于部署时注入密钥。
- `tools.action_responses` 支持 `{{key}}` 形式的简单模板替换。
- `tools.mcp` 支持 `stdio` / `sse` / `streamable` 三种连接方式，工具会以 `mcp.<id>.<tool>` 作为统一名称前缀。
  - **重要**：配置 MCP 工具的类型和动作响应时，必须使用完整前缀名称（如 `mcp.demo.get_device_status`），不能使用短名称。
- **本地工具**：使用短名称配置（如 `getTime`、`getWeather`）。
- **MCP 工具**：使用完整前缀名称配置（如 `mcp.demo.get_device_status`）。
- 未配置的工具类型默认为 `query`。
- Metrics 默认独立端口暴露 `/metrics`，可通过 `bearer_token` 简单鉴权。

## ws-server 独立配置

`cmd/ws-server` 不再直接复用 `voicebot.json`，而是使用独立配置模型：

- `logging` / `server` / `metrics`：服务端运行级配置
- `voicebot`：会话级配置池（ASR/TTS/LLM/Audio/Tools/Memory）

核心字段：

- `voicebot.default`：默认会话配置（可选）
- `voicebot.profiles`：按 profile id 管理多个 voicebot 配置
- `voicebot.local_bindings`：`device-id -> profile-id` 本地绑定

解析规则（当前实现）：

1. 命中 `voicebot.local_bindings[device-id]`，使用对应 profile
2. 未命中时，若 `voicebot.default` 存在则使用 default
3. 若 `voicebot.default` 为 `null`（关闭默认配置），则该设备视为未开通，需要绑定后才能接入

说明：

- `voicebot.default` 设置为 `null` 时，服务端不会兜底，未绑定设备会被拒绝。
- manager 服务（设备与 voicebot 关联管理）尚未实现，当前以本地绑定为准。
