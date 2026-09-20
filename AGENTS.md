# AGENTS.md

## Build & run

```bash
# Build (macOS needs ONNX Runtime + libopus via Homebrew: `brew install opus`)
make build

# Run the CLI harness (local mic/speaker)
make run-voicebot

# Run the WebSocket server (cmd/wsserver, one session per connection)
make run-wsserver          # listens on :8080/ws by default; see -addr/-path flags

# Build manually
GOTOOLCHAIN=$(go env GOTOOLCHAIN) CGO_CFLAGS="-I$(brew --prefix)/include/onnxruntime" CGO_LDFLAGS="-L$(brew --prefix)/lib" go build -o bin/voicebot ./cmd/voicebot
```

Config file: `data/voicebot.json` (template: `voicebot.example.json`). ASR/TTS/LLM keys are required on startup. `cmd/wsserver` shares the same config file.

## Test

```bash
make test              # go test ./...
make test-audio        # go test ./internal/audio (needs audio device + ASR keys)
```

No CI for Go -- only docs deploy in `.github/workflows/deploy-docs.yml`.

**After writing code, you MUST run `golangci-lint run ./...` and fix all issues before completing.**

## Architecture

Pipeline: `ASR → Agent → TTS → <output>` built via `pipeline.NewBuilder()` (linear) or `pipeline.NewDAGBuilder()` (fan-out/fan-in, e.g. `asr` broadcasting to both `agent` and a `stt`-echoing output stage). TTS audio flows through the pipeline `Message` bus (`TTSStage` registers `TTSProcessor.OnChunk` internally and emits `audio.TTSChunk` messages) rather than a side-channel callback, so `cmd/voicebot` and `cmd/wsserver` each plug in their own output stage (`PortAudioOutputStage`, `stages.WSOutputStage`).

Two entry points:

- `cmd/voicebot/` -- CLI harness: local mic/speaker, one process-wide session.
- `cmd/wsserver/` -- WebSocket server: one independent session + DAG pipeline per connection (`asr`→`agent`→`tts`→`ws_output`, plus `asr`→`ws_output` for STT echo). Protocol (hello/listen/abort/stt/tts JSON control frames + binary audio frames) is defined in `internal/wsproto/`; it's inspired by xiaozhi-esp32-server's protocol but deliberately omits `iot`/`mcp`/`server` (device-control/remote-tool-call/ops concerns unrelated to plain voice chat). Supports `auto` (server VAD) and `manual` (client-driven `listen start/stop`, via `ASRProcessor.BeginTurn/EndTurn`) modes, negotiated at hello time and fixed for the connection's lifetime. Audio wire format defaults to Opus/16kHz when a client's hello omits `audio_params` (negotiable per-connection, PCM passthrough or Opus, `internal/audio/codec/`); TTS output is always synthesized at 16kHz server-side (== `audio.InternalSampleRate`, independent of `cmd/voicebot`'s 22050Hz) since that's a valid Opus rate.

| Package | Role |
| --- | --- |
| `internal/config/` | JSON config loader + validation |
| `internal/logging/` | Zap wrapper; use `logging.Infof/Errorf/...` (not std lib `log`) |
| `internal/agent/` | LLM agent with tool calling loop |
| `internal/pipeline/` | Linear + DAG pipeline: `Stage` interface, `Builder`/`DAGBuilder`, `Message` bus |
| `internal/audio/` | ASRProcessor (VAD+ASR), TTSProcessor (splitter+synthesis), resampler |
| `internal/audio/codec/` | PCM passthrough / Opus codecs for WebSocket wire audio |
| `internal/wsproto/` | WebSocket voice session protocol message types |
| `internal/provider/` | ASR/TTS factory + Aliyun Dashscope impls |
| `internal/llm/` | LLM types + OpenAI-compatible provider |
| `internal/tools/` | Tool registry + MCP client |
| `internal/memory/` | Session buffer + SQLite long-term memory |
| `internal/session/` | Chat session / message tracking |
| `internal/text/` | Text segmenter, markdown filter, emotion tags |
| `internal/billing/` | 计费纯领域层（item / price / money / engine / estimate / wire + port），零依赖 |
| `internal/billing/service/` | 控制面：仓储、结算事务、worker、回收；唯一 import `internal/store` 的计费包 |
| `internal/billing/client/` | 数据面：HTTP client + `Sink`（缓冲/重试/本地熔断）；唯一 import `net/http` 的计费包 |
| `internal/store/billing_*.go` | 计费 9 张表的 GORM 模型与仓储，只被 `billing/service` 使用 |

### 计费（`docs/billing-design.md`）

所有计费模式都归一为 `quantity × unit_price`，差异只落在计量点、舍入/阶梯、价格匹配范围三处。职责切得很硬，改代码前先看 §19：

- **数据面（wsserver）只上报事实**：`internal/audio` / `internal/agent` 只吐原始量（合成了多少 rune、厂商报了多少秒、各糪 token），`item_code` 的映射在 `internal/channels/billing.go`。会话开始走 `authorize` 准入，过程中本地聚合，turn/会话边界 flush。
- **控制面（manager）独占钱**：定价、价格匹配、金额计算、余额、流水、对账。用量事件先落库（append-only 事实），再由 worker 幂等派生账单。
- 领域层是零依赖包，`depguard` 规则（`.golangci.yml` 的 `billing-domain`）会拦住它 import gorm / gin / store。
- 新增一个计费项 = 一条目录 seed（`internal/billing/item.go` + `internal/store/billing_seed.go`，两边要同步）+ 一条价格行 + 一个数据面埋点，不改结算代码。

Design docs in `docs/` -- read before modifying major modules.

## Naming conventions

我们自己定义的**标识值**（资源 ID、计费项 code、枚举值、命名空间、幂等键）用 `:` 拼接分段，不用 `.` / `_` / `/`：

- 计费项：`llm:tokens:input`、`tts:characters`、`voice:clone`
- 枚举值：`voice:clone`（meter source）、`half:up`（rounding）、`usage:event`（ref type）
- 幂等键：`settle:<event_id>`、`voice:clone:<voice_id>`

不适用，保持原样：

- 数据库表名/列名（`billing_prices`、`item_code`）、Go / JSON 字段名
- 文件路径、URL 路径、环境变量、HTTP header
- 镜像第三方协议或标准的值：厂商 wire 值（`content_block_delta`、`tool_calls`）、设备协议字段（`sentence_start`）、语言码（`zh-CN`）、厂商 model id（`qwen-plus`）
- 受外部格式约束的值：LLM function name 只允许 `[a-zA-Z0-9_-]`

## Quirks

- **Mock convention**: inline mock structs in `*_test.go` files, no mock generator.
- **Provider pattern**: ASR/TTS use factory registration (`provider/asr/factory.go`, `provider/tts/factory.go`). LLM uses `llm/provider/` with blank import in `main.go`.
