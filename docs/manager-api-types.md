# Manager MVP 接口与数据类型文档

## 1. 文档定位

该文档用于并行开发时冻结 contract，覆盖：

- API 路由与核心 DTO
- 数据库表结构与关键约束
- MCP 配置协议格式
- ws-server 集成返回结构与降级语义

## 2. 通用约定

- Base Path: `/api/v1`
- Internal Path: `/internal/v1`
- Content-Type: `application/json`
- 用户接口鉴权：`Authorization: Bearer <access_token>`
- 内部接口鉴权：`Authorization: Bearer <internal_token>`

统一成功响应：

```json
{
  "code": "OK",
  "message": "",
  "data": {}
}
```

统一失败响应：

```json
{
  "code": "ERR_FORBIDDEN",
  "message": "permission denied"
}
```

## 3. 枚举与类型（Go 草案）

```go
type UserRole string

const (
    RoleAdmin      UserRole = "admin"
    RoleNormalUser UserRole = "normal_user"
)

type UserStatus string

const (
    UserStatusActive   UserStatus = "active"
    UserStatusDisabled UserStatus = "disabled"
)

type ResourceCategory string

const (
    ResourceLLM ResourceCategory = "llm"
    ResourceASR ResourceCategory = "asr"
    ResourceTTS ResourceCategory = "tts"
)

type ResourceStatus string

const (
    ResourceStatusActive   ResourceStatus = "active"
    ResourceStatusInactive ResourceStatus = "inactive"
)

type ToolProtocol string

const (
    ToolProtocolMCP ToolProtocol = "mcp"
)

type ToolOfferType string

const (
    OfferFree           ToolOfferType = "free"
    OfferTrial          ToolOfferType = "trial"
    OfferPaid           ToolOfferType = "paid"
    OfferActivationCode ToolOfferType = "activation_code"
    OfferAdminGrant     ToolOfferType = "admin_grant"
    OfferUsagePack      ToolOfferType = "usage_pack"
    OfferTimeLimited    ToolOfferType = "time_limited"
)

type EntitlementStatus string

const (
    EntitlementPending EntitlementStatus = "pending"
    EntitlementActive  EntitlementStatus = "active"
    EntitlementExpired EntitlementStatus = "expired"
    EntitlementRevoked EntitlementStatus = "revoked"
)

type DeviceStatus string

const (
    DeviceStatusActive   DeviceStatus = "active"
    DeviceStatusDisabled DeviceStatus = "disabled"
)
```

## 4. 数据库表结构（MVP）

### 4.1 `users`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | 主键 |
| email | text unique | 登录账号 |
| password_hash | text | 密码 hash |
| role | text | `admin/normal_user` |
| status | text | `active/disabled` |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 更新时间 |
| deleted_at | timestamptz null | 软删除 |

### 4.2 `platform_resources`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | 主键 |
| category | text | `llm/asr/tts` |
| provider | text | SDK 路由关键字段 |
| resource_key | text unique | 资源唯一标识 |
| name | text | 展示名 |
| schema_version | int | 配置 schema 版本 |
| capabilities | jsonb | 能力标签 |
| config | jsonb | 非敏感配置 |
| credential_ref | text | 密钥引用 |
| status | text | `active/inactive` |
| created_by | uuid | 创建人 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 更新时间 |

建议索引：

- `uniq_platform_resources_resource_key`
- `idx_platform_resources_category_provider_status`

### 4.3 `platform_resource_versions`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | 主键 |
| entry_id | uuid | 关联 `platform_resources.id` |
| version | int | 版本号（从 1 递增） |
| config_snapshot | jsonb | 配置快照 |
| credential_ref_snapshot | text | 凭据引用快照 |
| published_at | timestamptz | 发布时间 |

### 4.4 `tool_market_items`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | 主键 |
| tool_key | text unique | 工具唯一键 |
| name | text | 名称 |
| provider | text | 工具提供方 |
| protocol | text | `mcp`（MVP） |
| config | jsonb | 协议配置 |
| status | text | `active/inactive` |
| created_by | uuid | 创建人 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 更新时间 |

### 4.5 `tool_offers`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | 主键 |
| tool_item_id | uuid | 关联 `tool_market_items.id` |
| offer_type | text | 开通方式 |
| price | numeric(18,2) null | 价格 |
| currency | text null | 币种 |
| quota_total | bigint null | 总额度 |
| duration_seconds | bigint null | 有效期秒数 |
| status | text | `active/inactive` |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 更新时间 |

### 4.6 `user_tool_entitlements`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | 主键 |
| user_id | uuid | 用户 |
| tool_item_id | uuid | 工具市场项 |
| offer_id | uuid | 开通方案 |
| source_type | text | purchase/code/admin_grant/system |
| source_ref | text | 外部关联 |
| status | text | pending/active/expired/revoked |
| starts_at | timestamptz | 生效时间 |
| expires_at | timestamptz null | 过期时间 |
| quota_total | bigint null | 总额度 |
| quota_used | bigint | 已用额度 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 更新时间 |

### 4.7 `tool_usage_ledger`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | 主键 |
| entitlement_id | uuid | 关联权益 |
| voicebot_id | uuid null | 调用 voicebot |
| device_id | uuid null | 调用设备 |
| consumed_units | bigint | 消耗额度 |
| created_at | timestamptz | 记录时间 |

### 4.8 `voicebots`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | 主键 |
| owner_user_id | uuid | 所属用户 |
| voicebot_key | text unique | 唯一标识 |
| name | text | 名称 |
| llm_resource_id | uuid | 关联平台 LLM |
| asr_resource_id | uuid | 关联平台 ASR |
| tts_resource_id | uuid | 关联平台 TTS |
| settings | jsonb | 用户侧 voicebot 配置 |
| is_active | bool | 是否可用 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 更新时间 |

### 4.9 `voicebot_tool_bindings`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | 主键 |
| voicebot_id | uuid | 关联 voicebot |
| entitlement_id | uuid | 关联用户权益 |
| created_at | timestamptz | 创建时间 |

唯一约束：`uniq_voicebot_entitlement (voicebot_id, entitlement_id)`

### 4.10 `devices`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | 主键 |
| device_id | text unique | 设备标识（外部） |
| owner_user_id | uuid | 所属用户 |
| name | text | 设备名称 |
| status | text | `active/disabled` |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 更新时间 |

### 4.11 `device_bindings`

| 字段 | 类型 | 说明 |
|---|---|---|
| device_id | uuid pk | 关联 `devices.id`（一设备一绑定） |
| voicebot_id | uuid | 关联 `voicebots.id` |
| bound_by_user_id | uuid | 操作人 |
| version | int | 乐观锁版本号 |
| updated_at | timestamptz | 更新时间 |

### 4.12 `audit_logs`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | 主键 |
| actor_user_id | uuid | 操作人 |
| action | text | 动作 |
| target_type | text | 目标类型 |
| target_id | text | 目标标识 |
| payload | jsonb | 变更详情 |
| created_at | timestamptz | 创建时间 |

## 5. API 路由清单（MVP）

### 5.1 认证

- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`

### 5.2 平台资源（admin 写，用户读）

- `POST /api/v1/admin/platform-resources`
- `GET /api/v1/platform-resources?category=...&provider=...&status=...`
- `PATCH /api/v1/admin/platform-resources/:id`
- `DELETE /api/v1/admin/platform-resources/:id`

### 5.3 工具市场 + 开通 + 用户工具仓库

- `POST /api/v1/admin/tool-market/items`
- `GET /api/v1/tool-market/items`
- `PATCH /api/v1/admin/tool-market/items/:id`
- `DELETE /api/v1/admin/tool-market/items/:id`
- `POST /api/v1/admin/tool-market/items/:id/offers`
- `GET /api/v1/tool-market/items/:id/offers`
- `POST /api/v1/tool-market/items/:item_id/activate`
- `GET /api/v1/me/tool-repo`
- `GET /api/v1/me/tool-repo/:entitlement_id/usage`
- `POST /api/v1/admin/tool-entitlements/grant`

### 5.4 voicebot + 设备

- `POST /api/v1/voicebots`
- `GET /api/v1/voicebots`
- `PATCH /api/v1/voicebots/:id`
- `DELETE /api/v1/voicebots/:id`
- `PUT /api/v1/voicebots/:id/tools`
- `POST /api/v1/devices`
- `GET /api/v1/devices`
- `PATCH /api/v1/devices/:id`
- `PUT /api/v1/devices/:device_id/binding`

### 5.5 ws-server 内部解析

- `GET /internal/v1/devices/:device_id/resolve`

## 6. 关键 DTO 示例

### 6.1 创建平台资源

```json
{
  "category": "llm",
  "provider": "zhipu",
  "resource_key": "llm-zhipu-prod",
  "name": "Zhipu Production",
  "schema_version": 1,
  "capabilities": {"stream": true},
  "config": {
    "base_url": "https://open.bigmodel.cn/api/coding/paas/v4",
    "model": "glm-4-flash"
  },
  "credential_ref": "secret://manager/llm/zhipu/prod"
}
```

### 6.2 工具绑定到 voicebot

```json
{
  "entitlement_ids": [
    "f97f9b59-05ad-4a57-8e46-61d39319fe20"
  ]
}
```

### 6.3 设备解析返回（内部）

```json
{
  "code": "OK",
  "data": {
    "found": true,
    "voicebot_id": "7609f3ea-632e-4d44-b417-c4a6eb770413",
    "session_config": {
      "asr": {
        "provider": "dashscope",
        "model": "fun-asr-realtime"
      },
      "tts": {
        "provider": "dashscope",
        "model": "cosyvoice-v3-flash",
        "voice": "longanyang"
      },
      "llm": {
        "provider": "zhipu",
        "model": "glm-4-flash"
      },
      "tools": {
        "mcp": []
      }
    },
    "updated_at": "2026-02-15T12:00:00Z"
  }
}
```

## 7. MCP 配置格式（工具市场）

当 `tool_market_items.protocol = mcp` 时：

```json
{
  "transport": "stdio | sse | stream_http",
  "timeout_ms": 30000,
  "tool_name_list": ["get_device_status"],
  "auth": {
    "type": "none | bearer | api_key",
    "token": "",
    "header": "Authorization"
  },
  "stdio": {
    "command": "python",
    "args": ["server.py"],
    "env": {"K": "V"},
    "cwd": "/opt/mcp"
  },
  "sse": {
    "endpoint": "https://host/mcp/sse",
    "headers": {"Authorization": "Bearer xxx"}
  },
  "stream_http": {
    "endpoint": "https://host/mcp",
    "headers": {"Authorization": "Bearer xxx"}
  }
}
```

校验规则：

- `transport=stdio` 时 `stdio.command` 必填。
- `transport=sse` 时 `sse.endpoint` 必填。
- `transport=stream_http` 时 `stream_http.endpoint` 必填。
- `tool_name_list` 为空表示加载全部工具。

兼容说明：现有 loader 内部使用 `streamable`，manager 下发时将 `stream_http` 映射为 `streamable`。

## 8. 业务规则与一致性

- voicebot 绑定工具时，必须校验 entitlement 属于当前用户且状态为 `active`。
- entitlement 如有 `expires_at`，当前时间超出后不可再绑定和消费。
- `quota_total` 不为空时，消费需保证 `quota_used + n <= quota_total`。
- 设备绑定更新必须使用事务 + `FOR UPDATE`。
- 权益扣减应使用原子条件更新，避免并发超扣。
- 关键变更操作写入 `audit_logs`。

## 9. ws-server 集成语义

manager resolve 结果建议：

- `found=true`：ws-server 使用 `session_config`
- `found=false`：ws-server 回退本地配置

HTTP 语义建议：

- `200`: 找到或未找到（通过 `found` 区分）
- `401/403`: internal_token 无效，ws-server 直接失败
- `5xx/timeout`: ws-server 按配置降级（`fallback_local_on_timeout`）

## 10. 错误码建议

- 400: `ERR_INVALID_ARGUMENT`
- 401: `ERR_UNAUTHORIZED`
- 403: `ERR_FORBIDDEN`
- 404: `ERR_NOT_FOUND`
- 409: `ERR_CONFLICT`
- 422: `ERR_BUSINESS_RULE`
- 500: `ERR_INTERNAL`
