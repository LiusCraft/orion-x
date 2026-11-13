# Realtime GA 服务

`cmd/realtime` 提供独立部署的 `/v1/realtime` WebSocket 服务。服务端复用现有的 ASR、Agent 和 TTS 流水线，使用 JSON 事件和 Base64 PCM 音频，不依赖 OpenAI Realtime 上游连接。

## 启动

```bash
go run ./cmd/realtime -config data/wsserver.yaml -addr :8090
```

`device_id` 通过查询参数或 `X-Device-ID` 请求头传入。配置存在于 Manager 时，服务按设备读取 ASR、LLM、TTS 和工具配置。启用数据面鉴权后，客户端需要同时提供 `device_id` 和 `Authorization: Bearer ...`。

## 当前事件集

客户端事件包括 `session.update`、`input_audio_buffer.append`、`input_audio_buffer.commit`、`input_audio_buffer.clear`、`conversation.item.create`、`response.create` 和 `response.cancel`。当前 `input_audio_buffer.clear` 只接受空闲缓冲区，活动语音段需要先完成 commit 或结束会话。

服务端事件包括 `session.created`、`session.updated`、输入音频缓冲区状态、`conversation.item.created`、响应文本和音频增量、音频转写增量、各输出完成事件、`response.done` 和 `error`。

输入音频使用单声道 PCM16。默认输入采样率为 24 kHz，服务端会转换到现有 ASR 的 16 kHz。输出音频为 24 kHz PCM16，并通过 `response.output_audio.delta` 返回 Base64 数据。

第一版的 `conversation.item.create` 支持 `input_text`。文本响应按 GA 的 `conversation.item.create` 加 `response.create` 流程触发。音频输入在 ASR 完成后根据 `turn_detection.create_response` 自动创建响应，也可以关闭自动响应后由客户端发送 `response.create`。

后续接入 OpenAI Realtime 上游 SDK 时，协议层和连接生命周期可以保留，只替换本地流水线后端。

