# WS 实时 STT 回传设计（partial/final）

## 背景

当前 ws-server 仅在 ASR `isFinal=true` 时下发 `stt`，客户端无法在用户讲话过程中展示实时识别结果，导致反馈滞后。

## 目标

1. 在不新增消息类型的前提下复用 `stt` 消息承载实时中间结果。
2. 通过 `hello.features` 能力协商实现默认兼容（默认仅 final）。
3. 保持 final 主链路不变：`send stt(final)` + `Orchestrator.OnASRFinal`。

## 非目标

- 不调整 ASR 模型/引擎配置。
- 不引入新的 websocket 消息类型。
- 不涉及客户端 UI 视觉方案。

## 协议扩展

### 1) hello.features capability

客户端可在 `hello.features` 中声明：

```json
{
  "type": "hello",
  "features": {
    "stt": { "interim": true }
  }
}
```

兼容别名：`features.interim_stt=true`。

协商结果：
- `true`：服务端允许发送 `stt.state=partial`。
- 缺省或 `false`：仅发送 `stt.state=final`。

### 2) stt 消息 state 字段

`stt` 增加 `state` 字段：

```json
{
  "type": "stt",
  "state": "partial",
  "text": "...",
  "session_id": "..."
}
```

取值：
- `partial`：实时中间识别结果
- `final`：最终识别结果

## 服务端处理流程

1. 握手阶段解析 `hello.features`，设置会话级 `interimSTT` 开关。
2. `AudioInPipe.OnASRResult(text, isFinal)`：
   - `isFinal=true`：
     - 重置 partial 去重/节流状态
     - 下发 `stt(state=final)`
     - 调用 `Orchestrator.OnASRFinal(text)`
   - `isFinal=false`：
     - 仍触发 `Orchestrator.OnUserSpeakingDetected()`
     - 仅在 `interimSTT=true` 且满足去重/节流时下发 `stt(state=partial)`

## 去重与节流策略

- 去重：相同 partial 文本不重复发送。
- 节流：最小发送间隔 `200ms`（仅对 partial 生效）。
- final 不受节流影响，始终发送。

## 兼容性

- 旧客户端不声明 capability 时行为不变（只收 final）。
- 新增字段为向后兼容扩展：
  - `hello.features` 为可选
  - `stt.state` 为可选字段（服务端会带上）

## 测试点

1. capability 解析：`features.stt.interim`、`features.interim_stt`、缺省/非法值。
2. partial 发送策略：首条发送、重复文本去重、节流抑制、间隔后放行。
3. final 路径：始终发送 `state=final`，并触发 `OnASRFinal`。
4. final 后重置：允许下一轮相同文本 partial 再次发送。
