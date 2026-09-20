# 访问密钥（API Key）设计

## 1. 目标与边界

**目标**：让脚本、CI、外部服务在拿不到控制台会话（JWT）的情况下调用 Manager API，而且拿到的权限小于账号本身；同一套凭据也可以作为设备接入 wsserver 的会话凭据。

**不是**：

- 不是第三方厂商密钥。给 ASR/TTS/LLM 的厂商凭据在 `providers.api_key_enc` 里，与本文的访问密钥无关。
- 不是服务间凭据。`/internal/*` 仍用共享 token（`internal.token`），它管的是进程之间，不是某个用户。
- 不是设备本身的身份。小智 WebSocket 会话默认仍按 `hello.device_id` 解析设备配置；带上 `apikey:scope:voice` 的密钥才会真正鉴权（见 §3.1）。
- 当前不做：过期时间、轮换、按 key 的速率限制与配额、按 key 归属用量/账单、单路由粒度的授权。

## 2. 凭据格式与落库

```
ox:sk:<48 个十六进制字符>      # 24 字节随机数，约 192 bit 熵
```

- 前缀按仓库命名约定用 `:` 分段（AGENTS.md 的 Naming conventions），校验时靠它把 Bearer 头里的 JWT 和 API Key 分开。
- 摘要用 SHA-256，不用 bcrypt：明文是高熵随机串，不需要抗爆破的慢哈希，而认证路径每次请求都要查一次，慢哈希会把延迟和 CPU 都吃满。
- `api_keys` 表存三份关于明文的材料：`key_hash`（唯一索引，**认证走它**）、`key_secret`（AES-256-GCM 密文，**只给“验证密码后复制”用**）、`key_prefix` / `key_last4`（列表页的脱敏展示）。摘要与密文并存是故意的：封装密钥缺失或轮换时，已有密钥照旧能认证，只是复制不出来。
- 封装密钥由部署主密钥（manager 的 `jwt.secret`）经 HKDF-SHA256 + 用途标签 `orion-x:apikey:secretbox:v1` 派生：不引入新的必填配置，且与同一主密钥在别处的用途（JWT 签名）互不影响。
- 明文不在任何日志、列表、错误响应里出现。`POST /api/apikeys` 的响应是唯一一个不带密码就能拿到明文的出口，因为那一刻明文本来就是调用方刚生成的。
- `internal/apikey` 是零依赖的领域层（格式、摘要、掩码、封套、权限范围判定），`depguard` 的 `apikey-domain` 规则拦住它 import gorm / gin / store。

**威胁模型**：单纯拖库（拿到 `api_keys` 表）拿不到任何明文或可用密钥；拖库 + 拿到 `jwt.secret` 才能解出明文（而那时攻击者本来就能伪造任意用户的 JWT，所以并没有新增一类沦陷）。轮换 `jwt.secret` 后旧密文解不开（返回 500 并提示主密钥变了），但密钥的认证能力不受影响。

## 3. 认证与授权

认证入口有两个，凭据是同一个：

```
X-API-Key: ox:sk:...
Authorization: Bearer ox:sk:...
```

`middleware.Auth` 一次做完认证与授权，认证后把密钥属主写进 `userID`，业务 handler 照旧用 `middleware.UserID(c)` 做属主隔离，不需要知道调用方是会话还是密钥。

取回明文（`POST /api/apikeys/:id/reveal`）要过三道门：控制台会话（路由本身不在 API Key 表里）、属主（按 `owner_id` 过滤）、账号密码（bcrypt 比对 `users.password_hash`）。密码错给 **403 而不是 401**：前端把 401 当“会话失效”处理，会顺手登出，而这里只是密码敲错了。OAuth 注册、还没设密码的账号得到 409 与一句明确的提示。

授权是**默认拒绝**的：`cmd/manager/middleware/apikey.go` 的 `apiKeyRouteScopes` 是唯一允许 API Key 触达的路由表，最长前缀命中，且要求前缀落在路径分段边界上。

| 前缀 | 需要的范围 |
| --- | --- |
| `/api/voicebots`（含设备、渠道与 MCP 绑定） | `apikey:scope:agent` |
| `/api/agent-templates` | `apikey:scope:agent` |
| `/api/mcp` | `apikey:scope:mcp` |
| `/api/data/memory`、`/api/data/knowledge` | `apikey:scope:data` |
| `/api/assets` | `apikey:scope:data` |

范围语义：

- `apikey:scope:all`：表内命名空间可读可写，并且可以作为 wsserver 的接入凭据。
- `apikey:scope:read`：表内命名空间只放行安全方法（GET / HEAD / OPTIONS），且不能用于 wsserver 接入。
- `apikey:scope:agent` / `apikey:scope:mcp` / `apikey:scope:data`：命中对应命名空间，可读可写。
- `apikey:scope:voice`：与 REST 命名空间无关，只用于 wsserver 握手的接入鉴权（见 §3.1）。

表外路由（供应商密钥、模型与音色、语言字典、计费、`/api/apikeys` 本身）只认控制台会话，密钥调用一律 403。两点是刻意的：

1. **授权写在认证里**，不拆成两个中间件。新增路由只要挂在 `middleware.Auth` 后面就自动是“会话可用、密钥默认拒绝”，没有“忘了挂授权”这种破口。
2. **密钥管不了密钥**：`/api/apikeys` 不在表里，所以不存在“拿一把旧密钥签一把新密钥”的提权路径；控制面能力（供应商凭据、计费、账号）同理。

`RequireAdmin` 仍然挂在计费路由上：API Key 路径不写 `is_admin`，所以即使将来把计费加进表里也进不去管理端。

### 3.1 wsserver 接入（`apikey:scope:voice`）

语音连接不经过 `/api`：客户端直连 wsserver 的 WebSocket，因此这条链路不复用上面的路由表，而是“数据面问控制面”的一次同步判定。

```
客户端 ──hello(device_id) + Authorization: Bearer ox:sk:...──▶ wsserver
                                                              │ POST /internal/apikey/authorize（internal token）
                                                              ▼
                                                     manager：密钥存在？带 voice 范围？设备属于该 key 的属主？
```

- 凭据入口：`Authorization: Bearer ox:sk:...` 或 `?access_token=ox:sk:...`（浏览器连 WS 时不能设请求头，所以必须支持查询参数）。协议本身不改：hello 不带 token。
- 判定顺序与拒绝原因（都在 `internal/apikey/wire.go`，同时作为 Close 帧的 reason）：`key:not_found` → `key:scope_missing` → `device:not_found` → `device:owner_mismatch`。
- **密钥只能接入自己名下的设备**：设备 → 智能体 → `owner_id`，与密钥属主比对。跨用户拿别人的 device_id 也没用。
- 本地判定：`key:missing`（开关打开但没带凭据）、`key:unavailable`（控制面不可达、内部 token 没配、数据面没注入校验器）。**取不到结论时拒绝**——放行等于没鉴权。
- 默认只校验“客户端主动带来的密钥”（带了就必须对）；`auth.require_api_key: true` 时所有连接必须带密钥。存量设备不带凭据，所以默认不能是强制。
- 拒绝走与计费拒绝同一条路：先回一条 hello，再 Close(1008)，设备不会卡在等 hello 的状态。
- 密钥的 `last_used_at` / `call_count` 会把每次接入计进去，控制台能看出某把钥在跑哪台设备。

## 4. 生命周期与可观测

- 签发、列出、删除、复制都只从控制台会话走，且只能操作自己的密钥（`owner_id` 过滤，跨用户删除/复制返回 404）。
- 删除是硬删除。密钥没有账单或流水引用，留软删除只会让“删了但还能用”的风险变模糊。
- 每次认证成功都会把 `last_used_at` 与 `call_count` +1（一次同步 `UPDATE`）。写失败只记日志、请求照常放行：统计不该影响可用性。量大时这里可以换成本地聚合 + 批量落库。
- 每次复制明文都写一行 `apikey: revealed key <id> (owner <user>)` 到日志，供事后审计。

## 5. 代码地图

| 路径 | 职责 |
| --- | --- |
| `internal/apikey/` | 凭据格式、摘要、掩码、封套（`secretbox.go`）、`Scope` 常量与 `Allows` 判定（零依赖） |
| `internal/store/apikey.go` | `api_keys` 模型与仓储：创建、按属主列出、删除、认证 + 用量统计、解密封装 |
| `cmd/manager/handler/apikey.go` | `GET/POST /api/apikeys`、`POST /api/apikeys/:id/reveal`、`DELETE /api/apikeys/:id`，列表与错误响应只给脱敏串；另有 `/internal/apikey/authorize`（数据面握手校验） |
| `internal/apikey/client/` | 数据面调用 `/internal/apikey/authorize` 的 HTTP 客户端 |
| `cmd/manager/middleware/auth.go` | `middleware.Auth`：JWT 与 API Key 的认证 + 授权入口 |
| `cmd/manager/middleware/apikey.go` | `apiKeyRouteScopes` 路由表与最长前缀匹配 |
| `web/manager/src/pages/ApiKeysPage.tsx` | 控制台页面：列表、创建（多选范围）、一次性明文、删除 |

## 6. 测试

- `internal/apikey`：格式/唯一性、摘要、掩码不泄露中段、封套的往返与篡改/异钥/版本不匹配、`ParseScopes` 校验、`Allows`/`AllowsScope` 的读写矩阵、`Decide` 的四步判定矩阵。
- `internal/apikey/client`：httptest 验请求契约（路径、Bearer、body）、拒绝不当作错误、401/500/非法响应都视为“没走通”。
- `internal/channels/xiaozhi`：握手鉴权的本地规则——没带凭据默认放行 / 开关打开必拒、陌生凭据忽略、控制面不可达时 fail closed、拒绝原因传递给 Close 帧。
- `internal/store`：DryRun 生成的 SQL 形状——插入语句里不能出现明文、（并在测试里把 `key_secret` 解开验证与明文一致）、认证按摘要查、揭示与删除带属主条件。
- `cmd/manager/middleware`：会话与密钥两条路径、表内/表外路由、只读与可写、属主传递。
- `cmd/manager/handler`：入参校验（名字、范围、密码）、创建响应含明文、列表响应不含明文、没设密码的账号拿到 409。
- `bin/smoke_apikey.py`（gitignored）：起真 manager 的端到端冒烟，覆盖建钥 → 权限矩阵 → 用量统计 → 错误密码 403 → 正确密码复制回同一把钥 → 属主隔离 → 删除 → 语音接入判定（自建智能体+设备）。

## 7. 演进

按优先级：复制接口与接入鉴权的尝试频率限制（当前与 `/api/auth/login` 一样没有锁定策略，两者等价；要么一起加，要么一起不加）→ 接入判定结果在数据面做短 TTL 缓存（每台设备每次连接一次同步 HTTP，量大时这里是唯一的放大点）→ 过期时间与到期提醒 → 轮换（新明文换旧摘要，保留重叠窗口）→ 审计（把 `key_id` 写进日志/流水）→ 按 key 的配额与账单归属 → 把 `userID` 换成“主体”抽象，让密钥也能代表设备或服务账号。
