# 资源存储（对象存储）与资源库设计

日期：2026-09-19
状态：后端（对象存储层 + 资源 API）已实现并验证；统一管理页面与领域接入待办（见 §11、§13）
关联：[知识库设计](2025-07-15-knowledge-base-design.md)、[TTS SDK 设计](../../tts-sdk-design.md)

## 1. 目标

给系统补一层「资源（文件）」能力：上传、存储、浏览、删除，覆盖三类已知资源。

| 资源 | purpose | 产生入口 | 消费方 |
| --- | --- | --- | --- |
| 音色复刻参考音频 | `voice_sample` | 语音复刻流程 | 厂商复刻 API（需要可访问的 URL） |
| 图片（logo / 封面 / 头像） | `image` | 资源库页面 | 控制台 UI（后续：智能体 logo） |
| 被向量化的文档 | `kb_document` | 知识库上传 | 解析/向量化；用户下载原件 |

三个技术目标：

1. **对象存储适配层**：`internal/storage`，用 AWS S3 SDK（`aws-sdk-go-v2`）实现；换厂商只改配置，代码里不出现厂商分支。本次以七牛云 Kodo 为首个接入目标——Kodo 提供 S3 兼容接口，因此**不引入七牛的厂商 SDK**。
2. **统一资源服务**：`assets` 表 + 上传/浏览/预签名/删除 HTTP API。
3. **持久化改造**：知识库文档与音色参考音频不再「只活在请求内存里」，支持失败重试与级联删除。

本次不做的事见 §10。

## 2. 现状

| 位置 | 现状 | 问题 |
| --- | --- | --- |
| `cmd/manager/handler/data_knowledge.go:166` `UploadDocument` | multipart → `kbSvc.IngestDocument(reader)` | 文件被 `io.ReadAll` 读进内存（`internal/knowledge/service.go:168`），处理完即丢弃 |
| `internal/knowledge/service.go:224` `RetryDocument` | 文件类型直接报错「请重新上传」（:245） | 失败只能用户重传，原件不可回溯 |
| `cmd/manager/handler/voice.go:249` `Clone` | 要求调用方自带公网 `source_audio_url` | 控制台上传的音频无处安放；厂商需要 URL 时也没有稳定来源 |
| `web/manager/src/pages/voice/VoiceClonePage.tsx:25` | `handleFileSelect` 写死文件名、进度是 `setInterval` 随机数 | 上传是假的 |
| 图片 / logo | 无任何存储能力（`AgentTemplate.icon` 只有 emoji） | 无法上传图片 |
| 全局 | 没有对象存储层 | 多副本 / 容器重建后文件即丢；也无法回答「我上传过什么」 |

## 3. 架构

```
┌── 浏览器（web/manager）────────────────────────────────────┐
│  资源库页 / 知识库页 / 语音复刻页                              │
│    ① 上传：POST /api/assets（multipart，经 manager）          │
│    ③ 浏览：<img>/<audio> 直连预签名 URL                       │
└───────────────┬───────────────────────────┬────────────────┘
                │ ① multipart               │ ③ 预签名 GET
                ▼                           │
┌── cmd/manager ────────────────────────────┼────────────────┐
│  handler/asset.go        handler/data_knowledge.go         │
│        │                        │                          │
│        ▼                        ▼                          │
│  internal/assets.Service  ←→  internal/knowledge.Service   │
│    ├─ Purpose 校验（扩展名/大小/魔数）                        │
│    ├─ key：{prefix}/{purpose}/{owner}/{assetID}{ext}        │
│    ├─ store.AssetStore（PostgreSQL）                        │
│    └─ storage.Storage（对象存储接口）                        │
│             │                                              │
│        internal/storage/s3.go（aws-sdk-go-v2）              │
└─────────────┼──────────────────────────────────────────────┘
              │ ② PutObject / GetObject / DeleteObject
              ▼
      ┌───────────────────────────────────────┐
      │ 对象存储（七牛 Kodo，S3 兼容接口）        │
      │ 私有空间，不支持匿名访问 → 只能预签名      │
      └───────────────────────────────────────┘
```

数据流要点：

- **写只经过 manager**：校验、落对象、落库都在一次请求里完成；浏览器不持有对象存储凭证，桶也不需要配 CORS。
- **读不经过 manager**：manager 只签发预签名 URL（本地 HMAC 计算，无网络往返），浏览器 / 厂商直连对象存储取文件。
- 对象存储不可用时资源接口返回 503；知识库上传保留原有的内存直传路径，保证无对象存储的本地开发不受影响（§9.3）。

## 4. 关键决策

### 4.1 SDK：`aws-sdk-go-v2`（S3）

| 方案 | 结论 |
| --- | --- |
| `aws-sdk-go-v2/{config,credentials,service/s3}` | ✅ 选定：S3 协议事实标准，预签名 / 重试 / 错误映射齐全，换厂商只改配置 |
| `qiniu/go-sdk`（Kodo 原生 API） | ❌ 把 manager 绑死在七牛，与「接入任一厂商」矛盾 |
| `minio-go` | ❌ 面向 MinIO 生态，AWS SDK 覆盖更完整、长期更稳 |

实测（2026-09-19，`go build` + httptest 抓包验证）：`aws-sdk-go-v2 v1.47.0`、`config v1.33.5`、`credentials v1.20.5`、`service/s3 v1.113.1`。

### 4.2 七牛 Kodo 接入要点

依据：七牛《AWS S3 兼容》《服务域名》《签名认证》《兼容 API》文档 + 本机 SDK 抓包。

| 事项 | 结论 |
| --- | --- |
| 服务域名 | `https://s3.<region>.qiniucs.com`，如华东-浙江 `s3.cn-east-1.qiniucs.com`（`region=cn-east-1`）；华东-浙江2 `cn-east-2`、华北-河北 `cn-north-1`、华南-广东 `cn-south-1`、北美-洛杉矶 `us-north-1`、亚太-新加坡 `ap-southeast-1` 等 |
| bucket | 填**「S3 空间名」**：七牛空间名全局唯一时二者相同，否则由系统另分配（空间概览 / GetService 可查）——配置注释必须写明，否则会出现「本地 MinIO 能跑、七牛 404」 |
| 签名 | 兼容 SigV2 / SigV4，SDK 用 SigV4；请求头签名与 Query 签名都支持 → 预签名可用 |
| 寻址 | path-style 与 virtual-host 都支持；默认 `use_path_style: true`（避免空间名含非法 DNS 字符） |
| 匿名访问 | **S3 域名不支持匿名访问**（必须带签名）→ 读取一律走预签名，不设计「公开桶直链」 |
| 校验和 | AWS SDK 新版默认给 PutObject 附加 CRC32 与 `aws-chunked` 传输编码，七牛兼容列表未包含 → 显式 `RequestChecksumCalculation: WhenRequired`、`ResponseChecksumValidation: WhenRequired`，退回「普通 PUT + 真实 payload `x-amz-content-sha256`」（抓包已确认请求头形态） |
| 依赖接口 | PUT/GET/DELETE/HEAD Object、HEAD Bucket、PUT Bucket cors 全兼容 → 上传、读取、删除、启动自检、未来浏览器直传都有路 |

### 4.3 上传通道：manager 代理上传（不做浏览器直传）

| 维度 | 代理上传（选定） | 预签名直传 |
| --- | --- | --- |
| 校验 | 服务端唯一入口，大小 / 类型 / 魔数 / 归属都能卡 | 需事后校验，或把规则写进桶策略 |
| 部署 | 私有网络也能用；无需桶 CORS | 桶必须对终端用户网络可达 + 配 CORS |
| 前端 | 一个 FormData（axios 还能出上传进度） | 申请 URL → 上传 → 确认，需要状态机与失败补偿 |
| 成本 | 流量过 manager（单文件 ≤50MB） | 省 manager 带宽 |

结论：v1 走代理上传。出现大文件（视频/模型）或带宽瓶颈时，`Storage` 接口加 `PresignPut` + 引入「待确认资源」状态即可平滑升级。

### 4.4 读取通道：预签名 URL（不自建反代流式接口）

- Kodo S3 域名不支持匿名访问，预签名是唯一「免凭证且不泄露 AK/SK」的访问方式。
- 预签名是本地计算（HMAC），列表逐条生成也不产生额外网络往返 → 列表响应可直接带 `url`。
- 浏览器 `<img>` / `<audio>` 直连对象存储：不占 manager 带宽，也不涉及 CORS。
- 下载：预签名时可带 `response-content-disposition: attachment; filename=...`（七牛文档：签名请求支持自定义标准响应头，匿名请求不支持），前端用普通 `<a>` 导航即可下载，无需读成 blob。**该行为需真机复核一次**（§12.3）。
- 备选：manager 反代 `GET /api/assets/:id/content`。仅在「最终用户网络访问不到对象存储」（纯内网部署）时才需要，v1 不做。

### 4.5 元数据：统一 `assets` 表

- 「我上传过什么」必须有统一来源，浏览 / 筛选 / 删除 / 配额也需要一个列表面。
- 领域表只存**指针**（`documents.asset_id`、`model_voices.source_asset_id`），不复制 key / URL。
- 只存 `object_key`，**不存 URL**（预签名会过期）；需要时现算。
- 不做内容去重（sha256）与引用计数：跨用户所有权与 GC 复杂度换不来当前收益。

### 4.6 归属与生命周期规则

| purpose | 创建 | 删除 | 资源库页面 |
| --- | --- | --- | --- |
| `image` | 资源库上传 | 资源库删除 | 可上传 / 可删除 |
| `voice_sample` | 语音复刻流程 | 音色删除时级联 | 只读（标「来源：音色复刻」） |
| `kb_document` | 知识库上传 | 文档删除时级联 | 只读（标「来源：知识库」） |

规则：**资源由使用它的领域创建与销毁，资源库负责统一浏览**。避免「在资源库删掉一个文档资源、知识库那边悄悄坏掉」的隐式级联。

一致性约定：

- 上传顺序 **先 PutObject 再落库**：任一步失败都不会出现「库里指向不存在对象」的资源。
- 删除顺序 **删对象 → 删资源行 → 删领域行**；S3 `DeleteObject` 对不存在的 key 返回成功 → 重试幂等。
- 删对象失败只告警、不阻塞用户操作（不出现「删不掉」）；残留对象按孤儿处理（§10）。

## 5. 模块与接口

```
internal/storage/            # 对象存储适配层（无业务概念）
├── storage.go   # Storage 接口、Config、New 工厂、ErrNotFound
├── s3.go        # aws-sdk-go-v2 实现（AWS S3 / Kodo / MinIO / OSS 同一份代码）
└── s3_test.go   # httptest 假 S3：校验寻址、签名、请求头

internal/assets/             # 资源业务层
├── service.go   # Service：Purpose 校验、key 生成、上传 / 删除 / 列表 / 预签名
└── service_test.go

internal/store/asset.go      # Asset 模型 + AssetStore（分页 / 筛选 / 删除）
cmd/manager/handler/asset.go # HTTP API
web/manager/src/pages/data/AssetsPage.tsx   # 资源库页面
```

`internal/storage/storage.go`：

```go
// Storage 是对象存储的最小能力集。实现：s3.go（S3 协议，兼容 Kodo/MinIO/OSS 等）。
type Storage interface {
	Put(ctx context.Context, key string, body io.Reader, opts PutOptions) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	// PresignGet 返回带签名的临时读取 URL（本地计算，不发网络请求）。
	PresignGet(ctx context.Context, key string, ttl time.Duration, opts PresignOptions) (string, error)
	Ping(ctx context.Context) error // HeadBucket，启动自检用
}

type PutOptions struct {
	ContentType string
	Size        int64 // 必填：S3 普通 PUT 需要 Content-Length（不走 aws-chunked）
}

type PresignOptions struct {
	// DownloadFileName 非空时附带 response-content-disposition: attachment
	DownloadFileName string
}

var ErrNotFound = errors.New("storage: object not found")
```

`internal/assets/service.go`：

```go
type Purpose string

const (
	PurposeImage       Purpose = "image"
	PurposeVoiceSample Purpose = "voice_sample"
	PurposeKBDocument  Purpose = "kb_document"
)

type Service struct {
	store      *store.AssetStore
	backend    storage.Storage
	prefix     string
	presignTTL time.Duration
}

// Create 校验 + 落对象 + 落库；任一步失败不留半截记录。
func (s *Service) Create(ctx context.Context, up Upload) (*store.Asset, error)
func (s *Service) List(ctx context.Context, q ListQuery) ([]store.Asset, int64, error)
func (s *Service) Get(ctx context.Context, ownerID, assetID string) (*store.Asset, error) // 归属校验
func (s *Service) Delete(ctx context.Context, ownerID, assetID string) error             // 仅 image
func (s *Service) PresignURL(ctx context.Context, a *store.Asset, download bool) (string, time.Duration, error)
// Open / DeleteByID 供领域流程使用（不校验归属，调用方已校验）；随领域接入（§13 Phase 4/5）实现
func (s *Service) Open(ctx context.Context, assetID string) (io.ReadCloser, error)
func (s *Service) DeleteByID(ctx context.Context, assetID string) error

type Upload struct {
	OwnerID  string
	Purpose  Purpose
	FileName string
	Size     int64
	Body     io.Reader // multipart.File：可 Seek，便于魔数探测后回到起点
}
```

## 6. 数据模型

```go
// Asset 一次上传产生的资源记录（对象存储里的对象 + 元数据）
type Asset struct {
	ID        string `gorm:"primaryKey;type:varchar(36)" json:"id"`
	OwnerID   string `gorm:"not null;index;type:varchar(36)" json:"owner_id"`
	Purpose   string `gorm:"not null;index;type:varchar(32)" json:"purpose"`    // image|voice_sample|kb_document
	Name      string `gorm:"not null;type:varchar(256)" json:"name"`            // 原始文件名，仅展示
	ObjectKey string `gorm:"not null;uniqueIndex;type:varchar(512)" json:"-"`   // 存储键
	MimeType  string `gorm:"type:varchar(128)" json:"mime_type"`                // 探测结果
	Size      int64  `gorm:"not null;default:0" json:"size"`
	BaseModel
}

func (Asset) TableName() string { return "assets" }
```

JSON 响应额外带 `url`（预签名，运行时计算、不入库）。

存储键规则：`{prefix/}{purpose}/{ownerID}/{assetID}{ext}`，例如 `kb_document/6f9c…/b21d….md`。

- `assetID` 全局唯一 → 不会撞键；**键里不含用户输入的文件名**（避免路径穿越与怪字符），原始名只留在 `name` 列。
- `prefix` 用于多环境共用一个桶（`dev` / `prod`）。
- 扩展名取原文件名小写，且必须命中用途白名单。

存量表改动（`store.Open` 的 AutoMigrate 自动加列）：`documents.asset_id varchar(36) index`、`model_voices.source_asset_id varchar(36)`。

## 7. HTTP API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/assets` | multipart 上传：`file` + `purpose`，201 返回资源 |
| GET | `/api/assets` | 分页浏览：`purpose`、`q`（文件名模糊）、`page`、`page_size`（默认 20，上限 100） |
| GET | `/api/assets/:id` | 资源详情（含预签名 `url`） |
| GET | `/api/assets/:id/url` | 取预签名 URL；`?download=1` 返回带 attachment 的下载 URL |
| DELETE | `/api/assets/:id` | 删除自有 `image` 资源（其它用途 403，提示去领域页面） |

响应示例：

```json
{
  "id": "0a4f…",
  "purpose": "image",
  "name": "logo.png",
  "size": 184320,
  "mime_type": "image/png",
  "url": "https://s3.cn-east-1.qiniucs.com/orionx/image/…/….png?X-Amz-Algorithm=…&X-Amz-Signature=…",
  "created_at": "2026-09-19T10:12:00+08:00",
  "creator": "8c21…"
}
```

错误语义：

| 场景 | 状态码 | 说明 |
| --- | --- | --- |
| 未配置对象存储 | 503 | `对象存储未配置`（与知识库 `needSvc` 同风格） |
| 用途非法 / 扩展名不在白名单 / 魔数不匹配 / 超大小 | 400 | 原因明确（如「仅支持 .png/.jpg/.jpeg/.webp/.gif，最大 5MB」） |
| 请求体超限 | 413 | `http.MaxBytesReader` 兜底，防超大 body 拖垮进程 |
| 归属不符 / 删除非 `image` 资源 | 403 | — |
| 资源不存在 | 404 | — |
| 对象存储写入 / 删除失败 | 502 | `对象存储写入失败，请重试` |

用途校验规则（`internal/assets` 内的 `specs` 表，单一事实来源）：

| purpose | 扩展名白名单 | 大小上限 | 魔数校验（`http.DetectContentType`） |
| --- | --- | --- | --- |
| `image` | .png .jpg .jpeg .webp .gif | 5MB | 必须 `image/*` |
| `voice_sample` | .wav .mp3 .m4a .flac | 20MB | 必须 `audio/*` 或 `video/mp4`（m4a 是 MP4 容器） |
| `kb_document` | 文本类扩展名 = 解析器注册表（`internal/knowledge/parser/text.go:16`：.txt .md .markdown .json .csv .log .yaml .go .py …） | 50MB | 必须 `text/*` |

- 扩展名是权威判定（决定 key 后缀与后续解析）；魔数只拦「改名伪装」（.png 实为 HTML/二进制），不匹配直接 400。
- 不支持 `.svg`（脚本执行风险）与二进制格式（PDF/DOCX 目前没有解析器，上传即无意义，等解析器接入后放开）。
- `kb_document` 白名单由 `parser.DefaultRegistry()` 支持的扩展名派生，避免「上传成功但解析不出文本」。

## 8. 业务接入

### 8.1 知识库（HTTP 契约不变，内部落对象存储）

- `POST /api/data/knowledge/knowledge_bases/:kb_id/documents` 的 multipart 契约**不变**（前端 `KnowledgePage.tsx` 零改动）。
- 流程：multipart → `assets.Create(purpose=kb_document)` → 写 `Document.AssetID` → 异步 `ingestAsync` 从对象存储 `Open` 原件解析，不再依赖请求内存。
- `RetryDocument`：文件类型文档也能重试（从对象存储重读），删掉 `internal/knowledge/service.go:245` 的「请重新上传」；仅当历史文档没有 `asset_id` 时保留旧提示。
- `DELETE /api/data/knowledge/documents/:doc_id`：删向量 → 删资源（对象 + 行）→ 删文档行。
- 顺带修既有问题：`DeleteKB`（`internal/knowledge/service.go:124`）目前只删向量与知识库行，文档行会残留；改为级联删除该 KB 下所有文档及其资源。

### 8.2 音色复刻（参考音频入对象存储）

- 请求体新增 `source_asset_id`，与 `source_audio_url` 二选一（后者保留兼容旧调用方 / 脚本）：

```json
POST /api/models/:id/voices/clone
{ "voice_id": "vendor-voice-id", "name": "温柔女声", "source_asset_id": "0a4f…", "langs": ["zh"] }
```

- 校验：资源存在 → 归属为当前用户 → `purpose == voice_sample`，否则 400 / 403。
- 落库：写 `model_voices.source_asset_id`；`source_audio_url` 仅作为 legacy 字段保留（**不把预签名 URL 写进库里**，它会过期）。
- 厂商需要 URL 时**现算**：`assets.PresignURL(asset, false)`。若厂商是异步拉取（提交后过一段时间才下载），预签名 TTL 要单独放大——等复刻 provider 落地时确认（§11-2）。

### 8.3 图片 / logo

- v1 入口就是资源库页面：拖拽 / 选择 → `POST /api/assets`（`purpose=image`）→ 列表预览、复制预签名链接、下载、删除。
- 智能体 logo 字段（`voicebots.logo_asset_id` + 详情页展示）是独立小需求，见 §11-3。

### 8.4 资源库页面（新页面，`/data/assets`）

控制台新增的一个页面，职责单一：**把「我上传过的文件」集中列出**。当前代码里没有这个页面（§2 的现状就是文件处理完即丢，无从浏览）。

- 位置：侧栏「数据」分组 → `记忆库 / 知识库 / 数据源 / 资源库`。
- 列表列：文件名、用途（图片 / 音色样本 / 知识库文档）、类型、大小、上传时间、来源（知识库上传 / 音色复刻 / 资源库上传）、操作（预览 / 下载 / 复制链接 / 删除）。
- 筛选与分页：用途筛选 + 文件名搜索 + 分页（`GET /api/assets`）。
- 预览：图片显示缩略图（预签名 URL），音频用 `<audio>` 播放，其它类型只能下载。
- 上传：仅 `image`（见 §11-1）；音色样本、知识库文档在各自领域页面上传，这里只读展示并标明来源。
- 它**不做领域逻辑**：文档在知识库页面管、音色在音色页面管；这里的删除只对 `image` 生效。

命名提醒：与「计费 > 其他资源」（用量 / 计费，`web/manager/src/pages/billing/ResourcesPage.tsx`）、「数据 > 数据源」（外部 DB / HTTP 连接，`SourcesPage.tsx`）不是一回事；名字可以改（如「文件管理」「素材库」），不影响设计。

## 9. 配置

### 9.1 manager 配置

```yaml
storage:
  type: "s3"                                    # 目前仅 s3（S3 协议：AWS S3 / 七牛 Kodo / MinIO / 阿里云 OSS…）
  endpoint: "https://s3.cn-east-1.qiniucs.com"   # 七牛 Kodo 华东-浙江；区域对照见七牛《服务域名》
  region: "cn-east-1"                            # 必须与 endpoint 区域一致（参与 SigV4 签名）
  bucket: "orionx-assets"                       # 填「S3 空间名」（空间概览可见，可能不同于七牛空间名）
  access_key: ""                                # 建议留空，用环境变量注入
  secret_key: ""
  use_path_style: true                          # 七牛 / 自建 MinIO 建议 true
  prefix: ""                                    # 可选 key 前缀，多环境共桶时用 dev/prod
  presign_ttl: "15m"                            # 预签名有效期（≤7 天）
  max_upload_size: 52428800                     # 全局单文件上限 50MB（用途上限再细分）
```

环境变量覆盖（沿用 `applyManagerEnv` 风格）：`STORAGE_ENDPOINT`、`STORAGE_REGION`、`STORAGE_BUCKET`、`STORAGE_ACCESS_KEY`、`STORAGE_SECRET_KEY`。

凭证安全：AK/SK 不写进仓库（`manager.example.yaml` 留空）；七牛侧建议用**子账号 + 仅授权该空间的策略**，不要用主账号 AK/SK。

### 9.2 启动自检

`storage.type != ""` 时，manager 启动构造 `storage.Storage` 并 `Ping`（HeadBucket）：成功 `Infof("object storage ready: %s/%s")`；失败 `Warnf` 且照常启动（与 pgvector 初始化失败仅告警的现有风格一致），资源接口随后返回 503/502。

### 9.3 未配置时的降级

`storage.type` 为空 → 不构造资源服务：资源接口一律 503（提示「未配置对象存储」）；知识库上传走**原有内存路径**，保证本地开发与无存储部署不受影响（此时文件类型文档仍不支持重试）。

## 10. 明确不做（本次）

- 浏览器预签名直传（`PresignPut`）与「待确认资源」状态机。
- 图片处理（缩略图 / 裁剪）与 CDN 域名、公开访问；Kodo S3 域名不支持匿名访问，公开页面要用图片时另做方案。
- 内容去重（sha256）与引用计数、跨用户复用。
- 生命周期 / 归档（七牛 `x-amz-storage-class` 仅支持 STANDARD/LINE/INTELLIGENT_TIERING/GLACIER/DEEP_ARCHIVE，本次统一默认 STANDARD）、配额与计费统计。
- 孤儿对象清理任务（对象删除失败、或领域行被直接 SQL 删除时产生）；`assets` 与领域表之间不建数据库外键（现状全库无外键），因此数据库级级联也排除在外。
- PDF/DOCX 等富格式解析（`internal/knowledge/parser` 目前只有文本解析器）。
- wsserver / 设备侧读取资源（`/internal/assets/...`）；设备协议目前没有图片 / 文件能力。
- 多桶 / 多租户隔离 / 每用户独立桶；资源库分页之外的高级检索（时间范围、大小、类型聚合）。

## 11. 决策记录（2026-09-19）

1. **统一管理页面（§8.4）：暂不做。** 本轮先交付后端的「上传 + 签名访问」能力，作为其它模块的统一资源入口；是否需要统一浏览页面后续再定，页面定义保留在 §8.4 作为备选方案。
2. **语音复刻页：前端本轮不动。** `clone` 接口接受 `source_asset_id` 随 Phase 5 接入。
3. **智能体 logo 字段：不做。**

## 12. 验证与测试

### 12.1 单测（纳入 `make test`）

- `internal/storage/s3_test.go`：`httptest.Server` 当假 S3，断言
  - path-style URL `/bucket/key`，`Content-Length` / `Content-Type` 透传；
  - 请求头带 `x-amz-content-sha256`（真实 payload 哈希）且**没有** `x-amz-checksum-*`、没有 `aws-chunked`（七牛兼容性回归）；
  - 预签名 URL 含 `X-Amz-Signature` / `X-Amz-Expires`，`download` 模式含 `response-content-disposition`；
  - 404 → `ErrNotFound`（错误映射）。
- `internal/assets/service_test.go`：内存假 Storage + 校验用例
  - 扩展名 / 大小 / 魔数三类用例（含「.png 实为 HTML」的伪装用例）；
  - key 规则（prefix、扩展名小写、不含用户文件名）；
  - 上传失败时不留 DB 记录（断言调用顺序）。
- `internal/knowledge`：`RetryDocument` 的分支行为（有 / 无 `asset_id`）。

### 12.2 手工验证（本地 MinIO 或真机 Kodo）

1. 配置 `storage` 指向 MinIO（`http://localhost:9000`，`use_path_style: true`）→ 启动 manager 看到 `object storage ready`；
2. 资源库上传 png（成功）、上传 `.exe` 改名 `.png`（400）、上传 6MB 图片（400）；
3. 知识库上传 .md → 文档 ready、`assets` 有记录、对象存储里能查到原始文件；
4. 制造一次解析失败（上传纯空白 .md）→ 文档 error → 点重试（无需重传）→ 成功；
5. 删除文档 → 资源与对象一并消失；删除知识库 → 文档与资源一并消失；
6. 音色复刻上传 wav → 拿到 `asset_id` → `POST …/voices/clone` 带 `source_asset_id` 成功；用别人的 asset_id → 403；用 `purpose=image` 的资源 → 400；
7. 前端：`npm run lint && npm run build`；资源库页面筛选 / 预览 / 下载 / 复制链接 / 删除全部可用。

### 12.3 Kodo 真机复核清单（拿真实桶跑一次）

- `Ping`（HeadBucket）通过；上传 / 下载 / 删除 200；
- 预签名 URL 浏览器直接打开能取到文件；`?download=1` 的 `response-content-disposition` 生效（若不生效：前端改 blob 下载 + 桶 CORS，需另行确认）；
- 中文文件名预览 / 下载正常（key 里不含文件名，展示名走 `name` 列）；
- 空间名与 S3 空间名不一致时不报错（确认配置项语义）。

## 13. 实施顺序与状态

| Phase | 内容 | 状态 | 验收 |
| --- | --- | --- | --- |
| 1 | `internal/storage`：接口 + S3 实现 + 单测；`ManagerConfig.Storage` + env 覆盖 + 启动自检 | ✅ 已完成 | `go test ./internal/storage/`；启动日志 `object storage ready` |
| 2 | `store.Asset` + `internal/assets`（校验 / key / 上传 / 列表 / 删除 / 预签名）+ 单测；`db.go` AutoMigrate 加 `Asset` | ✅ 已完成 | `go test ./internal/assets/` |
| 3 | `handler/asset.go` + 路由 + swagger 注解（`make swagger`）+ `manager.example.yaml` / README | ✅ 已完成 | 手工验证 §12.2-1、-2 |
| 4 | 知识库集成（`Document.AssetID`、上传落对象存储、ingest 从存储读、重试、级联删除、`DeleteKB` 清理） | ⏳ 待办 | 手工验证 §12.2-3、-4、-5 |
| 5 | 音色接口集成（`model_voices.source_asset_id`、clone 校验与落库） | ⏳ 待办 | 手工验证 §12.2-6 |
| 6 | 前端资源库页面（路由 `/data/assets`、导航「资源库」、`assetsApi`、上传 / 浏览 / 预览 / 下载 / 删除） | ⏳ 待办（§11-1 决定暂不做） | `npm run lint && npm run build`；手工验证 §12.2-7 |
| — | 全局 | ✅ | `golangci-lint run ./...` 0 issues；`go test ./...` 全绿 |

## 14. 本轮实测结果（2026-09-19）

- 单测：`internal/storage`（含 Kodo 兼容性回归：path-style 寻址、真实 payload `x-amz-content-sha256`、无 `x-amz-checksum-*`/`aws-chunked`、预签名参数、`response-content-disposition`、`ErrNotFound` 映射）与 `internal/assets`（校验矩阵、key 规则、失败不留半条记录、归属与删除规则、分页归一化）全绿；`go test ./...` 全绿；`golangci-lint run ./...` 0 issues。
- E2E：manager + 本地假 S3 + 独立 Postgres 库跑通完整链路——
  上传 `image` 得到 `dev/image/<user>/<assetId>.png` 的对象键与预签名 URL → 直接打开预签名 URL 字节一致；
  `?download=1` 返回 `Content-Disposition: attachment; filename="logo.png"`；
  列表分页 / 用途筛选 / 文件名搜索正常；
  删除后资源 404 且对象从存储消失；
  错误路径：非法扩展名 400、伪装扩展名（HTML 改名 .png）400、超用途上限 400、未认证 401、越权 403、不存在 404、未配置存储 503、存储不可达 502（启动仅告警，其余功能正常）。
- 未验证（环境限制）：真实七牛 Kodo 桶（无凭证，清单见 §12.3）；413 请求体超限路径（需要 >50MB 请求体，未构造）。
