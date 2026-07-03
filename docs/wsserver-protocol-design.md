# WebSocket 语音会话服务端设计

`cmd/wsserver` 为语音客户端（浏览器/App/设备等）提供服务端会话：每个 WebSocket
连接对应一个独立的 `session.Session` + 独立的 DAG Pipeline 实例。协议参考了
xiaozhi-esp32-server 的设计（JSON 控制帧 + 二进制音频帧），但裁剪为纯语音对话
场景。

## 协议范围

只实现核心语音交互：`hello`（握手）/ `listen`（start/stop/detect）/
`abort`（打断）/ `stt`（识别结果回显）/ `tts`（播报状态）。**不实现**
`iot`/`mcp`/`server`：

- `iot` 是给带外设硬件用的（上报/控制物理设备），与纯语音对话无关。
- `mcp` 是"服务端反向调用客户端本地工具"，与 orion-x 现有的
  `internal/tools/`（服务端主动连接外部 MCP Server）是完全不同的方向。
- `server` 是运维层面的远程控制指令，与对话业务无关。

`type` 字段保持可扩展，未来需要再加。消息结构定义在 `internal/wsproto/`。

## 消息格式

### hello（握手，双向）

客户端请求：

```json
{
  "type": "hello",
  "device_id": "dev-1",
  "audio_params": {
    "format": "opus",
    "sample_rate": 16000,
    "channels": 1,
    "frame_duration": 60,
    "bits_per_sample": 16,
    "play_buffer_duration": 2000
  },
  "mode": "auto"
}
```

- `audio_params.format`：`"pcm"` 或 `"opus"`（**省略默认 `"opus"`**）。
- `audio_params.sample_rate`：客户端上行音频的采样率（省略默认 16000）。
- `audio_params.channels`：声道数（省略默认 1）。
- `audio_params.frame_duration`：帧长（ms），决定 codec 分帧和发送 pacing 间隔。
  支持 `10/20/40/60` 四种值；省略或 0 默认 `60`。只对 Opus 有意义（PCM 忽略）。
- `audio_params.bits_per_sample`：位深。服务端整个 pipeline（codec/resampler/ASR/TTS）
  硬编码为 PCM16LE，所以**必须为 16 或省略**（省略默认 16）。非 16 值握手失败断连。
- `audio_params.play_buffer_duration`：客户端播放缓冲区大小（ms）。值越大，服务端
  在每轮播报开始时可以安全预缓冲更多帧再切换到匀速 pacing，从而降低被感知的延迟。
  省略或 ≤0 使用服务端默认值（3 帧 × frame_duration = 180ms worth）。
- `mode`：`"auto"`（服务端 VAD 自动判断说话起止）或 `"manual"`（客户端通过
  `listen start/stop` 明确控制）。省略默认 `"auto"`。**连接生命周期内不可
  变更**——切换意味着重建整个 `ASRProcessor`，协议上只在首次 hello 生效。

服务端响应：

```json
{
  "type": "hello",
  "session_id": "sess_xxx",
  "audio_params": {
    "format": "opus",
    "sample_rate": 16000,
    "channels": 1,
    "frame_duration": 60,
    "bits_per_sample": 16
  },
  "mode": "auto"
}
```

`audio_params` 在响应里描述的是**服务端下行（TTS 输出）实际使用的参数**，
不是回显客户端请求的采样率——TTS 固定按 16000Hz 合成（对所有连接一致，
详见下文"音频编码与采样率"），客户端应按这个值配置播放器。上行方向
（客户端→服务端）沿用客户端在请求里声明的参数，服务端按需重采样，不强行
要求客户端改用某个特定采样率。

### listen（客户端→服务端）

```json
{"type": "listen", "state": "start"}
{"type": "listen", "state": "stop"}
{"type": "listen", "state": "detect", "text": "直接注入的文本"}
```

- `start`/`stop`：manual 模式下驱动 `ASRProcessor.BeginTurn`/`EndTurn`
  （启动/结束一次 recognizer task）；auto 模式下忽略（VAD 自动管理边界）。
- `detect`：跳过 ASR，把 `text` 作为一轮新的用户输入直接注入 DAG（`asr`
  节点转发 `MessageTypeData` 类型的 input 消息），同时会经由 `asr→ws_output`
  这条 fan-out 边被当作 `stt` 消息回显给客户端。

**产品语义边界**：auto 模式下新语音会通过 VAD 的 `OnSpeechStart` 自动打断
当前播报；manual 模式下 `listen start` **不会**自动打断——用户需要显式发
`abort`。这与 xiaozhi-esp32-server 的既有行为一致：manual 模式允许连续多轮
追问而不误伤正在进行的回复。

### abort（客户端→服务端）

```json
{"type": "abort"}
```

注入 `pipeline.MessageTypeInterrupt` 到 DAG 的 `asr` 源节点，沿边传播打断
`AgentStage`（取消当前 LLM 生成）和 `TTSStage`（中止当前合成/播放）。

### stt（服务端→客户端）

```json
{"type": "stt", "text": "识别到的文本", "session_id": "sess_xxx"}
```

### tts（服务端→客户端）

```json
{"type": "tts", "state": "start", "session_id": "sess_xxx"}
{"type": "tts", "state": "sentence_start", "text": "这句话", "session_id": "sess_xxx"}
{"type": "tts", "state": "sentence_end", "text": "这句话", "session_id": "sess_xxx"}
{"type": "tts", "state": "stop", "session_id": "sess_xxx"}
```

状态语义：

- `start`：每轮播报开始时发一次（每轮可能有多个句子，但只发一次 start）。
- `sentence_start`：每个句子开始时发一次，`text` 为该句原文。
- `sentence_end`：每个句子结束时发一次，`text` 为该句原文。**保证在该句所有
  音频帧发送完毕之后才到达**（由 `audioPacer` 的序号语义保证：`sentence_end`
  标记以 pacedFrame 的 sentenceEnd 字段入队，run goroutine 在处理完前面所有
  音频帧之后才回调 `onSentenceEnd`）。底层依赖阿里云 DashScope TTS 的
  `result-generated` 事件中 `output.type == "sentence-end"` 触发，要求
  `word_timestamp_enabled=true`。
- `stop`：每轮播报结束时发一次。同样由 pacer 保证在所有音频帧发送完毕之后
  才到达（turnEnd marker 入队，run goroutine 在音频帧之后回调 onTurnEnd）。

打断场景（收到 `abort` 或新语音触发的 Interrupt）会立即发一次 `stop`，不等
`Final`，因为被打断的合成不会正常走到"结束"。

### 音频发送节奏（audioPacer）

`WSOutputStage` 不会把 TTS 产出的音频"产出多快就转发多快"——上游 TTS
provider（阿里云 DashScope）是按网络 batch 推送音频的，如果原样转发，客户端
听到的是"一小段音频 + 明显停顿"反复交替（burst-then-stall），除非客户端自
己做了较大的 jitter buffer，否则会有卡顿感。

`WSOutputStage` 把 PCM 攒够连接协商的 `frame_duration`（默认为 60ms，与 Opus
的固定帧长对齐，PCM 和 Opus 共用同一套 pacing）才编码入队，由内部
`audioPacer` 按该帧长节奏匀速吐给 `SafeConn`；每轮开始的前 N 帧立即发送
（不等节奏），以降低首包延迟，之后转为匀速。N 由 `play_buffer_duration/
frame_duration` 换算（默认 3，min=1，max=100）。打断时 `audioPacer.clear()`
会丢弃队列里尚未发出的帧。

`audioPacer` 内部队列是**无界**的（普通 slice，按需增长），而不是固定容量
的 buffer——这是一个真实踩过的坑：TTS 合成速度可以远快于实际播放速度（网络
可能在一两秒内推送完十几秒时长的音频），如果队列容量有限且满了就丢帧，会在
回复较长时**丢失部分语音内容**，这比卡顿更糟。内存占用只受单轮回复能产生
多少音频数据限制，实践中即使很长的回复也不过几百 KB。

**已知限制**：`sentence_start` 和 `sentence_end` 的 `text` 字段在流式
TTS provider（阿里云 DashScope）路径下均能正常工作——`sentence_start` 由
`TTSProcessor` 的 `sentenceSplitter` 驱动（在第一个该句音频 chunk 的 `Text`
字段里附上），`sentence_end` 由 provider 层的 `result-generated` 事件驱动。

## Auto / Manual 模式

由 `audio.ASRProcessor.EnableVAD` 配置决定（`mode=="auto"` 时为
`true`）：

- **auto**：Silero VAD 自动检测语音起止，`Write()` 内部走
  `processVAD`，识别到语音开始触发 `OnSpeechStart`（转为 Interrupt），
  静音后自动 `Finish`。`listen start/stop` 在这个模式下是信息性的，不驱动
  任何 recognizer 生命周期动作。
- **manual**：不启用 VAD，`Write()` 直接 `SendAudio`。recognizer task
  的生命周期完全由显式的 `BeginTurn`/`EndTurn` 驱动（对应
  `listen start`/`stop`），复用同一条底层 WebSocket 连接（已验证阿里云
  DashScope `Recognizer.Start()` 在连接存活时只发新 `run-task`，不重建
  连接）。没有活跃轮次时收到的音频帧会被 `Write()` 静默丢弃，不会报错
  导致 `ASRStage.readFromSource` 循环提前退出。

## 音频编码与采样率

- 输入（客户端→服务端）：`internal/audio/codec` 按 hello 声明的 format
  解码（PCM 直通或 Opus），若采样率非 16kHz 则用现有
  `internal/audio/resampler`（线性插值）转换到内部标准 16kHz mono 再喂给
  ASR。
- 输出（服务端→客户端）：`cmd/wsserver` 的 TTS Provider 固定配置为
  16000Hz（与 `cmd/voicebot` 的 22050Hz 相互独立，两者分别构造各自的
  `tts.Provider` 实例；也是 `audio.InternalSampleRate`，跟内部 ASR/VAD
  标准一致）。16000Hz 是合法的 Opus 采样率，因此 opus 下行路径完全不需要
  额外重采样。
- Opus 固定帧长 60ms（`internal/audio/codec/opus.go` 的
  `opusFrameDurationMs`），编解码器内部维护缓冲区处理"不定长 PCM 流 ↔
  固定帧长 Opus 包"的对齐。

## DAG 拓扑

每连接组装 4 节点：

```
asr ──────┬──→ agent ──→ tts ──→ ws_output
          └───────────────────────↑
          （asr→ws_output：识别文本回显为 stt 消息）
```

`ws_output` 是 sink 节点（无出边），按 `(msg.Type, payload 动态类型)` 区分
处理：`string` 来自 `asr`（→ `stt` 消息），`audio.TTSChunk` 来自 `tts`
（→ `tts` 状态 + 二进制音频帧）。

## 进程级共享资源 vs 每连接资源

`agent.Agent`、`tools.Registry`、`memory.Service` 无连接级状态，跨所有
WebSocket 连接共享单一实例（`cmd/wsserver/main.go` 构造一次）。每个连接
独立持有：`session.Session`、ASR `Recognizer` + `ASRProcessor`、TTS
`Provider` + `TTSProcessor`、`WSAudioSource`、DAG `Pipeline` 实例——这些
都在连接建立时创建、连接关闭时清理（`wsConnection.close()`）。

连接的 `context.Context` 用 `memory.WithContext` 标记了该连接自己的
`UserID`/`SessionID`（取自 hello 的 `device_id`，缺省时退化为
`session_id`），确保长期记忆按连接隔离，不会像 `cmd/voicebot` 那样所有
会话共享硬编码的 `"local"` 标识。

## 优雅关闭

`Server` 持有一个 `rootCtx`（所有连接的 ctx 都从它派生）和一个
`sync.WaitGroup`（追踪活跃连接）。收到 `SIGINT`/`SIGTERM` 时：
`http.Server.Shutdown` 停止接受新连接 → `Server.Shutdown(timeout)` cancel
`rootCtx`（每个连接一个专门的 goroutine 监听 `ctx.Done()` 并主动关闭底层
WS 连接——单纯 cancel context 并不能打断 `gorilla/websocket` 阻塞中的
`ReadMessage`，必须显式 `Close()`）→ 等待（带超时）所有连接的清理 goroutine
真正退出。这个机制经过 8 连接并发的手动验证：`SIGINT` 发送时连接仍在空闲
等待，服务端主动关闭了全部 8 个连接且没有超时。

macOS 上，任何加载过 Silero VAD（`mode=auto` 的连接）的进程在 `main()`
正常返回时会因为 ONNX Runtime 的 C++ 静态析构顺序问题崩溃
（`mutex lock failed: Invalid argument` / `SIGABRT`）——这在端到端并发测试
中被实际触发过。与 `cmd/voicebot/main.go` 处理 PortAudio 崩溃的方式相同，
`cmd/wsserver/main.go` 在收尾时用 `syscall.Syscall(syscall.SYS_EXIT,...)`
跳过正常的 C/C++ 库清理流程。

## 已知限制

- `internal/logging` 的 `trace_id`/`turn_id` 是进程级全局状态，多连接并发
  下会互相覆盖。`cmd/wsserver` 未改动 logging 包（影响面过大），日志改为
  手动带 `wsserver[<session_id>]` 前缀区分连接，但 `trace_id`/`turn_id`
  字段本身仍不可靠。

- `bits_per_sample` 校验失败静默断连，不新增协议层错误响应消息（不增加协议
  复杂度）。

- `frame_duration` 只支持 `{10,20,40,60}` 四个整数毫秒值，不支持 Opus 官方
  的 2.5/5ms（这些值在语音场景下包头开销过高，且 2.5 是非整数，与 JSON 整数
  协议字段冲突）。客户端声明不支持的值会被 codec 层拒绝，握手失败断连。

- `sentence_start`（我们自己的 `sentenceSplitter` 按标点切句触发）和
  `sentence_end`（DashScope provider 的 `result-generated` 事件触发）理论上
  可能不完全一一对应——比如"弱边界"（逗号）触发的文本片段传给 provider 后，
  provider 可能缓存等待更多文本才合成，导致 `sentence-begin`/`sentence-end`
  数量少于 `sentence_start` 次数。客户端不应做严格配对假设。

- `sentence_end` 的正确解析依赖 `word_timestamp_enabled: true`（已硬编码在
  `sendRunTask` 的参数中），如果未来 DashScope API 变更导致该参数失效，需要
  相应调整 provider 层的检测逻辑。
