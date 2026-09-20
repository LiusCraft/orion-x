# Orion-X

**面向多设备、多通道的实时语音智能体平台。**

Orion-X 将语音识别（ASR）、大语言模型（LLM）、语音合成（TTS）、记忆、知识库和工具调用组合为可配置的智能体。它提供一个管理控制面、一个按设备动态装配会话的运行时服务，以及配套的 Web 管理控制台。

![Orion-X Manager 智能体广场](docs/images/manager-agents-plaza.png)

> 项目仍在快速迭代中。面向生产部署时，请先阅读本文的安全说明和 [WebSocket 协议](docs/wsserver-protocol-design.md)。

## 功能概览

- **设备化智能体配置**：为每个智能体配置 ASR、TTS、LLM、系统提示词、VAD、记忆和工具；一个智能体可绑定多个设备。
- **实时语音会话**：小智兼容 WebSocket 通道，支持 Opus 或 PCM 音频、服务端 VAD 自动分段、手动开始/结束录音、文本直注入和随时打断。
- **流式输出**：ASR 文本、LLM 文本与 TTS 音频均可流式返回；TTS 带播放节奏控制，减少网络突发造成的卡顿。
- **多通道接入**：内置小智 WebSocket 实时语音通道，以及按设备独立配置的 Telegram Bot 文本/语音文件通道。
- **模型与音色管理**：在控制台维护供应商、ASR/TTS/LLM 模型和音色，运行时由 Manager 组装为设备配置。
- **MCP 与设备能力**：支持 `stdio`、SSE、Streamable HTTP MCP 服务；小智客户端也可声明设备端 MCP 工具。IoT 描述、状态和命令帧可用于设备控制。
- **记忆与知识库**：持久化用户记忆和对话轮次；支持文档上传、URL 导入、切分、向量检索，并将检索能力注册为智能体工具。
- **可观测与运维入口**：Manager 和运行时均提供 `/healthz`；Manager 提供 OpenAPI/Swagger 文档。

## 系统架构

```text
                         ┌───────────────────────────────┐
                         │        Web Manager (React)     │
                         │  智能体 / 设备 / 模型 / 数据管理 │
                         └───────────────┬───────────────┘
                                         │ /api
                         ┌───────────────▼───────────────┐
                         │        Manager (Go + Gin)      │
                         │ PostgreSQL · pgvector · JWT    │
                         └───────┬───────────────┬───────┘
              设备配置 / 记忆 / 知识库            │ Telegram Bot 配置
                        │                         │
          ┌─────────────▼─────────────────────────▼─────────────┐
          │                   wsserver (Go)                       │
          │ 小智 WebSocket Channel · Telegram Channel · 健康检查   │
          └─────────────┬─────────────────────────────────────────┘
                        │ 每个连接独立的 DAG Pipeline
          ┌─────────────▼─────────────────────────────────────────┐
          │ ASR ──┬──> Agent (LLM + MCP + 记忆 + 知识检索) ─> TTS  │
          │       └──> 会话输出（STT / LLM / TTS 音频）            │
          └───────────────────────────────────────────────────────┘
```

Manager 是控制面和数据面：保存用户、智能体、设备、模型、MCP、记忆与知识库数据；`wsserver` 是运行时，不在本地固化每台设备的模型密钥，而是在连接建立时用 `device_id` 向 Manager 获取配置。这样可在控制台修改配置并让后续连接生效。

## 快速开始

### 1. 前置条件

- Go `1.26+`
- Node.js `20+` 与 npm（管理控制台）
- PostgreSQL，并启用 `pgvector` 扩展（知识库检索）
- macOS：Homebrew 安装 ONNX Runtime 与 Opus；本地语音 CLI 额外需要 PortAudio

```bash
# macOS
brew install onnxruntime opus portaudio

# 创建数据库后，在目标数据库中启用向量扩展
psql "$DB_DSN" -c 'CREATE EXTENSION IF NOT EXISTS vector;'
```

Linux 部署需要提供等价的 ONNX Runtime、Opus 开发库和 PostgreSQL/pgvector 依赖。仅运行 Manager 时不需要音频设备。

### 2. 配置 Manager

项目提供 `data/manager.yaml` 作为本地开发配置。至少修改数据库连接、JWT 密钥和管理员初始密码；不要将真实凭据提交到仓库。

```yaml
server:
  addr: ":9090"
database:
  dsn: "postgres://USER:PASSWORD@localhost:5432/orionx?sslmode=disable"
jwt:
  secret: "replace-with-a-long-random-secret"
admin:
  username: "admin"
  password: "replace-this-initial-password"
```

也可通过环境变量覆盖：`DB_DSN`、`JWT_SECRET`、`ADMIN_USERNAME`、`ADMIN_PASSWORD` 与 `LOG_LEVEL`。首次启动时会创建配置中的管理员；账户已存在时不会覆盖其密码。

可选启用对象存储（用于上传与签名访问音色参考音频、图片、知识库文档原件）。不配置时资源接口返回 503，其余功能不受影响：

```yaml
storage:
  type: "s3"                                     # 走 S3 协议，可对接七牛云 Kodo / AWS S3 / MinIO 等
  endpoint: "https://s3.cn-east-1.qiniucs.com"   # 七牛华东-浙江；region 必须与 endpoint 区域一致
  region: "cn-east-1"
  bucket: "your-bucket"                          # 七牛填「S3 空间名」（空间概览可见，可能不同于空间名）
  access_key: ""                                 # 建议只放环境变量，不落配置文件
  secret_key: ""
  use_path_style: true                           # 七牛 / MinIO 建议 true；AWS S3 可设 false
  prefix: ""                                     # 可选 key 前缀，多环境共桶时用 dev/prod
  presign_ttl: "15m"                             # 预签名 URL 有效期
```

以上字段可用环境变量覆盖：`STORAGE_ENDPOINT`、`STORAGE_REGION`、`STORAGE_BUCKET`、`STORAGE_ACCESS_KEY`、`STORAGE_SECRET_KEY`。对象存储为私有桶即可：读取一律通过服务端签发的预签名 URL（默认 15 分钟有效）。

### 3. 启动控制面与运行时

在三个终端分别执行：

```bash
# 终端 1：管理 API，默认 http://localhost:9090
make run-manager

# 终端 2：安装并启动管理控制台，默认 http://localhost:5173
make install-frontend
make dev-frontend

# 终端 3：实时语音与 Telegram 运行时
# data/wsserver.yaml 中的 manager.url 应指向 Manager
make run-wsserver
```

访问 `http://localhost:5173`，使用在 `data/manager.yaml` 中配置的管理员账号登录。依次创建供应商、模型和音色，创建智能体与设备并将模型/音色绑定到智能体。此后客户端携带该设备的 `device_id` 连接 `ws://localhost:8080/ws` 即可建立会话。

`data/wsserver.yaml` 的服务级配置示例：

```yaml
server:
  addr: ":8080"
  ws_path: "/ws"
health:
  addr: ":8081"
manager:
  url: "http://localhost:9090"
logging:
  level: "info"
  format: "console"
```

运行时可用 `MANAGER_URL`、`HEALTH_ADDR` 和 `LOG_LEVEL` 覆盖对应配置。Manager 健康检查为 `http://localhost:9090/healthz`，运行时健康检查默认为 `http://localhost:8081/healthz`。

## 管理控制台

`web/manager` 是 React + Vite 控制台，开发服务器会将 `/api` 和 `/internal` 代理到 `http://localhost:9090`。控制台当前覆盖：

| 区域 | 能力 |
| --- | --- |
| 智能体与设备 | 创建、编辑、删除智能体；设置 ASR VAD、TTS 音色和参数、LLM 提示词、设备及 Telegram Token；提供浏览器内快速对话 |
| 模型与供应商 | 管理 API 供应商、ASR/TTS/LLM 模型和模型音色；按语言筛选可用资源 |
| MCP | 浏览市场、维护私有 MCP 服务、测试连接/工具调用，并绑定到智能体 |
| 数据 | 按智能体/设备查看与删除记忆；管理知识库、文档上传、URL 导入、检索和绑定 |
| 账户 | 登录、修改密码、绑定邮箱、签发/吊销 API Key（访问密钥）与用量资源页面 |

构建静态前端：

```bash
make build-frontend
```

构建结果位于 `web/manager/dist/`。生产环境应通过 Nginx、Caddy 等反向代理托管该目录，并将 `/api` 代理至 Manager；项目当前不由 Go 二进制直接托管前端静态文件。

## WebSocket 接入

小智 WebSocket 服务默认监听 `ws://HOST:8080/ws`。客户端在第一条文本帧发送 `hello`，其中 `device_id` 必填；服务端据此解析设备配置并返回协商后的会话参数。

接入鉴权是可选的：升级请求带上控制台签发的 API Key（`Authorization: Bearer ox:sk:...` 或 `?access_token=ox:sk:...`，需勾选「语音接入」范围）时，服务端会校验密钥与设备属主，只能接入自己名下的设备；不带的连接照旧按 `device_id` 接入。要锁定为“无钥不入”，在 `wsserver.yaml` 里打开 `auth.require_api_key: true`（需要 `manager.token` 与 Manager 的 `internal.token` 一致）。

```json
{
  "type": "hello",
  "device_id": "device_xxx",
  "mode": "auto",
  "audio_params": {
    "format": "opus",
    "sample_rate": 16000,
    "channels": 1,
    "frame_duration": 60,
    "bits_per_sample": 16
  },
  "features": { "mcp": true }
}
```

- 上行音频为二进制 WebSocket 帧，支持 PCM 或 Opus；内部统一处理为 16 kHz、单声道、PCM16LE。
- `auto` 模式使用服务端 Silero VAD 判断说话边界；`manual` 模式由 `listen start` / `listen stop` 控制轮次。
- `listen` 的 `detect` 状态可直接提交文本；`abort` 会立即取消当前 LLM/TTS 轮次。
- 服务端以 JSON 控制帧返回 `stt`、`llm` 和 `tts` 状态，并以二进制帧返回合成音频。`iot` 用于设备控制，`mcp` 用于客户端作为设备端 MCP Server 的场景。

完整字段、时序、音频协商、播放节奏和限制见 [docs/wsserver-protocol-design.md](docs/wsserver-protocol-design.md)。

## 本地语音 CLI

`cmd/voicebot` 保留为本机麦克风/扬声器调试入口，使用 `data/voicebot.json` 配置 ASR、TTS、LLM 与本地音频参数：

```bash
cp voicebot.example.json data/voicebot.json
# 填入 API Key 后运行
make run-voicebot
```

它适合验证音频链路；多设备、WebSocket、Telegram、知识库和控制台管理应使用 Manager + `wsserver` 组合部署。

## API 与数据模型

Manager API 默认位于 `http://localhost:9090`：

- `GET /api-docs`：API 索引
- `GET /swagger/index.html`：Swagger UI
- `POST /api/auth/login`：获取 JWT
- `/api/voicebots`、`/api/providers`、`/api/models`、`/api/mcp`：认证后的管理 API
- `/api/data/memory`、`/api/data/knowledge`：记忆与知识库 API
- `/api/assets`：资源上传、浏览与预签名访问（需配置对象存储）
- `/api/apikeys`：签发、列出、删除访问密钥，验证账号密码后可再次复制明文

访问密钥让程序化调用不必持有控制台会话，请求带 `X-API-Key: ox:sk:...` 或 `Authorization: Bearer ox:sk:...` 即可。它的可达范围只有智能体（`/api/voicebots`、`/api/agent-templates`）、MCP（`/api/mcp`）与数据（`/api/data/*`、`/api/assets`）三类接口，由权限范围 `apikey:scope:all` / `apikey:scope:read` / `apikey:scope:agent` / `apikey:scope:mcp` / `apikey:scope:data` 决定；`apikey:scope:voice` 不是 REST 命名空间，而是接入 wsserver 的会话凭据（见上文「WebSocket 接入」）。供应商密钥、模型与音色、计费、密钥管理仍然只认控制台 JWT。签发与管理只能在控制台会话里做——密钥自己调不了 `/api/apikeys`，所以不存在拿旧密钥换新密钥的提权路径。设计说明见 [docs/apikey-design.md](docs/apikey-design.md)。

`/internal/*` 路由供 `wsserver` 在受信任网络内加载设备配置、写入记忆/会话轮次和执行知识检索使用，**没有 JWT 保护**。生产环境必须将 Manager 内部接口限制在私有网络或网关策略之后，不要直接暴露到公网。

## 工程结构

| 路径 | 说明 |
| --- | --- |
| `cmd/manager/` | Manager HTTP API、JWT 认证、OpenAPI 与控制面启动入口 |
| `cmd/wsserver/` | 多通道运行时启动入口与健康检查 |
| `cmd/voicebot/` | 本地麦克风/扬声器 CLI 调试入口 |
| `web/manager/` | React + Vite 管理控制台 |
| `internal/channels/` | 通道抽象、小智 WebSocket、Telegram 实现 |
| `internal/audio/` | ASR/TTS 处理器、VAD、重采样、PCM/Opus 编解码 |
| `internal/agent/` | 流式 LLM Agent、工具调用与子任务 |
| `internal/memory/` | 记忆快照、上下文压缩、后台回顾与会话检索 |
| `internal/knowledge/` | 文档解析、切分、向量化、pgvector 检索与 HTTP 客户端 |
| `internal/storage/` | 对象存储适配层（S3 协议，兼容七牛 Kodo / MinIO / AWS S3） |
| `internal/assets/` | 资源上传校验、对象键规则、元数据与预签名访问 |
| `internal/apikey/` | 访问密钥领域层：凭据格式、摘要与权限范围判定 |
| `internal/tools/` | 工具注册表、MCP 客户端、记忆/知识库工具 |
| `internal/store/` | GORM 数据模型与 PostgreSQL 持久化 |
| `pkg/pipeline/` | 线性与 DAG 流式处理管道 |
| `docs/` | 协议、架构与模块设计文档 |

## 开发

```bash
make build            # 构建 voicebot、wsserver、manager 到 bin/
make test             # go test ./...
make test-audio       # 需要音频设备与 ASR 配置
make lint             # golangci-lint run ./...
make swagger          # 更新 Manager OpenAPI 文档

make install-frontend # 安装前端依赖
make dev-frontend     # 启动 Vite 开发服务器
make build-frontend   # 类型检查并构建前端
```

修改 Go 代码后请运行 `make lint`；修改 Manager 路由注释后运行 `make swagger` 并提交 `docs/manager/` 的生成结果。更多模块设计资料位于 [docs/](docs/)。

## 技术栈

- Go 1.26、Gin、Gorilla WebSocket、Zap、GORM
- PostgreSQL + pgvector；SQLite 用于本地 CLI 长期记忆
- 阿里云 DashScope 实时 ASR / CosyVoice TTS（当前内置实现）
- OpenAI-compatible LLM Provider
- Silero VAD + ONNX Runtime、PortAudio、Opus
- React、TypeScript、Vite、Tailwind CSS、shadcn/ui

## 安全与部署注意事项

- 为数据库、JWT、管理员密码、模型 API Key 和 Telegram Token 使用密钥管理或环境变量；不要采用示例配置中的默认值。
- 所有 `/internal/*` 接口仅面向服务间通信，应通过网络隔离、反向代理 ACL 或 mTLS 保护。
- WebSocket 升级当前允许任意 Origin。将浏览器客户端部署到公网前，应在反向代理或代码层加入来源校验和认证策略。
- MCP 的 `stdio` 传输会在运行时主机上执行配置的命令。仅允许可信用户配置 MCP，并限制进程权限、网络与文件系统访问。
- 对外部署时请使用 TLS（`https` / `wss`）、受限数据库账号，并设置日志、备份和健康检查告警。

## 许可证

当前仓库尚未提供 `LICENSE` 文件。使用、分发或商业部署前，请先与项目维护者确认适用的授权条款。
