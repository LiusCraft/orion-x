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
| `internal/apikey/` | 访问密钥领域层：凭据格式、摘要、权限范围判定（零依赖） |
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
| `internal/billing/` | 计费纯领域层（item / price / money / engine / estimate / wire + port），零依赖；另含充值链路的领域模型与 `PaymentGateway` port（`payment.go`） |
| `internal/billing/service/` | 控制面：仓储、结算事务、worker、回收；另含充值链路的下单/回调/对账（`payment.go`）；唯一 import `internal/store` 的计费包 |
| `internal/billing/client/` | 数据面：HTTP client + `Sink`（缓冲/重试/本地熔断）；唯一 import `net/http` 的计费包 |
| `internal/billing/gateway/epay/` | 出站支付网关适配器（易支付 SDK），实现 `billing.PaymentGateway`；唯一 import 支付 SDK 的地方 |
| `internal/store/billing_*.go` | 计费与充值共 11 张表的 GORM 模型与仓储，只被 `billing/service` 使用 |

### 计费（`docs/billing-design.md`）

所有计费模式都归一为 `quantity × unit_price`，差异只落在计量点、舍入/阶梯、价格匹配范围三处。职责切得很硬，改代码前先看 §19：

- **数据面（wsserver）只上报事实**：`internal/audio` / `internal/agent` 只吐原始量（合成了多少 rune、厂商报了多少秒、各糪 token），`item_code` 的映射在 `internal/channels/billing.go`。会话开始走 `authorize` 准入，过程中本地聚合，turn/会话边界 flush。
- **控制面（manager）独占钱**：定价、价格匹配、金额计算、余额、流水、对账。用量事件先落库（append-only 事实），再由 worker 幂等派生账单。
- 领域层是零依赖包，`depguard` 规则（`.golangci.yml` 的 `billing-domain`）会拦住它 import gorm / gin / store。该规则只匹配 `internal/billing/*.go`（直接子文件），`service/` / `client/` / `gateway/` 子包不受约束。
- 新增一个计费项 = 一条目录 seed（`internal/billing/item.go` + `internal/store/billing_seed.go`，两边要同步）+ 一条价格行 + 一个数据面埋点，不改结算代码。

### 充值（`payment` 段配置，支付渠道 → 余额）

链路是 `下单(pending) → 网关收款 → 回调/对账(paid) → 入账(credited)`，不进 authorize/report/settle 那条用量链路：

- 数据面（wsserver）完全不参与充值，`internal/audio` / `internal/agent` 没有埋点。
- 支付渠道是易支付（`github.com/liuscraft/epay-sdk-go`），适配器在 `internal/billing/gateway/epay`，只实现 `billing.PaymentGateway` 四件事：下单 / 查单 / 退款 / 验回调。换渠道只换这个包。
- 入账一律调 `service.Service.Credit`（`kind = recharge`、`ref_type = order`），幂等键 `credit:order:<out_trade_no>`（`billing.RechargeIdempotencyKey`）。**回调金额只用来核对订单，入账金额永远取自订单行**。
- 回调两条路由（`POST|GET /pay/epay/notify`、`GET /pay/epay/return`）是公网匿名路由，**不能挂任何鉴权中间件**；响应体必须是纯文本 `success`。
- 充值不是计量项：不建 `billing_items` seed、不录价格行。赠款要送就走 `billing_grants`（§12）。
- 网关的 `money` 只有两位小数 → 下单金额必须是整分（`amount_micro % billing.MicroPerCent == 0`），不整分直接拒。
- `paid` 与 `credited` 分开落库；manager 里有个每分钟的 sweeper 扫 `paid` 未入账补账、扫过期未付主动查单。改动这两段时记得它们要幂等。
- 退款（`POST /api/billing/recharge/:out_trade_no/refund`，仅 admin）是**先让网关退钱、再改自己的账**：顺序反了的话，网关调用挂了而账已经退了，钱就凭空多出来。整单退，不支持部分退款（那需要一张单独退款单表），幂等键 `refund:order:<out_trade_no>`（`billing.RefundIdempotencyKey`），出账走 `service.Service.Refund`（`kind = refund`）。

### 访问密钥（`internal/apikey`，设计见 `docs/apikey-design.md`）

密钥是用户自建的**程序化调用凭据**，用来在控制台会话之外调 Manager API：

- **明文可复制，但要重验身份**：认证走 `key_hash`（SHA-256）；可复制形态是 `key_secret`（AES-256-GCM，密钥由 `jwt.secret` 经 HKDF 派生，换 secret 后旧密文解不开但密钥照旧能用）。`POST /api/apikeys/:id/reveal` 验过账号密码才解密返回，列表页只有脱敏串。
- **默认拒绝**：`cmd/manager/middleware/apikey.go` 的 `apiKeyRouteScopes` 是唯一允许 API Key 触达的路由表（agent / mcp / data 三个命名空间），表外路由（供应商密钥、模型、计费、密钥管理）只认控制台会话。新增路由不必记得加白名单。
- **密钥管不了密钥**：签发、删除、复制都只在控制台会话里做，密钥自己调不了 `/api/apikeys`，没有“旧钥换新钥”的提权路径。
- **范围语义**：`apikey:scope:all` 可写表内的全部命名空间并可用于 wsserver 接入，`apikey:scope:read` 只放行安全方法，其余取值按命名空间命中；`apikey:scope:voice` 不对应 REST，而是握手时的设备接入凭据（数据面走 `/internal/apikey/authorize` 问控制面，取不到结论就拒）。
- 认证与授权都写在 `middleware.Auth` 里（不拆两个中间件），这样“新路由忘了挂授权”不可能发生。

Design docs in `docs/` -- read before modifying major modules.

## Naming conventions

我们自己定义的**标识值**（资源 ID、计费项 code、枚举值、命名空间、幂等键）用 `:` 拼接分段，不用 `.` / `_` / `/`：

- 计费项：`llm:tokens:input`、`tts:characters`、`voice:clone`
- 枚举值：`voice:clone`（meter source）、`half:up`（rounding）、`usage:event`（ref type）
- 幂等键：`settle:<event_id>`、`voice:clone:<voice_id>`、`credit:order:<out_trade_no>`、`refund:order:<out_trade_no>`
- 权限范围：`apikey:scope:all`、`apikey:scope:read`、`apikey:scope:voice`；密钥前缀：`ox:sk:`

不适用，保持原样：

- 数据库表名/列名（`billing_prices`、`item_code`）、Go / JSON 字段名
- 文件路径、URL 路径、环境变量、HTTP header
- 镜像第三方协议或标准的值：厂商 wire 值（`content_block_delta`、`tool_calls`）、设备协议字段（`sentence_start`）、语言码（`zh-CN`）、厂商 model id（`qwen-plus`）
- 受外部格式约束的值：LLM function name 只允许 `[a-zA-Z0-9_-]`

## Quirks

- **Mock convention**: inline mock structs in `*_test.go` files, no mock generator.
- **Provider pattern**: ASR/TTS use factory registration (`provider/asr/factory.go`, `provider/tts/factory.go`). LLM uses `llm/provider/` with blank import in `main.go`.
- **前端是客户界面**（`web/manager`）：面向使用者的文案里不许出现我们自己的说法——配置项名（`payment` 段、`billing.enabled`）、表名、包名、幂等键、以及“网关 / 回调 / 流水 / 微元 / 服务端”这类实现词。只写“会发生什么”和“用户要做什么”。服务端的 `{error}` 是给我们排障用的（如 `billing: amount must be between ...`），不能原样贴到界面上——前端已经有 `userFacingError(err, fallback)` 拦这种内部前缀（`web/manager/src/lib/billing.ts`）；表单能用前端校验挡住的错（金额上下限、可选支付方式）就通过接口把参数下发下去，不要靠服务端报错来教用户。
