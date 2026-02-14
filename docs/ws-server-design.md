# WebSocket Server 设计文档

## 目标

基于 `docs/WS_PROTOCOL.md` 实现 `/xiaozhi/v1/` WebSocket 服务端，支持测试页协议（hello/listen/abort + JSON + 二进制音频）。

## 协议流程

```
WS Connect
S -> C: hello (默认 audio_params)
C -> S: hello (可选携带 audio_params)
S -> C: hello (确认后的 audio_params)
```

## 消息映射

- `listen detect(text)` → 发送 `stt` → 触发 Orchestrator.OnASRFinal
- `listen start` → 启动 ASR
- `binary audio` → 解码（opus/pcm）→ AudioInPipe.SendAudio
- `listen stop / empty frame` → AudioInPipe.Stop
- `abort` → AudioOutPipe.Interrupt + tts stop(is_aborted)

## 音频流

### 输入
- 支持 `opus/pcm`
- Opus 解码为 PCM16 → ASR
- PCM 仅支持 16-bit little-endian

### 输出
- TTS 输出 PCM16 → AudioMixer → WebSocketSink
- WebSocketSink 按帧输出：
  - `format=opus`: PCM16 → Opus packet
  - `format=pcm`: 直接发送 PCM16 bytes

## Session 生命周期

1. 建立连接并完成 hello 交换
2. 基于 `device-id` 解析会话级 `voicebot` 配置
3. 初始化音频管线（Mixer + WebSocketSink + AudioOutPipe）
3. 监听 JSON / 二进制消息
4. 断开连接后释放：
   - Orchestrator.Stop()
   - AudioInPipe.Stop()
   - Mixer.Stop()

## 配置分层

- 服务端配置（`server`/`metrics`/`logging`）只负责 WS 服务运行。
- 会话配置放在 `voicebot` 下，包含 ASR/TTS/LLM/Audio/Tools/Memory。

会话配置解析规则：

1. 先查 `voicebot.local_bindings[device-id]`
2. 未命中则尝试 `voicebot.default`
3. 若 `voicebot.default = null`，设备未绑定则拒绝连接（未开通）

> manager 服务（设备绑定查询）当前未接入，后续作为扩展。

## 依赖

- WebSocket: `github.com/gorilla/websocket`
- Opus 编解码: `github.com/hraban/opus`
  - 需要系统安装 `libopus`

## 监控

- 独立端口暴露 Prometheus `/metrics`。
- 指标详见 `docs/metrics-design.md`。
