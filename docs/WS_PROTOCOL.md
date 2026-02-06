# 小智 WebSocket 协议规范（面向 test_page.html）

本文档描述 `/Users/liushunshun/workspace/coding/qbox/xrobotd/main/xiaozhi-server/test/test_page.html` 与小智 WebSocket 服务端之间的协议细节，便于在其他项目中实现兼容的 WebSocket Server。

> 适用范围：`/xiaozhi/v1/` WebSocket 接口（JSON + 二进制音频）

---

## 1. 术语与规范性说明

- **MUST / 必须**：强制要求，否则测试页无法正常工作。
- **SHOULD / 建议**：推荐实现，以提升兼容性或稳定性。
- **MAY / 可选**：按需实现。

---

## 2. 协议概述

- 传输层：WebSocket
- 消息类型：
  - **文本消息**：UTF-8 JSON
  - **二进制消息**：原始 Opus 帧（或 PCM）
- URL 路径：`/xiaozhi/v1/`

---

## 3. 连接与握手

### 3.1 URL 与必需参数

测试页连接时会在 URL query 里追加：

- `device-id`（必需）
- `client-id`（必需）

示例：

```
ws://127.0.0.1:8000/xiaozhi/v1/?device-id=AA:BB:CC:DD:EE:FF&client-id=web_test_client
```

服务端行为：

- 若 Header 中没有 `device-id`，必须从 URL query 读取。
- 若仍无法获取 `device-id`，服务端会向客户端发送文本：`参数错误!请检查header以及body参数`，随后断开连接。

### 3.2 session_id 生成

- 若 Header 中存在 `X-Reqid` / `x-reqid`，服务端将其作为 `session_id`。
- 否则服务端生成 UUID。

### 3.3 认证

服务端可启用认证（配置项 `server.auth.enabled`）：

- **支持方式**：`Authorization: Bearer <token>` 或设备白名单 `allowed_devices`
- **浏览器限制**：WebSocket 无法添加自定义 Header，测试页无法发送 `Authorization`。

**兼容建议：**
- 若使用测试页，建议关闭认证或将 `device-id` 加入白名单。

> 注意：测试页在 URL 中追加的 `token/device_id/device_mac` 不会被认证逻辑使用。

### 3.4 握手流程

1. WebSocket 建立后，服务端立即发送 `hello`。
2. 客户端发送 `hello`。
3. 服务端再次发送 `hello`（确认音频参数）。

---

## 4. 音频参数（audio_params）

客户端 `hello` 可携带 `audio_params`，服务端会验证并回传确认值。

| 字段 | 类型 | 允许值 | 说明 |
|---|---|---|---|
| format | string | `opus` / `pcm` | 音频格式 |
| sample_rate | int | `16000` | 采样率 |
| channels | int | `1` / `2` | 声道数 |
| frame_duration | number | `20/40/60/100` | 帧时长(ms) |
| bits_per_sample | int | `16/24/32` | 位深 |
| play_buffer_duration | int | `>=100` | 播放缓冲(ms) |

> 测试页默认：`opus / 16000 / 1ch / 60ms`。

---

## 5. 消息格式

### 5.1 JSON 消息通用字段

```json
{
  "type": "...",
  "session_id": "..." // 服务端下发时常带
}
```

### 5.2 二进制消息

- 直接发送原始 Opus 帧或 PCM
- 不包 JSON、不包容器

---

## 6. 客户端 → 服务端 消息

### 6.1 `hello`

**用途**：会话初始化、参数声明、功能支持声明

示例：
```json
{
  "type": "hello",
  "device_id": "AA:BB:CC:DD:EE:FF",
  "device_name": "Web测试设备",
  "device_mac": "AA:BB:CC:DD:EE:FF",
  "token": "your-token1",
  "features": {
    "mcp": true,
    "notify": {
      "config_updated": true
    }
  },
  "agent_params": {
    "custom_replace_prompt": {
      "user_name": "Web测试用户",
      "location": "测试环境"
    }
  },
  "audio_params": {
    "format": "opus",
    "sample_rate": 16000,
    "channels": 1,
    "frame_duration": 60,
    "bits_per_sample": 16,
    "play_buffer_duration": 300
  }
}
```

### 6.2 `listen`

**用途**：文本输入/语音控制

| 字段 | 说明 |
|---|---|
| mode | `manual` / `auto` / `realtime` |
| state | `start` / `stop` / `detect` |
| text | `state=detect` 时用于文本输入 |
| text_response | 可选，直接生成 TTS（不走 LLM） |

示例：
```json
{ "type": "listen", "mode": "manual", "state": "detect", "text": "你好" }
{ "type": "listen", "mode": "manual", "state": "start" }
{ "type": "listen", "mode": "manual", "state": "stop" }
```

### 6.3 `abort`

```json
{ "type": "abort" }
```

### 6.4 `mcp`

承载 JSON-RPC 消息：
```json
{ "type": "mcp", "payload": { ... } }
```

### 6.5 `iot`

```json
{ "type": "iot", "descriptors": [...] }
{ "type": "iot", "states": [...] }
```

### 6.6 `server`

仅在 `read_config_from_api = true` 且 secret 校验通过时有效。

```json
{ "type": "server", "action": "update_config", "content": { "secret": "..." } }
{ "type": "server", "action": "restart", "content": { "secret": "..." } }
```

---

## 7. 服务端 → 客户端 消息

### 7.1 `hello`

```json
{
  "type": "hello",
  "version": 1,
  "transport": "websocket",
  "session_id": "...",
  "audio_params": {
    "format": "opus",
    "sample_rate": 16000,
    "channels": 1,
    "frame_duration": 60
  }
}
```

### 7.2 `tts`

| state | 说明 |
|---|---|
| start | 语音开始 |
| sentence_start | 语句开始（带 text） |
| stop | 语音结束 |

示例：
```json
{ "type": "tts", "state": "start", "session_id": "..." }
{ "type": "tts", "state": "sentence_start", "text": "你好", "session_id": "..." }
{ "type": "tts", "state": "stop", "session_id": "...", "is_aborted": false }
```

> 测试页会处理 `sentence_end`，但当前服务端默认不会发送。

### 7.3 `llm`

```json
{
  "type": "llm",
  "text": "🙂",
  "emotion": "happy",
  "action": "happy",
  "session_id": "..."
}
```

### 7.4 `stt`

```json
{
  "type": "stt",
  "text": "识别结果",
  "session_id": "...",
  "error_code": 10001
}
```

支持错误码（兼容字段）：
- `10001` 配置未找到
- `10002` 绑定码格式错误
- `10003` 设备未绑定
- `10004` 超出每日限额

### 7.5 `mcp`

```json
{ "type": "mcp", "payload": { ... } }
```

### 7.6 `notify`

```json
{ "type": "notify", "event": "config_updated", "agent_id": "..." }
```

### 7.7 `iot`

```json
{ "type": "iot", "commands": [ { "name": "灯", "method": "on", "parameters": {"power": true} } ] }
```

### 7.8 `server`

```json
{ "type": "server", "status": "success", "message": "配置更新成功", "content": { "action": "update_config" } }
```

---

## 8. 音频二进制流

### 8.1 客户端 → 服务端

- 发送 **原始 Opus 帧**（Uint8Array/ArrayBuffer）。
- 帧大小：16000Hz 下 60ms = 960 采样点。
- 录音结束时：
  - 发送一个 **空帧**（长度 0）
  - 发送 `listen stop`

### 8.2 服务端 → 客户端

- 发送 **原始 Opus 帧**
- 测试页会缓冲若干帧后播放

---

## 9. MCP 子协议（JSON-RPC）

### 9.1 服务端 → 客户端

#### initialize
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "capabilities": {
      "roots": { "listChanged": true },
      "sampling": {},
      "vision": { "url": "...", "token": "..." }
    },
    "clientInfo": { "name": "XiaozhiClient", "version": "1.0.0" }
  }
}
```

#### tools/list
```json
{ "jsonrpc": "2.0", "id": 2, "method": "tools/list" }
```

#### tools/call
```json
{
  "jsonrpc": "2.0",
  "id": 123,
  "method": "tools/call",
  "params": {
    "name": "self.get_device_status",
    "arguments": {}
  }
}
```

### 9.2 客户端 → 服务端

#### tools/list 回包
```json
{
  "type": "mcp",
  "payload": {
    "jsonrpc": "2.0",
    "id": 2,
    "result": {
      "tools": [
        {
          "name": "self.get_device_status",
          "description": "...",
          "inputSchema": { "type": "object", "properties": {} }
        }
      ]
    }
  }
}
```

#### tools/call 回包
```json
{
  "type": "mcp",
  "payload": {
    "jsonrpc": "2.0",
    "id": 123,
    "result": {
      "content": [ { "type": "text", "text": "true" } ],
      "isError": false
    }
  }
}
```

---

## 10. 时序示例

### 10.1 文本会话
```text
WS Connect
S -> C: hello
C -> S: hello
S -> C: hello
C -> S: listen detect(text)
S -> C: stt / llm / tts
S -> C: binary opus frames
```

### 10.2 语音会话
```text
C -> S: listen start
C -> S: binary opus frames...
C -> S: empty opus frame
C -> S: listen stop
S -> C: stt / llm / tts
S -> C: binary opus frames
```

---

## 11. 兼容性提示

- 测试页包含 base64 JSON 音频兜底发送，但服务端并不处理该格式。
- 测试页会处理 `tts.state = sentence_end`，当前服务端通常不会发送该状态。
- 测试页在连接前会执行 OTA HTTP 检查，此不属于 WS 协议范围。

---

## 12. 参考实现位置

- `/Users/liushunshun/workspace/coding/qbox/xrobotd/main/xiaozhi-server/core/connection.py`
- `/Users/liushunshun/workspace/coding/qbox/xrobotd/main/xiaozhi-server/core/handle/textHandle.py`
- `/Users/liushunshun/workspace/coding/qbox/xrobotd/main/xiaozhi-server/core/handle/helloHandle.py`
- `/Users/liushunshun/workspace/coding/qbox/xrobotd/main/xiaozhi-server/core/handle/sendAudioHandle.py`
- `/Users/liushunshun/workspace/coding/qbox/xrobotd/main/xiaozhi-server/core/handle/mcpHandle.py`
- `/Users/liushunshun/workspace/coding/qbox/xrobotd/main/xiaozhi-server/core/config_update_listener.py`
- `/Users/liushunshun/workspace/coding/qbox/xrobotd/main/xiaozhi-server/config.yaml`
- `/Users/liushunshun/workspace/coding/qbox/xrobotd/main/xiaozhi-server/test/test_page.html`

