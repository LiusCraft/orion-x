# Manager Provider Template API Proposal

## 背景

当前前端 `Provider Catalog` 已升级为可视化模板编辑器（字段卡片），不再依赖手写 JSON。
为了让后端可落地实现，需要把前端行为拆成稳定的接口和数据结构。

本提案目标：

- 支持 `category + provider` 维度的模板管理
- 支持模板字段类型化约束（text/number/integer/select）
- 模板字段支持多级路径（dot path，例如 `audio.codec.sample_rate_hz`）
- `base_url` 与 `access_key` 作为资源顶层必填字段，不放入模板
- 支持资源创建时按模板渲染表单并校验

## UI 侧核心实体

### ProviderTemplate

- `id`: string(uuid)
- `category`: `llm | asr | tts`
- `provider`: string（建议小写，`^[a-z][a-z0-9-]*$`）
- `status`: `active | inactive`
- `version`: number（模板版本，>=1）
- `fields`: `ProviderTemplateField[]`
- `created_at` / `updated_at`

### ProviderTemplateField

- `key`: string（建议 `^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`）
- `label`: string
- `type`: `text | number | integer | select`
- `required`: boolean
- `default_value`: string | number | null
- `helper_text`: string
- `placeholder`: string
- `min`: number
- `max`: number
- `step`: number
- `options`: `[{ value: string, label: string }]`（仅 `select`）

保留关键字（不允许作为模板字段）：

- `base_url`
- `access_key`

## 推荐后端接口（MVP）

统一响应结构继续沿用：

```json
{
  "code": "OK",
  "message": "",
  "data": {}
}
```

### 1) 查询模板（登录可读）

- `GET /api/v1/provider-templates`
- Query:
  - `category` 可选
  - `provider` 可选
  - `status` 可选

返回：

```json
{
  "items": [
    {
      "id": "uuid",
      "category": "llm",
      "provider": "zhipu",
      "status": "active",
      "version": 3,
      "fields": [
        {
          "key": "model",
          "label": "Model",
          "type": "text",
          "required": true,
          "default_value": "glm-4-flash"
        }
      ],
      "created_at": "2026-02-16T10:00:00Z",
      "updated_at": "2026-02-16T10:00:00Z"
    }
  ]
}
```

### 2) 创建模板（admin）

- `POST /api/v1/admin/provider-templates`

请求体：

```json
{
  "category": "llm",
  "provider": "zhipu",
  "status": "active",
  "fields": [
    {
      "key": "model",
      "label": "Model",
      "type": "text",
      "required": true,
      "default_value": "glm-4-flash"
    }
  ]
}
```

### 3) 更新模板（admin）

- `PATCH /api/v1/admin/provider-templates/:id`

可更新字段：

- `status`
- `fields`（建议全量替换）

语义建议：每次成功更新自动 `version + 1`。

### 4) 删除模板（admin）

- `DELETE /api/v1/admin/provider-templates/:id`

建议默认软删除（`status=inactive` 或 `deleted_at`），避免影响历史资源。

## 资源接口联动建议

现有 `platform_resources` 接口保留不变，但建议新增字段：

- `provider_template_id`（可选）
- `provider_template_version`（可选）

创建/更新资源时建议请求体包含：

- `base_url`（required）
- `access_key`（create required，edit optional）

并由后端做两步校验：

1. `category + provider` 存在可用模板
2. `config` 满足模板字段规则

这样前端和后端校验逻辑可对齐，减少“前端通过、后端失败”的情况。

## 建议表结构（MVP）

### `provider_templates`

- `id` uuid pk
- `category` text not null
- `provider` text not null
- `status` text not null default `active`
- `version` int not null default 1
- `fields` jsonb not null
- `created_by` uuid not null
- `created_at` timestamptz not null
- `updated_at` timestamptz not null

唯一约束建议：

- 若只允许单活模板：`unique(category, provider)`
- 若允许多版本并存：`unique(category, provider, version)` + status 控制活跃版本

## 错误码建议

- `400 ERR_INVALID_ARGUMENT`：字段结构、类型、范围不合法
- `401 ERR_UNAUTHORIZED`
- `403 ERR_FORBIDDEN`
- `404 ERR_NOT_FOUND`
- `409 ERR_CONFLICT`：同 category/provider 重复冲突
- `422 ERR_TEMPLATE_VALIDATION`：资源 config 不满足模板
