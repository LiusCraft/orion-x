# Prometheus Metrics 设计

## 目标

- 为 WS Server 与语音链路提供统一的 Prometheus 指标。
- 独立端口暴露 `/metrics`，避免影响 WebSocket 服务。
- 避免高基数标签，默认不包含 `session_id/device_id`。

## 注册器

- 使用 `internal/metrics.NewRegistry()` 创建独立 registry。
- 注册 Go runtime、process、build info 标准指标。

## 指标命名

- `namespace=orion`，按子系统区分：`ws / llm / asr / tts / tool / turn`。

## 关键指标

### WS 连接与会话
- `orion_ws_connections_total{result}`
- `orion_ws_active_sessions`
- `orion_ws_handshake_duration_seconds{result}`
- `orion_ws_session_duration_seconds`

### 消息与流量
- `orion_ws_messages_in_total{type}`
- `orion_ws_messages_out_total{type}`
- `orion_ws_audio_in_bytes_total`
- `orion_ws_audio_out_bytes_total`
- `orion_ws_read_errors_total{kind}`
- `orion_ws_write_errors_total{kind}`
- `orion_ws_write_queue_dropped_total`

### LLM / ASR / TTS / Tools
- `orion_llm_requests_total{result}`
- `orion_llm_latency_seconds`
- `orion_llm_time_to_first_token_seconds`
- `orion_asr_results_total{final}`
- `orion_asr_audio_bytes_total`
- `orion_asr_errors_total{stage}`
- `orion_tts_requests_total{result}`
- `orion_tts_sentences_started_total`
- `orion_tts_sentences_finished_total`
- `orion_tts_interrupts_total`
- `orion_tts_queue_size`
- `orion_tts_buffer_size`
- `orion_tts_is_playing`
- `orion_tool_calls_total{tool,result}`
- `orion_tool_latency_seconds{tool}`
- `orion_turn_asr_to_tts_start_seconds`

## 暴露方式

- 配置 `metrics.address` 与 `metrics.path`。
- `bearer_token` 非空时要求 `Authorization: Bearer <token>`。
