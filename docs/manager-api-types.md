# Manager MVP 接口与数据类型文档

## 1. API 约定

- Base Path: `/api/v1`
- Internal Path: `/internal/v1`
- Auth: `Authorization: Bearer <access_token>`
- Content-Type: `application/json`

统一响应：

```json
{
  "code": "OK",
  "message": "",
  "data": {}
}
```

错误响应：

```json
{
  "code": "ERR_FORBIDDEN",
  "message": "permission denied"
}
```

## 2. 数据类型（Go 领域对象草案）

```go
type UserRole string

const (
    RoleAdmin      UserRole = "admin"
    RoleNormalUser UserRole = "normal_user"
)

type ResourceCategory string

const (
    ResourceLLM ResourceCategory = "llm"
    ResourceASR ResourceCategory = "asr"
    ResourceTTS ResourceCategory = "tts"
)

type ToolProtocol string

const (
    ToolProtocolMCP ToolProtocol = "mcp"
)

type EntitlementStatus string

const (
    EntitlementPending EntitlementStatus = "pending"
    EntitlementActive  EntitlementStatus = "active"
    EntitlementExpired EntitlementStatus = "expired"
    EntitlementRevoked EntitlementStatus = "revoked"
)
```

## 3. 核心表结构（MVP）

### 3.1 `platform_resources`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | 主键 |
| category | text | `llm/asr/tts` |
| provider | text | 供应商标识（SDK 路由关键字段） |
| resource_key | text unique | 资源唯一标识 |
| name | text | 展示名 |
| schema_version | int | 配置 schema 版本 |
| capabilities | jsonb | 能力标签 |
| config | jsonb | 非敏感配置 |
| credential_ref | text | 密钥引用 |
| status | text | active/inactive |
| created_by | uuid | 创建人 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 更新时间 |

建议索引：

- `uniq_platform_resources_resource_key`
- `idx_platform_resources_category_provider_status`

### 3.2 `tool_market_items`

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uuid pk | 主键 |
| tool_key | text unique | 工具唯一键 |
| name | text | 名称 |
| provider | text | 工具提供方 |
| protocol | text | 目前 `mcp` |
| config | jsonb | 协议配置 |
| status | text | active/inactive |
| created_by | uuid | 创建人 |
| created_at | timestamptz | 创建时间 |
| updated_at | timestamptz | 更新时间 |

### 3.3 `user_tool_entitlements`

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

### 3.4 `voicebots`

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

## 4. MCP 配置类型定义（工具市场）

`tool_market_items.protocol = mcp` 时，`config` 结构：

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

兼容说明：现有 loader 内部使用 `streamable`，manager 下发时可将 `stream_http` 映射为 `streamable`。

## 5. 接口草案

### 5.1 平台资源

- `POST /api/v1/admin/platform-resources`
- `GET /api/v1/platform-resources`
- `PATCH /api/v1/admin/platform-resources/:id`
- `DELETE /api/v1/admin/platform-resources/:id`

创建请求示例：

```json
{
  "category": "llm",
  "provider": "zhipu",
  "resource_key": "llm-zhipu-prod",
  "name": "Zhipu Production",
  "schema_version": 1,
  "capabilities": {"stream": true},
  "config": {"base_url": "https://open.bigmodel.cn/api/coding/paas/v4", "model": "glm-4-flash"},
  "credential_ref": "secret://manager/llm/zhipu/prod"
}
```

### 5.2 工具市场与开通

- `POST /api/v1/admin/tool-market/items`
- `GET /api/v1/tool-market/items`
- `POST /api/v1/tool-market/items/:item_id/activate`
- `GET /api/v1/me/tool-repo`
- `POST /api/v1/admin/tool-entitlements/grant`

### 5.3 Voicebot 与工具绑定

- `POST /api/v1/voicebots`
- `GET /api/v1/voicebots`
- `PUT /api/v1/voicebots/:id/tools`

绑定请求示例：

```json
{
  "entitlement_ids": [
    "f97f9b59-05ad-4a57-8e46-61d39319fe20"
  ]
}
```

### 5.4 设备解析（ws-server）

- `GET /internal/v1/devices/:device_id/resolve`

响应示例：

```json
{
  "code": "OK",
  "data": {
    "found": true,
    "voicebot_id": "7609f3ea-632e-4d44-b417-c4a6eb770413",
    "session_config": {
      "asr": {"provider": "dashscope", "model": "fun-asr-realtime"},
      "tts": {"provider": "dashscope", "model": "cosyvoice-v3-flash", "voice": "longanyang"},
      "llm": {"provider": "zhipu", "model": "glm-4-flash"},
      "tools": {"mcp": []}
    },
    "updated_at": "2026-02-15T12:00:00Z"
  }
}
```

## 6. 状态码与错误码建议

- 400: `ERR_INVALID_ARGUMENT`
- 401: `ERR_UNAUTHORIZED`
- 403: `ERR_FORBIDDEN`
- 404: `ERR_NOT_FOUND`
- 409: `ERR_CONFLICT`
- 422: `ERR_BUSINESS_RULE`
- 500: `ERR_INTERNAL`

## 7. 并发与事务要求

- 设备绑定更新：事务 + `FOR UPDATE`
- entitlement 扣减：条件更新防止超扣
- 关键路径操作写入 `audit_logs`
