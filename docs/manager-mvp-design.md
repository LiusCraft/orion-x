# Manager 服务 MVP 设计（产品 + 架构）

## 1. 背景与目标

当前 `ws-server` 的设备配置绑定依赖本地配置 `voicebot.local_bindings`，缺少统一管理能力。MVP 目标是落地一个独立 `manager` 服务，用于承载：

- 用户与权限管理（`admin` / `normal_user`）
- 用户可创建多个私有 voicebot
- 平台资源管理（LLM/ASR/TTS，带 `provider`）
- 工具市场 + 用户工具仓库（可开通能力）
- `device-id -> voicebot` 的在线解析能力（供 ws-server 调用）

## 2. MVP 范围

### 2.1 In Scope

- 账号体系：登录、刷新 token、角色鉴权
- Voicebot：每个 normal_user 支持多个 voicebot
- 平台资源：`platform_resources` 统一管理 LLM/ASR/TTS
- 工具体系：工具市场、开通方案、用户工具仓库、voicebot 工具绑定
- 设备绑定：设备与 voicebot 绑定关系管理
- 内部接口：`/internal/v1/devices/:device_id/resolve`
- ws-server 超时降级策略定义

### 2.2 Out of Scope

- 完整计费闭环（支付网关）
- 复杂审批流
- 多租户隔离（组织维度）

## 3. 角色与权限

### 3.1 admin

- 管理 `platform_resources`
- 管理工具市场与开通方案
- 可执行管理员开通（grant entitlement）
- 查看全局审计日志

### 3.2 normal_user

- 管理自己的 voicebot（多条）
- 管理自己的设备与绑定
- 查看可用平台资源
- 查看工具市场，开通工具到个人工具仓库
- 只能将“自己工具仓库里的有效权益”绑定给自己的 voicebot

## 4. 领域模型（MVP）

### 4.1 核心实体

- `users`
- `voicebots`
- `devices`
- `device_bindings`
- `platform_resources`
- `platform_resource_versions`
- `tool_market_items`
- `tool_offers`
- `user_tool_entitlements`
- `tool_usage_ledger`
- `voicebot_tool_bindings`
- `audit_logs`

### 4.2 关键设计意图

- `platform_resources` 用于统一表达 LLM/ASR/TTS 资源，`provider` 决定运行时 SDK 适配器。
- `user_tool_entitlements` 是“用户工具仓库”的标准权益层，不与开通方式强绑定。
- voicebot 绑定工具时绑定 entitlement，而不是直接绑定市场条目，便于支持限时/计次/撤销。

## 5. 高层架构

```
Client/Admin Console
      |
      v
 manager HTTP API
      |
      +-- Auth/RBAC
      +-- Voicebot Domain
      +-- Resource Domain (platform_resources)
      +-- Tool Market + Entitlement Domain
      +-- Device Binding Domain
      |
      v
 PostgreSQL (GORM)

ws-server ----(internal resolve API)----> manager
```

## 6. 关键业务流程

### 6.1 用户开通工具

1. 用户在工具市场选择 `tool_market_item`
2. 发起开通（`activate`）并匹配 `tool_offer`
3. 生成 `user_tool_entitlement`
4. entitlement 状态变为 `active`

### 6.2 绑定工具到 voicebot

1. 用户请求将 entitlement 绑定到 voicebot
2. 系统校验：voicebot 属于当前用户、entitlement 属于当前用户、状态可用
3. 写入 `voicebot_tool_bindings`

### 6.3 ws-server 解析设备

1. ws-server 调用 `/internal/v1/devices/:device_id/resolve`
2. manager 返回 `session_config`
3. ws-server 使用该配置启动会话
4. 若超时且允许降级，回退到本地 resolver

## 7. 模块拆分（供并行开发）

### 7.1 A 组：认证与权限

- JWT 登录/刷新
- RBAC 中间件
- 用户状态校验（active/disabled）

### 7.2 B 组：资源域

- `platform_resources` CRUD
- `provider` + `category` 约束校验
- 资源版本快照

### 7.3 C 组：工具市场与权益域

- 工具市场 CRUD
- 开通流程（先落地 admin_grant + activation_code）
- 用户工具仓库查询

### 7.4 D 组：voicebot 与设备域

- voicebot CRUD
- 设备 CRUD
- 设备绑定 + 乐观锁/行锁

### 7.5 E 组：ws-server 集成

- internal resolve API
- ws-server manager resolver
- timeout fallback + metrics

## 8. 非功能要求

- 并发安全：绑定和权益扣减必须在事务内完成
- 可观测性：记录 manager resolve 延迟、结果分类指标
- 容错：manager 调用超时可降级，不阻塞核心握手路径

## 9. MVP 验收标准

- normal_user 至少可创建 2 个 voicebot 且隔离生效
- voicebot 工具绑定仅允许来自用户工具仓库
- `platform_resources` 可按 `category + provider` 正确产出会话配置
- ws-server 在 manager 超时场景仍能建立连接（降级生效）
