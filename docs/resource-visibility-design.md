# 资源归属（私有 / 官方）设计

> 状态：已实现 | 日期：2026-09-30 | 范围：manager HTTP API、internal/store、provider 同步、manager 控制台 | 关联：`internal/store/sync.go`、`cmd/manager/handler/provider.go`、`web/manager/src/pages/models/`

## 评审面（≤3 屏）

### 1. 结论与边界

管理员在控制台「添加厂商 / 添加模型」时可选归属：**私有**（仅创建者可见，默认）或**官方**（所有用户可见、平台维护）。后端在创建接口增加可选布尔字段 `is_system`，仅 `is_admin` 可置 true；可见性继续复用既有 `is_system` 语义，不新增数据模型、不做迁移。

| 决策 | 选择 | 理由 | 代价 |
| --- | --- | --- | --- |
| 归属字段 | 复用 `is_system` | 可见性（`provider.go:18`）、改删鉴权（`provider.go:93,176`）、控制台角标与筛选（`shared.tsx:6`）都已有该语义 | 字段名有历史包袱：请求叫 `is_system`、UI 文案叫「官方」 |
| 授权落点 | handler 内 `middleware.IsAdmin(c)` | 与既有 Update/Delete 一致；API key 鉴权永不设置 admin（`middleware/apikey.go:32-36`），天然挡住 | 两个创建端点各多 3 行判断 |
| 官方记录与 sync 的关系 | sync 的 slug 匹配只认 `source != manual` 的行 | 管理员上架的官方记录永不被 sync 更新/接管/删除；同 slug 多账号本就合法（`models.go:85-88`） | `sync.go:151-162` 三个匹配索引加过滤 |
| 控制台自定义 slug | 加「自定义」输入（所有用户） | 官方上架的主场景是自建厂商（如 `llm:deepseek`）；服务端本就接受任意 slug，未知 slug 数据面默认走 openai-completions（`internal.go:265-275`） | 前端自己兜格式校验（现存选择框只列内置 slug，`ProvidersPage.tsx:366-385`） |
| 官方模型的前置条件 | provider 也必须是官方，否则 400 | 否则其他用户看得到模型、看不到 provider | 创建路径多一次 provider 查询 |

**非目标**：创建后改档（私有↔官方）；官方记录的删除/降级（本次仍只有 SQL / sync 一条路）；语音、智能体模板等其它资源的同一开关（控制台无语音创建入口，官方音色走 `/internal/voices`，`server.go:313`）；组织/角色级可见性。

**现状**：两个创建接口一律 `IsSystem: false`（`provider.go:32-47`、`aimodel.go:40-56`），请求体没有归属字段——官方记录只能由代码同步或 SQL 产生。9/30 事故里的 DeepSeek 正是「管理员添加 + SQL 标官方」的产物；把官方化收编进正常流程，也是让 `is_system` 不再被手工改的前提。

**关键约束**：鉴权绕过面 0 —— 非管理员（含 API key）创建官方记录必须 403（curl 可验）；官方记录恒为 `is_system=true 且 source=manual`；服务端是唯一权威（前端字段不可信）；无 schema 变更，迁移量为 0。

### 2. 骨架

| 模块 | 负责 | 不负责 | 落点 |
| --- | --- | --- | --- |
| store（Provider / AIModelStore） | 按参数写入 `is_system`，固定 `source=manual` | 鉴权、可见性判断 | `internal/store/provider.go:32`、`aimodel.go:40` |
| manager handler | 解析 `is_system`、admin 鉴权、官方模型→官方 provider 校验 | 数据可见性、同步 | `cmd/manager/handler/provider.go:59`、`model.go:62` |
| sync | 只维护 `source=code` 记录；匹配排除 `manual` | 管理员上架/私有记录 | `internal/store/sync.go:151-162`、`386-404` |
| 控制台 | 归属开关、自定义 slug 输入、错误提示 | 一切鉴权（服务端权威） | `web/manager/src/pages/models/ProvidersPage.tsx:353`、`MyModelsPage.tsx:506` |

依赖方向：控制台 → HTTP → handler → store → DB；sync → store/DB。无环，控制台不直连 store。

主链路是单请求单表单写（控制台 → handler → store → DB），无跨模块协作，省略时序图；唯一的跨模块关系是 sync 的读取，见 §3.3 不变量 2。

### 3. 契约

#### 3.1 接口

```go
// isSystem=true ⇒ is_system=true、source=manual（官方）；false ⇒ 私有（现状）。
// 授权与「官方模型→官方 provider」校验在 handler，store 只负责落库。
func (s *ProviderStore) Create(name, slug, baseURL, apiKeyEnc, creator string, isSystem bool, extra datatypes.JSONMap) (*Provider, error)
func (s *AIModelStore) Create(providerID, name string, modelType ModelType, baseURL, modelID, creator string, isSystem bool, extra datatypes.JSONMap) (*AIModel, error)
```

- 错误语义：store 只返回 `provider store: create: %w` 这类 DB 错误；403 / 400 由 handler 在调用 store 之前产生，store 不返回鉴权错误。
- 并发与所有权：无状态、无锁；同 slug 允许多行，不加唯一约束（`models.go:85-88`）。
- 兼容：请求新增可选字段，旧客户端不传 = 现状；响应里的 `is_system` 早已存在（`api.ts:196-205`）。

#### 3.2 对外端点（只列变更的既有端点）

| 方法 / 路径 | 谁可调 | 变更 | 状态码 |
| --- | --- | --- | --- |
| POST `/api/providers` | 登录用户 / API key | 请求新增可选 `is_system`，仅 admin 可 true | 201 / 400 / 403 |
| POST `/api/models` | 登录用户 / API key | 同上；官方模型要求 provider 官方 | 201 / 400 / 403 |

```bash
# 管理员创建官方厂商（自定义 slug）
curl -sS -X POST https://dash.orion-x.org/api/providers -H "Authorization: Bearer $ADMIN_JWT" \
  -H 'Content-Type: application/json' \
  -d '{"name":"DeepSeek","slug":"llm:deepseek","base_url":"https://api.deepseek.com","api_key":"sk-…","is_system":true}'
# 201（2026-09-30 本地实测响应，省略手动输入字段）
{"id":"de6f42c5-…","name":"DeepSeek","slug":"llm:deepseek","base_url":"https://api.deepseek.com",
 "is_system":true,"source":"manual","created_at":"2026-09-30T14:19:05+08:00","updated_at":"2026-09-30T14:19:05+08:00","creator":"9f49e812-…"}

# 非管理员（或 API key，实测 model:write key）传 is_system=true
# 403
{"error":"admin only"}

# 官方模型挂在私有厂商下
# 400
{"error":"official model requires an official provider"}
```

幂等：不幂等——重复提交得到重复行（slug 无唯一约束），与现状一致。
刻意不接受 API key：官方创建要求 `is_admin`，API key 鉴权永不设置 admin；否则一把泄漏的 key 就能向全平台投放资源。

#### 3.3 数据结构

无新字段 / 新表 / 迁移；官方 = `is_system=true, source=manual`（`models.go:70-83`）。仅创建时定档，无状态迁移。

不变量（保证方）：

1. 官方 ⇒ 全部用户可见：`is_system = true OR creator = ?`（`provider.go:18`、`aimodel.go:18`）。
2. 官方 ⇒ `source=manual` ⇒ sync 不更新、不清理（`sync.go:389,394,399`），且匹配时跳过（本次新增的 `sync.go:151-162` 过滤）。
3. 官方模型 ⇒ 其 provider 必须官方（handler 校验）。
4. 私有 ⇒ 仅 creator 可见、仅 creator 可改删；admin 的特权只覆盖官方记录（`provider.go:93,176`、`model.go:96,140`）。

### 4. 决策与风险

**决策（候选与否决理由）**：

| 难点 | 候选 | 决策 | 否决理由 |
| --- | --- | --- | --- |
| 请求字段形状 | `is_system: bool` / `visibility: "private"\|"official"` / 另开 `/api/admin/providers` | `is_system` | 响应与库已是 `is_system`，两套名字会漂；独立端点要复制整套路由与 scope 鉴权 |
| 官方 + 内置 slug 冲突 | sync 匹配排除 manual / 拒绝官方创建内置 slug | 匹配排除 manual | 拒绝挡住了合法用法（同 slug 多账号）；原行为是 sync 静默覆盖管理员的 base_url |

**风险**：

| 风险 | 影响 | 缓解 | 触发条件（回滚 / 换方案） |
| --- | --- | --- | --- |
| 控制台先于后端上线，旧后端忽略 `is_system`，官方被静默建成私有 | 管理员误以为已上架 | 上线顺序后端 → 控制台；控制台校验响应 `is_system` 与所选不符即报错 | 出现选了官方但记录 `is_system=false` → 回滚控制台静态包重发 |
| sync 匹配过滤误伤既有 code 记录 | 内置 provider 不再更新 | 过滤只排除 `source=manual`；postgres 集成测试覆盖（§D） | 集成测试红或启动日志出现重复内置记录 → 回滚该过滤 |
| 同 slug 的官方 + 内置两条记录 | 列表重复、用户困惑 | 已有「官方/我的」筛选与 slug 标签；引导优先编辑内置记录 | 用户投诉 → 控制台按 slug 合并展示 |

待验证假设：未知 slug（如 `llm:deepseek`）在数据面回落到 openai-completions 适配器（依据 `internal.go:265-275` 的 default 分支，且 9/30 现网已在用）；验证动作见 §D。

### 5. 未决问题

| 问题 | 阻塞谁 | 谁能定 | 最晚 |
| --- | --- | --- | --- |
| 官方记录改档与删除（误建后目前只能 SQL） | 后续迭代（本次已按非目标实现） | 项目负责人 | 下一次相关需求前 |
| 语音 / 模板是否复用同一开关 | 后续迭代 | 项目负责人 | 不阻塞本次（默认不做） |

## 支撑材料

### A. 背景与现状（展开）

| 现状 | 位置 |
| --- | --- |
| 创建即私有（`IsSystem: false`） | `internal/store/provider.go:32-47`、`aimodel.go:40-56` |
| 创建请求无归属字段 | `cmd/manager/handler/provider.go:51-57`、`model.go:53-60` |
| 可见性 = 官方或本人 | `internal/store/provider.go:16-22`、`aimodel.go:17-30`、`voice.go:23-33` |
| 改删鉴权：官方仅 admin、私有仅 creator | `handler/provider.go:93,176`、`handler/model.go:96,140` |
| 官方记录目前只能由 sync / SQL 产生 | `internal/store/sync.go:19-33,192-203` |
| 控制台现状：isAdmin 已可用、表单无归属 | `ProvidersPage.tsx:79,141-164,353-467`、`MyModelsPage.tsx:117,195-208,506` |
| 官方音色的既有通道（internal token） | `handler/voice.go:274-300`、`server.go:313` |
| API key 无 admin | `cmd/manager/middleware/apikey.go:32-36` |
| 9/30 事故修复引入的 `source` 标记与回填 | `internal/store/models.go:70-83`、`db.go:51-71` |

### B. 需求与约束（展开）

| 编号 | 需求 | 判定方式 |
| --- | --- | --- |
| FR1 | 管理员创建 provider / model 时可选私有或官方，默认私有 | 勾选后 POST 带 `is_system:true`，落库 `is_system=true, source=manual` |
| FR2 | 非管理员（含 API key）不能创建官方记录 | 直调接口 403，库中不落行 |
| FR3 | 官方记录对所有用户可见，且不被 sync 改动 | 另一账号 GET 可见；重启 manager 后 sync 日志无该记录的 updated / removed |
| FR4 | 官方模型的 provider 必须官方 | 直调接口 400 |
| 质量属性 | 口径 | |
| 鉴权绕过面 | 0：所有写路径必须过 `IsAdmin`；前端字段一律不可信 | |

### C. 业界调研

不适用：沿用仓库既有 `is_system` + `RequireAdmin` 模式，无新协议 / 存储 / 依赖 / 算法。

### D. 落地与验证

实现与验证都已执行（2026-09-30，本地 manager + 本地 Postgres，独立测试库）：

1. **store + sync**：`Create` 增加 `isSystem` 参数、匹配排除 `manual`。验证：`internal/store/sync_postgres_test.go` 覆盖「同 slug 的 manual 官方行不被 sync 覆盖 / 接管 / 删除」与原有事故场景；`TestPlanStaleCleanup` 覆盖纯策略。
2. **manager API**：请求字段 + 403 / 400 校验。验证（实测三态）：admin 创建官方 provider / model → 201 且 `is_system=true, source=manual`；普通用户与 `model:write` API key 传 `is_system=true` → 403 `admin only`；官方模型配私有 provider → 400；同一 API key 建私有 provider → 201；另一用户可见官方 provider / model；重启 manager 后 sync 日志 `+0 ~0 -0`，记录不变。
3. **控制台**：归属开关 + 自定义 slug。验证：`tsc -b && vite build` 通过、oxlint 无新增告警；浏览器手测留待部署时确认。

部署顺序：后端（1+2）→ 控制台（3）；回滚 = revert 对应改动，已创建的官方记录不受影响（数据用的是既有语义）。上线后观察：sync 启动日志（updated / removed 计数）、控制台是否出现 403 / 400 报错；无新增指标。

未在测试环境验证：自定义 slug 的数据面适配器回落（§4 待验证假设）—— 依据 `internal.go:265-275` 的 default 分支，现网 `llm:deepseek` 已在用。

### E. 失败路径

不适用：创建是单表单写，无跨模块补偿 / 重试；重复提交产生的重复行与现状一致（slug 无唯一约束）。

### F. 附录

术语：**官方** = `is_system=true`（含代码内置与管理员上架，全用户可见）；**私有** = `is_system=false`（仅 creator 可见）；**内置** = `source=code`（sync 维护）；**管理上架** = `source=manual`（人维护，sync 不碰）。前端只依赖 `is_system`，不依赖 `source`。

变更记录：

- 2026-09-30 初稿。
- 2026-09-30 实现：按 §D 三个阶段落地；§3.2 响应换成本地实测；§D 记录验证结果；§5 首条未决问题改为后续迭代。
