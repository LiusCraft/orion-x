# API Key（凭证）模块架构设计

> 状态：已实现（阶段 1 + 阶段 2 合并落地；开关 `apikey.enabled` 默认关闭，灰度开启） | 日期：2026-09-25 | 范围：Manager 进程（`cmd/manager`）+ 控制台（`web/manager`）；数据面（`wsserver`/WS 协议）不在本期 | 关联代码：`internal/apikey/`、`cmd/manager/middleware/apikey.go`、`cmd/manager/middleware/scope_table.go`、`cmd/manager/handler/apikey.go`、`internal/store/api_key.go`、`web/manager/src/pages/ApiKeysPage.tsx`；关联文档：[计费设计](./billing-design.md) §19、[WS 协议设计](./wsserver-protocol-design.md)

## 1. 结论

给 Manager 的 `/api/*` 增加第二种凭证：**用户自助创建的 API Key**，让脚本、CI、第三方集成不再需要借用人格化的 JWT。

一句话定义：**API Key 是一条绑定到某个用户的高熵随机串，服务端只存摘要；校验通过后，它在请求上下文里产出的东西与 JWT 完全一样——一个 `userID`，外加一组可选的 scope。**

关键决策：

| 决策 | 选择 | 一句话理由 | 代价 |
| --- | --- | --- | --- |
| D1 入口范围 | P1 只保护 `/api/*`；WS 握手与未来的 `/v1` 兼容面留扩展点 | 一次动两个进程会把"凭证设计"和"数据面改造"两件事绑在一起，谁出问题都难定位 | 数据面"知道 `device_id` 就能白嫖"的洞（§2 F3）本期不修 |
| D2 主体 | `api_keys.user_id`，**零依赖 `internal/billing`** | 下游（含计费）本来就只认 `middleware.UserID(c)`，复用同一个 seam 即自动打通 | key 泄露 = 账号 API 面暴露，靠 scope + 撤销收窄 |
| D3 串格式 | `ox_sk_<lookup>_<secret>_<crc>`，四段 | 可被扫描器识别、支持 O(1) 查库、打库前就能拒掉畸形输入 | 格式一旦发布不可改；将来只能靠新前缀版本化（`ox_sk2_`） |
| D4 存储 | 只存 `sha256(完整串)` + 明文 `lookup` | 高熵随机值的离线爆破不可行，慢哈希在这里是性能反模式（§4 来源 4/5） | key 丢失不可找回（前端已承诺"仅显示一次"，`ApiKeysPage.tsx:294`） |
| D5 授权 | scope = `resource:action`，deny-by-default，**不支持通配** | 机器凭证没有"角色"概念，缺 scope 比多 scope 安全 | 新增路由必须同步补 scope（用路由覆盖性测试兜住，§7 V3） |
| D6 撤销 | 软删 `revoked_at`，无缓存 → 撤销延迟为 0 | P1 每次校验都是一次索引点查，没有缓存副本可以变旧（多副本后这条更重要：缓存还需要跨副本失效） | 热路径多一次 DB 往返（p99 +1~2ms 量级，口径见 §3） |
| D7 计数 | `last_used_at` / `call_count` 内存聚合、定时刷库 | 与 `internal/billing/client/sink.go` 同一哲学：非关键路径不阻塞请求 | 进程重启会丢一个窗口的计数（口径见 §3，不允许用它对账） |
| D8 限流 | 进程内每 key 令牌桶，藏在 `Limiter` 接口后面；默认值由配置下发 | 现在是单副本，不值得引入 Redis（`go.mod` 无 Redis 依赖） | **多副本是已确定的未来**（Q5）：届时实际配额会变成 副本数 × 配置值，必须先换实现（两条候选路径见 §6 N6） |
| D9 scope 增长 | scope 做成**服务端目录**（分组 / 描述 / 预设），授权判定永远是精确匹配；分组与预设只在**创建时**展开成显式列表 | 未来 scope 会很多，主要成本落在 UI 与"用户能不能理解自己给了什么"，而通配会把最小权限作废 | 目录与预设多一层维护；收紧某条路由的 scope 会成为破坏性变更（§5.3 兼容规则） |

工作量与风险量级：**约 2 个阶段的开发（表 + 领域 + 中间件 + 前端接真数据 ≈ 1 个阶段；全量 scope + 限流 + 撤销/轮换 UI ≈ 1 个阶段）**。主要风险不在技术，在边界：scope 表与路由漂移会造成越权（§6 R2），明文串泄漏进日志会造成凭证流失（§6 R3）。两者都有可执行的机制兜住，不是"注意一下"。

## 2. 背景与目标

### 2.1 现状（带代码位置的事实）

| # | 事实 | 证据 |
| --- | --- | --- |
| F1 | 控制台已经有一版**纯前端 mock** 的 API Keys 页面：5 个中文 scope、`ox_sk_` 假 key、调用次数与最后使用时间，数据来自组件内常量 | `web/manager/src/pages/ApiKeysPage.tsx:24`（scope）、`:36`（`MOCK_KEYS`）、`web/manager/src/App.tsx:108`（路由）；README.md:149 已把它写进"控制台当前覆盖"的能力表 |
| F2 | 后端零实现：`/api/*` 只认 JWT，`internal/store` 里没有任何 key 表 | `cmd/manager/server.go:99-260`、`cmd/manager/middleware/auth.go:18`、`internal/store/db.go:20` 的 AutoMigrate 清单 |
| F3 | 数据面更空：WS 握手的唯一身份是明文 `device_id`，再拿它去**无鉴权**的 `/internal/device-config` 换回整套配置（含厂商密钥）；而 `device_id` 是客户端自己填的 | `internal/channels/xiaozhi/wsproto/protocol.go:98`、`internal/channels/xiaozhi/device_config.go:44`、`internal/store/device.go:13`（主键由调用方传入）、`cmd/manager/handler/device.go:120-122` |
| F4 | 计费链路已就绪，主体枚举是 `user` / `org`，账户按键主体记账 | `internal/billing/status.go:50-51`、`internal/billing/service/resolver.go:37-46`（device → voicebot → OwnerID） |
| F5 | 所有资源归属都是同一句手写比较：`资源.OwnerID == middleware.UserID(c)`；计费的控制面扣费也读同一个 `UserID` | `cmd/manager/handler/device.go:77`、`cmd/manager/handler/mcp.go:358`、`cmd/manager/handler/voice_clone.go:329-334`、`cmd/manager/handler/billing.go:573` |
| F6 | 仓库里没有限流/配额中间件，没有 Redis，也没有 metrics 设施 | `go.mod` 无 redis/prometheus 依赖；`cmd/manager/server.go:46` 只有 `gin.Logger()`（默认不打印请求头） |

### 2.2 为什么不够用

- 第三方集成只能拿用户密码换 JWT（`web/manager/src/lib/api.ts:5-9`：token 存 localStorage）。**机器不能借用人的身份**：无法按集成粒度收窄权限、无法单独吊销、无法追溯"这笔调用是谁发的"。
- 前端已经把这个能力画出来了（F1），但页面是死的——用户看到的 `ox_sk_...` 是假的，任何照着它去接 CI 的人都会撞墙。
- 计费（F4/F5）已经把"谁付钱"这件事收敛到 `user_id` 一个字段上，此刻加凭证，**是唯一不需要动计费的时机**。

### 2.3 目标（可验证）

| 编号 | 目标 | 怎么判定为真 |
| --- | --- | --- |
| G1 | 用户能在控制台创建一把 key，用它在 `curl` 里调 `/api/voicebots` 拿到与 JWT 相同的响应 | 端到端脚本：`curl -H "Authorization: Bearer $KEY" /api/voicebots` 返回 200，与 JWT 结果逐字节一致 |
| G2 | key 的明文只在创建响应里出现一次，服务器与日志再无第二份 | 遍历创建后所有落库字段与日志输出，grep 不到完整串（单测断言，§7 V2） |
| G3 | 撤销后**下一个**请求即 401，不需要重启、不需要等缓存过期 | 集成测试：`DELETE` 后立刻重放请求，断言 401 |
| G4 | 缺失 scope 的 key 调对应路由返回 403，且响应里给出 `required` / `granted` | 表驱动测试覆盖 §5.3 的 scope 表每一行 |
| G5 | 每条 `/api/*` 路由要么标注了 required scope，要么在"key 不可达"白名单里；漏标即测试失败 | 路由覆盖性测试（§7 V3），遍历 `router.Routes()` |

### 2.4 非目标（本期明确不做）

| 不做 | 为什么现在不做 |
| --- | --- |
| 设备凭证 / WS 握手鉴权（修 F3） | 信任模型不同：设备凭证要假设"一定能被提取"（固件、串口日志），需要单设备吊销与轮换；混进账号 key 的表里最容易出的 bug 是"设备 key 能调 `/api/*`"。留到 P2 单独设计（§8 Q4） |
| OpenAI 兼容面的 `/v1/*` 凭证入口 | 依赖 `/v1` 通道本身是否落地；key 表结构已按"将来只加一列"设计（§6 N4） |
| OAuth2 client credentials / 短期 token | 调研结论是"企业客户提出合规要求时再加"（§4 来源 12）；现在上等于为一个还不存在的需求引入一整套授权服务器 |
| key 维度的独立余额 / 预付卡 | 钱仍记在 user 账户上（§4 与 §6 N4）；真做也只是给 key 加一个额度字段，不动主体 |
| 审计事件表、IP 白名单、自动轮换、到期提醒 | 都是"有了真实用量才知道阈值"的东西，列进 §8 |
| 通配 scope（`*`、`agent:*`） | 一次签名就能把未来的新 scope 一并授出去，等于把"最小权限"这个唯一卖点丢掉 |

## 3. 需求与约束

### 3.1 功能需求

| 编号 | 需求 | 判定 |
| --- | --- | --- |
| FR-1 | 创建 key：需要 `name` 与至少一个 scope；返回明文一次 + 元数据 | 创建响应含 `key` 字段；再查列表不含 |
| FR-2 | 列出自己的 key：只能看到所属账号的 key，且只显示 `ox_sk_<lookup>••••` 掩码 | 跨账号列表为空；响应体无完整串 |
| FR-3 | 撤销 key：只能撤自己的；软删，可重复调用（幂等） | 首次 204、再删仍 204；不存在/不属于你 404（契约见 §5.3「HTTP API」表） |
| FR-4 | 用 key 调用资源路由：校验通过后行为与 JWT 完全一致 | G1 |
| FR-5 | scope 授权：deny-by-default；缺 scope → 403 + `required`/`granted`；单条路由可要求**多个** scope（AND） | G4 |
| FR-6 | 每 key 限流：超出 → 429 + `Retry-After` + `X-RateLimit-*` | 压测同 key 并发，第 N+1 个请求 429 |
| FR-7 | 过期：`expires_at` 到点后拒绝，无需人工撤销 | 集成测试用过去时间创建 |
| FR-8 | 使用统计：`last_used_at`、`call_count` | 调 5 次后列表里计数增长（允许一个刷新窗口的延迟） |
| FR-9 | 开关：`apikey.enabled: false` 时管理面 503，且不挂载 key 中间件 | 关闭态下 ① 管理路由 503 ② key 请求 401（与未实现时一致） |
| FR-10 | scope 目录与预设由服务端下发（`GET /api/api-keys/scopes`），前端不硬编码；预设展开在服务端完成 | 改一个 scope 的描述/分组不需要重新构建前端 |

这些需求对应的 **HTTP 契约（方法 / 路径 / 请求响应 / 状态码）集中在 §5.3 的「HTTP API」表**，那是唯一权威。

### 3.2 质量属性（带口径）

| 属性 | 目标 | 口径与条件 |
| --- | --- | --- |
| 校验延迟 | 单次校验 p99 ≤ 2ms（不含业务处理） | 单实例 manager、PG 同机房、`api_keys` ≤ 10 万行、无缓存；此为假设，上线后用实测替换（§7.4 观测项） |
| 撤销时效 | 撤销后第一个请求即失效（0 延迟） | P1 无任何 key 缓存；一旦引入缓存，必须在文档里给出"撤销延迟上限"并同步改写本节 |
| 创建→可用 | 立即（同一事务返回后即可用） | 无异步投递 |
| 计数延迟 | ≤ 60s | `counter_flush` 默认 30s，窗口内计数只在内存 |
| 计数精度 | **非精确**，允许重启丢一个窗口；**禁止**作为计费或对账依据 | 计费用量另有链路（`docs/billing-design.md`），两者的口径不得混用 |
| 成本 | 新增外部依赖 0，新增付费组件 0 | 限流与计数都在进程内 |
| 容量假设 | 单账号 key 数 ≤ 100；全平台 key ≤ 10 万；**scope 目录 ≤ 200 条且只增** | 假设，上线后观测；超限时收紧创建配额，不引入分片与位图（§5.4） |

### 3.3 约束

- **技术**：Go 1.26 + Gin + GORM/PostgreSQL；不引入新依赖（`go.mod` 现状即上限）；表结构走 `internal/store/db.go:20` 的 `AutoMigrate` 一次性追加。
- **架构**：`internal/apikey` 不 import `internal/billing`，反向也不成立；计费领域层的 `depguard` 规则（`.golangci.yml` 的 `billing-domain`）只匹配 `internal/billing/*.go`，本模块不受它约束，但**自愿遵守同样的零依赖纪律**。
- **兼容**：不修改任何既有 JWT 路由的语义；`middleware.UserID(c)` 的键名（`cmd/manager/middleware/auth.go:13`）保持不变，既有 handler 一行不改。
- **规范**：key 串用 `_` 分段（对齐 GitHub/Stripe，且下划线能让双击选中整串）；落库的**标识值**（`id`、scope 名、`revoke_reason`）仍按 AGENTS.md 用 `:` 分段。
- **前端约束**（AGENTS.md "前端是客户界面"）：界面文案不得出现 `scope`、表名、包名等实现词；服务端的 `{error}` 用 `web/manager/src/lib/billing.ts` 同款 `userFacingError` 兜住，不通盘直出。

## 4. 业界调研

检索角度：① key 串格式与存储口径（GitHub/Stripe/Seam + 安全指引）② 泄漏事故与轮换响应（GitGuardian/Truffle Security）③ 设备凭证 vs 账号凭证（OWASP ISVS、nRF Cloud、AWS IoT、Azure/ClearBlade）④ scope 与每 key 限流（Layers、规模化设计文）。

| 做法 | 可借鉴什么 | 为什么不直接用 | 来源 |
| --- | --- | --- | --- |
| GitHub token：`ghp_` 等 3 字母类型前缀 + `_` 分隔 + 末 6 位 CRC32（Base62） | 前缀 + 分隔符 + 校验段的三段式；校验段的两个收益：扫描器离线判伪、服务端不打库就拒畸形输入 | 它的多前缀（`ghp`/`gho`/`ghu`…）是为区分 token 类型，我们只有一种凭证，一个 `ox_sk_` 足够 | [GitHub Blog](https://github.blog/engineering/platform-security/behind-githubs-new-authentication-token-formats/)（2021-04-05，访问 2026-09-25） |
| Stripe：`sk_`/`rk_`/`pk_` + `live`/`test`；官方推荐用受限密钥（RAK）替代无限制的 secret key；支持 IP 白名单 | "受限密钥"就是我们的 scope；密钥分类让人一眼知道自己拿的是什么 | 我们没有 publishable key，也没有 test/live 双环境（README 无此概念），格式里塞环境段只会制造"看着像但其实不生效"的字段 | [Stripe Docs: API keys](https://docs.stripe.com/keys)（访问 2026-09-25） |
| Seam `prefixed-api-key`：`prefix_shortToken_longToken`，服务端只存 `hash(longToken)`，用 `shortToken` 做索引 | 两段式：**可查的公开段** + **不可存的秘密段**；短段还能用于日志与客服排障 | 引入一个 npm 库不值得，我们自己写这 40 行更容易审 | [seamapi/prefixed-api-key](https://github.com/seamapi/prefixed-api-key)（访问 2026-09-25） |
| Zuplo / apikeys.guide：API key 用 SHA-256 存，**不要** bcrypt/argon2；存明文前缀做 O(1) 查库；常量时间比较；缓存 TTL 与撤销延迟是同一个旋钮 | 校验三步（形状 → 索引查 → 常量时间比对）；"缓存 TTL = 撤销延迟上限"这个口径 | 它们的 edge 多副本缓存前提（我们在 §6 N3 里明确否决，原因是我们只有一个副本） | [Zuplo: API key best practices](https://zuplo.com/blog/api-key-best-practices.md)、[apikeys.guide: Hashing & Storage](https://apikeys.guide/docs/security/hashing-and-storage)（2026-05-05，访问 2026-09-25） |
| OWASP Password Storage Cheat Sheet：慢哈希（argon2id/bcrypt/PBKDF2）的适用前提是**低熵、用户自选**的密码 | 反过来解释我们为什么不用我们已有的 bcrypt（`cmd/manager/handler/auth.go:52`）：API key 是 190 bit 的服务端随机串，不存在字典攻击面 | 该清单本身不针对 API key，直接照搬会带来每请求数十毫秒（bcrypt 默认 cost 的单次校验量级）的开销 | [OWASP Cheat Sheet Series](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)（访问 2026-09-25） |
| Truffle Security：0.1% 的 GitHub push 含有效密钥；故意发布的 AWS key **10 分钟内**被使用；删除代码不等于修复，必须轮换 | 撤销必须是即时且不可逆的；每把 key 必须能单独吊销 | 事件响应流程（rotating checklist）不是模块设计，不进本文 | [Truffle Security](https://trufflesecurity.com/blog/remediate-leaked-api-keys-with-key-rotation)（2023-08-03，访问 2026-09-25）、[GitGuardian](https://blog.gitguardian.com/leaking-secrets-on-github-what-to-do/)（2022-12-20，访问 2026-09-25） |
| nRF Cloud：应用用 **API key 覆盖一个 team 的全部设备**；设备用 **device-specific JWT** 且只能访问更小的一组端点；AWS IoT/Azure IoT 用每设备 X.509/SAS，支持多把并发密钥做无中断轮换 | 账号凭证与设备凭证分家，且设备凭证粒度更细、权限更小——这正是 §2.4 把设备凭证排除在本期的外部依据 | 他们给设备发**非对称**凭证（需要设备侧密钥库/安全芯片，OWASP ISVS 2.4.4 甚至要求专用安全芯片）；我们的客户端形态（浏览器/ESP32 固件）暂不假设有安全存储，P2 从随机串起步 | [nRF Cloud: Security and credentials](https://docs.nrfcloud.com/docs/legacy-nrfcloud/device-security.md)、[OWASP ISVS V2](https://owasp.org/IoT-Security-Verification-Standard-ISVS/en/V2-User_Space_Application_Requirements.html)（2.1.9/2.1.10/2.4.1）、[AWS IoT](https://docs.aws.amazon.com/iot/latest/developerguide/authorizing-direct-aws.html)（访问 2026-09-25） |
| Layers：scope 命名 `resource:action`；**deny-by-default，空 scope 什么都干不了**；不可委托的 `org:admin`；每 key 的 tier 与 `X-RateLimit-*` 响应头 | scope 命名与错误语义；把"越权的钥匙"（管钱、管组织）单独做成不可通配的一类 | 它的通配符（`*`、`ads:*`）我们不采用：通配把"最小权限"卖点作废 | [Layers docs: API keys](https://layers.com/docs/api/concepts/api-keys)（访问 2026-09-25） |
| 规模化 API key 设计文：每 key 一条独立令牌桶（按 key 分片，而不是按租户）；scope 检查要落在 handler 层而不只是认证中间件；403 响应带 `required`/`granted` | 限流按 key 而不按用户（一个失控脚本不该拖垮同一账号的另一条集成）；403 的可诊断性 | 它们的 Redis 分片计数器假设多副本 + 高 QPS（我们单副本，见 §6 N6） | [scalablesystem.dev](https://scalablesystem.dev/system-design/build-an-api-key-management-system)、[Let's Build: API auth for B2B SaaS](https://letsbuildsolutions.com/blog/web-engineering/designing-api-authentication-for-b2b-saas-api-keys-oauth-client-credentials-and-scoped-access-tokens/)（2026-04-18，访问 2026-09-25） |
| Safeguard.sh：把"要不要结构化的 key 串"退化成一个判据——**凭证会不会离开你的组织**；会离开（被贴进客户的仓库、CI、脚本）就值得前缀/校验段/可扫描性，不会离开就是过度设计；另外点名 `last_used` 是访问复盘能否进行的前提 | 判据本身：我们的 key 必然被用户贴进他们自己的流水线，所以结构化的收益成立；`last_used_at` 不是"好看"的字段，是让"这把 key 还有没有人用"可回答的字段 | 它是格式设计综述，不涉及限流、存储选型与多副本缓存，不能作为那些决策的依据 | [Safeguard.sh: API Key Format Design](https://safeguard.sh/resources/blog/designing-an-api-key-that-can-be-found-when-it-leaks)（2026-09-18，访问 2026-09-25） |

**检索里被剔除的来源**：搜索命中了若干"某 SDK 泄露造成 24 万美元云费用"之类的复盘文章（如 johal.in 的两篇），它们数字具体、但没有任何一手出处可交叉验证，且同类叙述在多个站点重复出现——疑似 SEO 生成内容，不作为依据。真正有信息量的两条事实（发布即被用、删除代码不等于修复）由 Truffle Security 的一手实验支撑，已列入上表。

**调研结论落到哪个决策**：格式三段式 + 校验段 → D3；SHA-256 而非慢哈希、前缀索引查库 → D4；即时撤销是硬需求 → D6/G3；账号凭证与设备凭证分家 → §2.4 与 §8 Q4；scope 命名与 deny-by-default、每 key 限流、403 可诊断 → D5/D8/FR-5。

## 5. 模块与关系

### 5.1 模块划分

| 模块 | 职责（一句话） | **明确不负责** | 依赖谁 | 落点 |
| --- | --- | --- | --- | --- |
| 领域与生成 | key 串的生成/解析/摘要、scope 目录与预设、授权判定、错误语义、限流与计数聚合 | 不碰 HTTP、不碰 gin、不查库；**不 import `internal/billing`**；不认识账号余额；不自己决定该不该拒绝某条路由（那是路由→scope 表的事） | 标准库 | `internal/apikey/`（`key.go` / `scope.go` / `limiter.go` / `counter.go` / `service.go`） |
| 仓储 | `api_keys` 表的 CRUD 与索引 | 不做校验、不做权限判断（拿到什么存什么） | `internal/store` 既有模式 | `internal/store/api_key.go` |
| 认证中间件 | 解析 `Authorization` → 产出 `userID` / scope 集合 / `keyID`；与 JWT 分流 | 不做具体权限判断（只产出事实）；不打印凭证；不产生 `isAdmin` | `internal/apikey` | `cmd/manager/middleware/apikey.go` |
| 路由→scope 表 | 中央声明"哪条路由要哪个 scope"，并提供覆盖性校验函数 | 不参与认证；不认识用户 | `internal/apikey` 的 scope 常量 | `cmd/manager/middleware/scope_table.go` |
| 管理 API | 创建/列表/撤销自己的 key；只回明文一次 | 不提供"查看明文"；不做管理员代管他人 key（P1） | `internal/apikey` | `cmd/manager/handler/apikey.go` |
| 控制台页面 | 创建（展示一次 + 复制）、列表（掩码/scope/计数/最后使用）、撤销；scope 中英映射与预设组合 | 不出现实现词；不自己伪造 key（删掉 `MOCK_KEYS`） | Manager API | `web/manager/src/pages/ApiKeysPage.tsx` |

判据自查：每个模块只有一个变化原因——加一种凭证（P2 设备凭证）改领域与仓储；改权限粒度改 scope 表；改 UI 只改页面。

### 5.2 依赖与时序图

```mermaid
graph TD
    FE["web/manager ApiKeysPage"] -->|"/api/api-keys"| H["cmd/manager/handler apikey.go"]
    H --> SVC["internal/apikey Service"]
    SVC --> ST["internal/store api_key.go"]
    MW["cmd/manager/middleware apikey.go + scope_table.go"] --> SVC
    ROUTE["/api/* 资源路由"] --> MW
    MW -.->|"c.Set('userID')"| DOWN["既有 handler 读 middleware.UserID(c)"]
    DOWN -.->|"同一个 user_id"| BILL["internal/billing Meter / Authorize"]
```

图的要点：**实线是 import，虚线是运行期共享的标识**。`internal/apikey` 与 `internal/billing` 之间刻意没有画出任何边——它们只在 `user_id` 这个字符串上相遇，谁都不 import 谁；`cmd/manager/middleware` 是唯一把凭证翻译成身份的地方（`c.Set("userID", ...)`，键名沿用 `middleware/auth.go:13`）。

成功路径（创建 → 使用）：

```mermaid
sequenceDiagram
    participant U as 控制台/用户
    participant H as handler(apikey)
    participant S as apikey.Service
    participant DB as PostgreSQL
    participant R as /api/* 路由
    U->>H: POST /api/api-keys {name, scopes}
    H->>S: Create(userID, name, scopes)
    S->>S: 生成 lookup + secret + crc，算 sha256
    S->>DB: INSERT api_keys(hash, lookup, scopes, ...)
    S-->>U: 明文 key（唯一一次）+ 元数据
    Note over U: 用户存进 CI secret
    U->>R: GET /api/voicebots（Bearer ox_sk_...）
    R->>S: Authenticate(raw)
    S->>S: 形状校验（前缀/长度/CRC，不打库）
    S->>DB: SELECT ... WHERE lookup = ?
    S->>S: 常量时间比对 hash；检查 revoked/expires
    S-->>R: Identity{keyID, userID, scopes}
    R->>R: scope 表命中？限流？
    R->>R: 业务 handler（读 middleware.UserID）
```

失败路径（限流 + 撤销，两条最需要说清"谁清理什么"的路径）：

```mermaid
sequenceDiagram
    participant U as 客户端
    participant MW as 认证+scope+限流
    participant DB as PostgreSQL
    participant BG as 计数聚合 goroutine
    Note over U,BG: 路径 A：限流拒绝
    U->>MW: 第 N+1 个请求（同 key）
    MW->>MW: 令牌桶取不到令牌
    MW-->>U: 429 + Retry-After + X-RateLimit-Limit/Remaining/Reset
    Note over MW,DB: 不打库、不计入 last_used/call_count（它没走到业务层）
    Note over U,BG: 路径 B：撤销 → 立刻失效
    U->>MW: DELETE /api/api-keys/:id
    MW->>DB: UPDATE api_keys SET revoked_at=now()
    MW->>BG: 清掉该 key 在内存里的限流桶与待刷计数
    MW-->>U: 204
    U->>MW: 重放刚才的业务请求
    MW->>DB: SELECT ... WHERE lookup = ?
    DB-->>MW: revoked_at != NULL
    MW-->>U: 401（无缓存，无需等待）
```

### 5.3 接口与契约

领域层（`internal/apikey`，不 import gin）：

```go
const Prefix = "ox_sk_"        // key 串的公开前缀，改它等于换一代凭证

// Generate 生成明文（仅此一次返回）与可落库的记录。
func Generate(userID, name string, scopes []string, expiresAt *time.Time) (plaintext string, key store.APIKey, err error)

// Parse 只做形状校验：前缀、分段、字符集、长度、CRC。不打库。
func Parse(raw string) (lookup, secret string, ok bool)

// Digest 返回落库用的摘要（sha256 hex）。
func Digest(raw string) string

// Identity 是校验成功后交给上层的全部事实。
type Identity struct {
    KeyID  string
    UserID string
    Scopes []string
}

type Service struct{ /* store + limiter + counter */ }

// Create / List / Revoke：用户自助管理，全部按 userID 过滤。
func (s *Service) Create(ctx context.Context, userID, name string, scopes []string, expiresAt *time.Time) (string, *store.APIKey, error)
func (s *Service) List(ctx context.Context, userID string) ([]store.APIKey, error)
func (s *Service) Revoke(ctx context.Context, userID, keyID string) error

// Authenticate 是热路径：形状 → 查库 → 常量时间比对 → 过期/撤销判定。
func (s *Service) Authenticate(ctx context.Context, raw string) (Identity, error)

var (
    ErrInvalid  = errors.New("apikey: invalid key")   // 形状不对或查不到
    ErrRevoked  = errors.New("apikey: key revoked")
    ErrExpired  = errors.New("apikey: key expired")
)
```

key 串的形状（实现逐字对齐，改了就是换一代凭证）：

```text
ox_sk_<lookup:10>_<secret:32>_<crc:6>
       └─公开段─┘  └─秘密段─┘  └校验┘
```

- 字母表统一用 **base58（Bitcoin）**：去掉 `0OIl` 等易混字符，人工抄写 / 电话报 key 时少出错。代价是每字符熵略低（≈ 5.86 bit），32 字符的秘密段 ≈ **187 bit**，远超所需的 128 bit（§3.2）。
- `crc = base58(CRC32-IEEE(lookup + "_" + secret))`，取低 32 位、左补零到 6 字符。它只用于**不打库就拒掉畸形输入**与让外部扫描器少报误判，不承担安全职责。
- `Digest` 取**整串**（含前缀与校验段）的 SHA-256：校验段也被哈希盖住，改一个字符必然失败。
- 取整串哈希也意味着**日志里出现任何形如 `ox_sk_...` 的完整串都是事故**（R3）。

中间件（`cmd/manager/middleware`）：

```go
// Auth 取代资源路由上原来的 JWT 中间件：按前缀在 JWT 与 API key 之间分流，
// 两条路都只做一件事——把 userID（key 另加 scopes/keyID）放进上下文。
// 关键约束：API key 这条路**永远不设置 isAdmin**，因此 RequireAdmin 天然挡住它。
func Auth(jwtSecret []byte, keys *apikey.Service) gin.HandlerFunc

// RequireScopes 从中央路由→scope 表取出本条路由要求的 scope 集合（AND），按 deny-by-default 判定。
func RequireScopes(table ScopeTable) gin.HandlerFunc

func Principal(c *gin.Context) string // "jwt" | "api_key"
func KeyID(c *gin.Context) string
```

#### HTTP API（本期新增的端点）

本期**只新增管理面 4 个端点，全部只接受 JWT**；既有资源路由不新增任何端点，只是多接受一种凭证；数据面（`wsserver`）与 `/internal/*` 零改动（不新增内部端点）。

**端点索引**（请求体字段与状态码语义看下面的例子，这里只当导航）：

| 方法 | 路径 | 谁可调 | 干什么 | 状态码 |
| --- | --- | --- | --- | --- |
| POST | `/api/api-keys` | JWT | 创建一把 key，返回明文一次 | 201 / 400 / 401 / 503 |
| GET | `/api/api-keys` | JWT | 列出自己的 key（只有掩码） | 200 / 401 / 503 |
| DELETE | `/api/api-keys/:id` | JWT | 撤销（幂等） | 204 / 401 / 404 / 503 |
| GET | `/api/api-keys/scopes` | JWT | scope 目录与预设（前端渲染表单用） | 200 / 401 / 503 |

**① 创建 —— 明文只在这一条响应里出现**

```bash
curl -X POST https://manager.example.com/api/api-keys \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"生产环境 CI","scopes":["agent:read","model:read"],"expires_at":"2027-01-01T00:00:00Z"}'
```

```json
201 Created
{
  "key": "ox_sk_7Qm2Vx4bTk_9fJ3aPz0Lq8Rw5Nc1Yh6BdE2Gs4UmXk_8hKq2P",
  "api_key": {
    "id": "0b0e5c3a-6b1f-4a3d-9f2e-7c8d1a4b5e90",
    "name": "生产环境 CI",
    "masked_key": "ox_sk_7Qm2Vx4bTk••••",
    "scopes": ["agent:read", "model:read"],
    "expires_at": "2027-01-01T00:00:00Z",
    "revoked_at": null,
    "last_used_at": null,
    "call_count": 0,
    "created_at": "2026-09-25T09:12:00Z"
  }
}
```

`key` 之后再也拿不到（G2）——丢了只能重建；`scopes` 可以是服务端预设展开后的列表（§5.3 目录与预设）。

**② 列表 —— 永远只有掩码**

```bash
curl "https://manager.example.com/api/api-keys?page=1&page_size=20" \
  -H "Authorization: Bearer $JWT"
```

```json
200 OK
{
  "data": [
    {
      "id": "0b0e5c3a-6b1f-4a3d-9f2e-7c8d1a4b5e90",
      "name": "生产环境 CI",
      "masked_key": "ox_sk_7Qm2Vx4bTk••••",
      "scopes": ["agent:read", "model:read"],
      "expires_at": "2027-01-01T00:00:00Z",
      "revoked_at": null,
      "last_used_at": "2026-09-25T10:03:11Z",
      "call_count": 284,
      "created_at": "2026-09-25T09:12:00Z"
    }
  ],
  "total": 1,
  "page": 1
}
```

`last_used_at` / `call_count` 可能落后一个刷新窗口（≤ 60s），且 `call_count` 非精确（§3.2）。

**③ 撤销 —— 幂等**

```bash
curl -X DELETE https://manager.example.com/api/api-keys/0b0e5c3a-6b1f-4a3d-9f2e-7c8d1a4b5e90 \
  -H "Authorization: Bearer $JWT"
```

```text
204 No Content
```

再删一次仍 204（行还在，只是终态）；不存在或不属于你都是 404，两者同形不提供枚举面：

```json
404 Not Found
{"error":"api key not found"}
```

**④ scope 目录与预设 —— 控制台就靠它渲染表单**

```bash
curl https://manager.example.com/api/api-keys/scopes -H "Authorization: Bearer $JWT"
```

```json
200 OK
{
  "scopes": [
    {"value":"agent:read","group":"agent","title":"查看智能体","desc":"列出与读取智能体配置","default":true,"deprecated":false},
    {"value":"agent:write","group":"agent","title":"管理智能体","desc":"创建、修改、删除智能体与设备","default":false,"deprecated":false},
    {"value":"model:write","group":"model","title":"管理模型与音色","desc":"增删改供应商/模型/音色；含音色克隆，会产生费用","default":false,"deprecated":false}
  ],
  "presets": [
    {"name":"readonly","title":"只读","scopes":["agent:read","model:read","data:read","billing:read"]}
  ]
}
```

注意 `model:write` 的 "会产生费用" 只在 `desc` 文案里，没有结构化字段（§5.3 目录与预设）。

**用 key 调既有资源路由（不是新端点，只是换了凭证）**

```bash
curl https://manager.example.com/api/voicebots \
  -H "Authorization: Bearer ox_sk_7Qm2Vx4bTk_9fJ3aPz0Lq8Rw5Nc1Yh6BdE2Gs4UmXk_8hKq2P"
```

成功响应与 JWT **逐字节一致**（G1）。四种失败：

```json
401 Unauthorized
{"error":"api key revoked"}
```

```json
403 Forbidden
{"error":"insufficient scope","required":["agent:read"],"granted":["data:read"]}
```

```text
429 Too Many Requests
Retry-After: 1
X-RateLimit-Limit: 20
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1758795120
```

```json
{"error":"rate limit exceeded"}
```

| 管理面还有三类 400 | 触发 |
| --- | --- |
| `{"error":"name is required"}` / `{"error":"at least one scope is required"}` | 名字为空 / `scopes` 为空（deny-by-default 下空 scope 等于造一把废钥匙） |
| `{"error":"unknown scope","unknown":["agent:blah"]}` | 带了不认识的 scope——**不静默丢弃**，否则用户以为授了权 |
| `{"error":"expires_at must be in the future"}` | 时间已过 |

关闭态（`apikey.enabled: false`，FR-9）四个端点全回 `503 {"error":"api keys are disabled"}`。

`api_key` 对象（列表与创建响应同形，**永不含明文**）：

| 字段 | 类型 | 语义 |
| --- | --- | --- |
| `id` | string | 用于 `DELETE` |
| `name` | string | 用户自取；**允许重名**（名字是标签不是身份） |
| `masked_key` | string | `ox_sk_<lookup>••••`，列表里唯一的 key 展示 |
| `scopes` | string[] | 创建时已展开的显式列表 |
| `expires_at` | string 或 null | RFC3339 |
| `revoked_at` | string 或 null | 非空 = 已撤销（终态） |
| `last_used_at` | string 或 null | 可能落后一个刷新窗口（≤ 60s，§3.2） |
| `call_count` | int | 非精确（§3.2），不得用于对账 |
| `created_at` | string | RFC3339 |

**为什么这四个端点不给 key 用**（本节最要紧的一条约束）：

一把泄漏的 key 如果能**创建** key，撤销原 key 就挡不住攻击者——他在被拆穿前先造一把新的、写进自己的脚本，形成**持久化后门**（原 key 撤销后新 key 依然有效，而你可能永远不知道）。所以规则是：

- **凭证不能自我复制**。管理面（含 scope 目录）只接 JWT；`/api/api-keys*` 进"key 不可达"白名单（结构保证，不靠 scope）。
- 代价：CI 不能自助轮换 key，要人去控制台点一下。P1 接受。将来若要自动化轮换，正确做法是短时效的轮换凭据或把轮换交给 JWT 身份，**而不是让 key 能造 key**。
- 管理面不参与每 key 限流（那是资源请求的事）；它的鉴权就是现有的 JWT 中间件（`cmd/manager/middleware/auth.go:18`）。

按仓库约定，这 4 个端点要在 `cmd/manager/swagger.go` 里加注解并跑 `make swagger`（生成物入 `docs/manager/`，见 `Makefile:59-61`）——忘了这步，Swagger 与文档站就停在旧版。

调用方拿到这些响应必须怎么做（响应的字面形式见上面四条 curl）：

- **401 `invalid api key` / `api key revoked` / `api key expired`**：停止重试——这是配置问题，不是瞬时故障；后两种要去控制台换新 key。
- **403 `insufficient scope`**：改请求或换一把 key，**不要**盲目重试；`required` / `granted` 直接告诉你差哪一项。
- **429**：按 `Retry-After` 退避；`X-RateLimit-*` 可用于自适应限速。

区分"撤销/过期"与"查不到"是有意的：key 的主人就是我们的用户，排障信息比"不泄露 key 存在性"更值钱（另一种做法是统一 401，代价见 §6 N7）。

并发与所有权：

- `Identity` 一旦从校验函数返回即视为不可变，随 gin 上下文在单次请求内传递，不跨请求缓存。
- 限流桶与计数聚合各持一把 mutex（`internal/apikey` 内部）；热路径除那一次索引查询外不做任何 IO。
- 计数聚合的刷库由进程级单个 goroutine 完成，生命周期跟随 manager 的 root context；manager 退出时最多丢一个窗口（§3.2 已认账）。

兼容规则：

- API 响应字段只增不改；`scopes` 是开放枚举，新增 scope 不改变既有 key 的行为（它们拿不到新 scope）。
- **不支持通配**：将来新增 scope 时，所有既有 key 都不会自动获得它——这是特性不是缺陷。
- **收紧是破坏性变更**：把某条路由的 required scope 从 A 改成 B（或把某个 scope 拆成两个）会让所有只持有 A 的 key 开始 403。必须走公告 + 过渡期：旧 scope 停止授予新 key，但继续在旧路由上放行，直到旧路由下线。
- **scope 值一经发布不得改变含义**，只能废弃（不再挂到新路由/不再进预设）——否则历史 key 的授权面会静默变化。
- **命名深度**：默认两段 `resource:action`；当一个资源家族的动作超过约 15 个、或需要把有副作用（含会扣费）的动作从写里拆出来时，才允许第三段（`resource:action:sub`，如 `data:knowledge:write`）。不允许超过三段：再深就该把它做成新资源。
- 前端按未知 scope 原样展示（不隐藏），保证"服务端先上、前端后补文案"时不会丢信息。

#### scope 表（P1 全量）

| scope | 覆盖路由（组） | 说明 |
| --- | --- | --- |
| `agent:read` | `GET /api/voicebots*`、`GET /api/agent-templates/*` | 看智能体与广场模板 |
| `agent:write` | 智能体的增删改、`POST /api/agent-templates/:id/use` | 改配置、从模板建智能体 |
| `device:read` | `GET /api/voicebots/:id/devices` | 看设备与 TG 通道状态（不回 token） |
| `device:write` | 设备的增删、TG 通道设置/删除 | |
| `model:read` | `/api/providers`、`/api/models`、`/api/voices/*`、`/api/available-resources`、`/api/languages` 的读 | 供应商/模型/音色 |
| `model:write` | 上述资源的增删改、`POST /api/models/:id/voices/clone` | 音色克隆会**扣费**（`handler/voice_clone.go:329`）——只有 `model:read` 的 key 触发不了它 |
| `data:read` | `/api/data/memory/*`、`/api/data/knowledge/*` 的读、`/api/assets` 读、`GET /api/sessions` | 记忆、知识库、资源 |
| `data:write` | 记忆删除、知识库/文档上传与删除、KB 绑定、资源上传与删除 | |
| `mcp:read` | `/api/mcp/market`、`/api/mcp/servers` 读 | |
| `mcp:write` | MCP server 增删改、voicebot 的 MCP 绑定 | 绑定关系按"被改动的资源"归到 mcp |
| `mcp:call` | `/api/mcp/test-connection`、`/api/mcp/list-tools`、`/api/mcp/call-tool` | 会真的执行外部工具，可能有副作用与成本，**不与 `mcp:write` 合并** |
| `billing:read` | `/api/billing/summary`、`/api/billing/usage`、`/api/billing/usage-by-model`、用户面 `GET /api/billing/prices` | 只读余额与用量 |

**key 不可达的路由（结构保证，不靠 scope）**：`/api/api-keys*`（**凭证不能自我复制**——泄漏的 key 若能造 key 就是持久化后门，见 §5.3 HTTP API）、`/api/auth/*`（改密码、绑邮箱、解绑 OAuth——账号安全不接受机器凭证）、`RequireAdmin` 的管理端（key 永不产生 `isAdmin`）、`/api/billing/recharge*`（变更余额的动作）、`/pay/epay/*`、`/internal/*`、`/healthz` 与文档路由。

#### scope 目录与预设（应对 scope 变多）

scope 会持续增加，所以"有哪些 scope、它们叫什么、怎么分组、会做什么"不能散在代码与前端里，必须有一份**服务端单一事实源**：

```go
// internal/apikey/scope.go：目录是编译期常量表，路由与前端都从它取。
type ScopeInfo struct {
    Value      string // "agent:write"，唯一标识，一经发布不变
    Group      string // "agent"，只用于展示分组与批量选择（不参与授权判定）
    Title      string // 中文短标题，控制台展示
    Desc       string // 一句话说明"授予后会发生什么"（要提示会扣费就写在这里）
    Default    bool   // 是否进默认预设
    Deprecated bool   // 不再挂到新路由，但历史 key 仍可持有（兼容规则要求的过渡态）
}

// Presets 是服务端定义的预设。预设**在创建时展开成显式 scope 列表落库**，
// 不存"预设名"：否则预设定义一改，历史 key 的权限会静默变。
var Presets = map[string][]string{ "readonly": {...}, "integration": {...} }
```

三个约束把"很多 scope"限制在展示层，不让它污染授权语义：

1. **授权判定永远是精确匹配**，不认通配、不认分组、不认预设名——只有落库的那串显式 scope 算数。
2. **分组与预设只在创建时展开**：UI 上勾一个组，请求里发的是展开后的完整列表；服务端不自己展开组名。
3. **目录只增不改**：改分组/标题/描述随时可以；改 `Value` 或它的含义不行（见上面的兼容规则）。

还有一条边界：**目录只管授权语义，不管钱**。"这个动作会扣费"不是 scope 的属性——计费可以整个关掉（`billing.enabled: false` 时一分钱不花），价格也会变，把它做成 `Costs bool` 等于把计费事实复制进凭证目录，而且从写下的那天就开始过期。要提示就写进 `Desc` 的人话里（如 `model:write` 的描述写"含音色克隆，会产生费用"）；真正的权威在计费侧（`internal/billing/item.go` 的 item 目录与价格行）。将来若真要做准确的"会扣费"徽标，那是控制台渲染时去问价格表的事，不是这个目录的字段。

前端的分组、批量选择、搜索、以及描述里的提示语，全部读 `GET /api/api-keys/scopes`（FR-10）；目录变大时控制台不需要发版。

### 5.4 数据结构与存储

领域模型 `store.APIKey`（表名由 `TableName()` 钉死为 `api_keys`，遵循 `internal/store/asset.go:24` 的写法）：

| 字段 | 类型 | 含义与约束 |
| --- | --- | --- |
| `id` | varchar(36) PK | UUID；API 路径里用它，不复用 lookup |
| `user_id` | varchar(36) not null, index | **唯一的主体列**；不可变（不提供改归属的 API） |
| `name` | varchar(64) not null | 用户自取的名字，展示用 |
| `lookup` | varchar(16) not null, **unique** | 串里的公开段，索引查找用；撤销后不复用 |
| `hash` | varchar(64) not null, unique | `sha256(完整串)` 的 hex |
| `scopes` | `pq.StringArray` not null | 至少一个元素（服务层校验）；用仓库既有的 text[] 写法（`internal/store/agent_template.go:59`）；**不做位图**——位图只在百万 key × 每秒二十万次校验的规模才划算（§4 来源 12），而 scope 目录变多不改变这一点（一个 key 的 scope 数仍在几个到几十个之间） |
| `expires_at` | timestamptz null | 空 = 不过期 |
| `revoked_at` | timestamptz null | 空 = 有效；非空 = 终态 |
| `last_used_at` | timestamptz null | 异步刷 |
| `call_count` | bigint not null default 0 | 异步刷，非精确（§3.2）；多副本下由各副本**追加增量** |
| `BaseModel` | — | `created_at`/`updated_at`/`creator`（谁创建的，审计用） |

存什么、不存什么：**只存摘要与公开段，不存明文、不存密钥的加密副本**。这决定了"忘了就重建"的产品语义。

**多副本安全的刷库写法**：计数与最后使用时间必须写成**增量与取最大**，不能写成绝对值——

```sql
UPDATE api_keys SET call_count = call_count + ?, last_used_at = GREATEST(last_used_at, ?) WHERE id = ?
```

Q5 已确认未来会多副本。单副本时这两种写法行为相同，但选增量写法让扩副本**不需要改代码**，也避免了"后写的副本把先写的增量覆盖掉"。

状态机：

```mermaid
stateDiagram-v2
    [*] --> Active: Create（hash + lookup + scopes 落库）
    Active --> Revoked: DELETE（revoked_at = now）
    Active --> Expired: 时间越过 expires_at（无需写库，读时判定）
    Revoked --> [*]
    Expired --> [*]
```

非法迁移的处理：`revoked_at` 一旦写入不再清空（没有"恢复"接口）；对已撤销的 key 再次 `DELETE` 为幂等成功；对已过期的 key 调 `DELETE` 仍写 `revoked_at`（列表里显示"已撤销"比"已过期"对用户更明确）。**没有"停用/启用"中间态**——它会让状态机多两条边和一个"停用了但列表里看起来还在"的灰色体验，若确需，见 §8 Q2。

不变量（永远为真 + 由谁保证）：

| 不变量 | 由谁保证 |
| --- | --- |
| 明文串只存在于创建响应与用户手里 | 代码：没有第二个返回明文的入口；仓储只有 `hash`/`lookup` 两列 |
| `lookup` 全局唯一 | 数据库唯一索引 + 创建冲突重试 |
| `hash` 唯一 | 数据库唯一索引（碰撞概率可忽略，撞了就是重试） |
| 有效 key 的 scope 集合非空 | 服务层创建校验 + 表驱动测试 |
| 已撤销/已过期 ⇒ 校验必然失败 | 单一校验函数（`Authenticate`）+ 集成测试；不允许其它路径自行判断有效性 |
| key 的 `user_id` 不变 | 不提供改动接口；仓储不暴露 `Update(user_id)` |
| 多副本下 `call_count` 等于各副本增量之和（允许漏数，不允许重数或回退） | 增量 SQL（上面的 `+=` / `GREATEST` 写法）+ 行级并发由 PG 保证 |

迁移与保留：`AutoMigrate` 追加一张表，无数据迁移；软删意味着行会累积，P1 不清理（10 万行量级对 PG 无压力），保留策略进 §8。

## 6. 技术难点与取舍

| 编号 | 难点 | 候选方案 | 决策 | 理由 | 代价 | 怎么验证 |
| --- | --- | --- | --- | --- | --- | --- |
| N1 | key 串要不要结构 | ① 纯随机串 ② 前缀 + 可查段 + 校验段（GitHub/Seam 式） | ②（D3） | ①在打库前无法拒绝畸形输入，也无法被任何扫描器识别；②的每一项都有明确消费者（前缀→扫描器与排障、lookup→O(1) 查库、CRC→免打库拒伪） | 格式出版即冻结，只能靠新前缀版本化 | 单测覆盖：两段长度/字符集/CRC 的边界；对随机串与"合法串改一位"做批量测试，CRC 必须拒绝全部单点改动 |
| N2 | 存储形态 | ① 明文 ② sha256 ③ bcrypt/argon2 ④ AES 加密可检索 | ②（D4） | 190 bit 熵下离线爆破不可行（§4 来源 4/5）；②还能被唯一索引加速；我们用 bcrypt 存的是**人选的**密码，两者威胁模型不同 | 忘记即不可恢复；DB + 应用配置同时泄露时摘要也可能被撞（概率极低） | 单测：库里 grep 不到明文；比对函数用常量时间（`subtle.ConstantTimeCompare`） |
| N3 | 校验路径要不要缓存 | ① 每次查库 ② 进程内缓存 + TTL ③ 缓存 + 跨副本失效广播 | ①（D6） | 单副本时，缓存只买到"少一次索引查询"，却把"撤销延迟"从 0 变成 TTL；**Q5 已确认会多副本，这会让缓存多一个跨副本失效问题**——不缓存不仅现在对，扩副本后更对 | 热路径多一次 DB 往返（§3.2 的 p99 已含它） | 压测：单副本 p99 与 DB QPS；若真的要缓存，必须同时给出 TTL（= 撤销延迟上限）与跨副本失效方案 |
| N4 | 主体是谁（用户/设备/组织） | ① `user_id` ② 设备主体 ③ org/workspace 层 ④ 计费的 `subject_type` 二元组 | ① | 下游（含计费）本来就只认 `middleware.UserID(c)`（F5）；设备凭证信任模型不同（§4 来源 9/10）；org 今天不存在，全仓库十几个 handler 都手写 `OwnerID == UserID` 比较 | 账号级 key 泄露的面更大；无法给单设备发凭证 | 端到端 G1；计费侧**零改动**即可用 key 扣费（回归测试：用 key 调音色克隆，账户被扣） |
| N5 | scope 粒度与增长 | ① 粗（`read` / `write`）② `resource:action` 细粒度 + 服务端目录 ③ 细粒度 + 通配 | ②（D5/D9） | ①挡不住"只让 CI 建智能体"这类真实诉求；③让最小权限失效；②把"数量增长"的成本赶到目录与 UI 层（分组、搜索、预设、描述提示语），授权语义保持精确匹配 | 前端要维护中英映射（改由目录 API 下发，FR-10）；每条新路由要选 scope；目录与预设多一层维护 | G4 + §7 V3 的**双向**覆盖测试 |
| N6 | 限流实现 | ① 不做 ② 进程内令牌桶（藏在 `Limiter` 接口后） ③ 一开始就上 Redis 分片计数 | ②（D8），**接口预留** | Q5 已确认未来会多副本：现在上 Redis 是无收益复杂度（F6），但不做接口抽象会让将来改不动 | 多副本到来前必须换实现；进程重启桶清零（可接受，桶本来就是软保护） | 并发压测：第 N+1 个请求 429；重启后桶恢复默认；换实现时只需替换 `Limiter` 的构造处 |
| N7 | 撤销/过期的响应文案 | ① 统一 `invalid api key` ② 区分 revoked/expired | ② | 使用者就是我们的用户，知道"被撤了"还是"过期了"直接决定下一步动作；key 存在性的信息价值极低 | 轻微的信息泄露面（能探测某 key 是否存在） | 集成测试断言三种响应 |
| N8 | key 的权限模板 | ① 只有原始 scope 列表 ② 原始 scope + 预设组合（只读/集成） | ② | 前端 mock 页本来就有"只读/智能体调用"这类选项（`ApiKeysPage.tsx:24`），用户心智已经在这；预设只是 scope 数组的快捷方式，**不引入新的授权语义**——但展开必须在服务端、落库只存展开后的列表 | 多一层映射要维护；预设定义一改不影响历史 key（因为不存预设名） | 前端单测：预设展开后的 scope 集合等于服务端定义；后端单测：预设改定义后历史 key 的 scopes 不变 |
| N9 | scope 变多后"路由↔scope 表"如何不腐化 | ① 中央表 + 双向覆盖测试 ② 在每条路由声明上写 scope ③ 从目录代码生成路由包装 | ①（配测试） | 中央表是唯一能一眼回答"这个 scope 到底管什么"的地方；②把授权事实碎成 100 多处，③的代码生成在 Gin 的路由变量用法下省不下多少事 | 表与真实路由可能漂移 → 用测试钉住；新增路由时会先看到测试红 | §7 V3：正向（路由都有 scope）+ 反向（目录里的 scope 要么被路由使用、要么标了 `Deprecated`） |

风险：

| 编号 | 风险 | 概率 | 影响 | 缓解措施 | 触发条件（出现即回滚/改方案） |
| --- | --- | --- | --- | --- | --- |
| R1 | 多副本部署后限流失效 | **高（Q5 已确认会到）** | 中 | 三层准备：① `Limiter` 接口隔离实现（N6）；② 配置项留 `rate_limit.rps/burst`；③ 零代码过渡手段——多副本刚出现时把配额除以副本数（静态分摊）先挡一轮 | manager 副本数 > 1 → 先静态分摊，再按下面两条路径选一条落地 |
| R2 | 新路由漏标 scope → 越权；目录腐化 → "给了但没生效" | 中 | **高** | **双向覆盖测试**（§7 V3）：正向——每条 `/api/*` 必须在 scope 表里或有"key 不可达"白名单项；反向——目录里每个 scope 必须被某条路由使用或标了 `Deprecated` | 该测试被跳过或被加白名单绕过 → 视为阻断发布的缺陷 |
| R3 | 明文串泄漏进日志/错误信息/前端状态 | 中 | **高** | 中间件不打印 `Authorization`；`gin.Logger()` 默认不打 header（F6）；错误里只回前缀；单测断言日志与响应体不含完整串 | 日志中出现 `ox_sk_` 后接完整四段 → 立即轮换所有 key（研究里的响应顺序：先撤销，再排查，见 §4 来源 7） |
| R4 | 用户创建后没保存，明文永久丢失 | 高 | 低 | 创建响应的大横幅 + 复制按钮（沿用 mock 页已有交互）+ 文案提前告知；不做"再次查看" | 支持请求里出现"能否找回"的比例升高 → 评估加密存储（那是一次明确的取舍反转，不是补丁） |
| R5 | 计数/`last_used` 刷库把写压力打到 PG | 低 | 低 | 30s 聚合、单 goroutine、失败只告警不重试 | PG 写延迟出现可观测抬升 → 拉长窗口或转为采样 |

待验证假设：

- A1：`api_keys` 规模 ≤ 10 万行、校验 p99 ≤ 2ms（口径见 §3.2，实际值待观测）。
- A2：多副本**不会在 P2 之前到来**（Q5 已确认它会来，只是时间未定）——若提前，先走 R1 的静态分摊。
- A3：用户能接受"明文只显示一次"（R4 的观测会给出答案）。

### 多副本限流的演进路径（Q5 已确认会到来）

限流的实现会换，但**语义与响应头不变**（FR-6）。两条候选路径，到真的要扩副本时二选一：

| 路径 | 做法 | 优点 | 代价 | 何时选它 |
| --- | --- | --- | --- | --- |
| A 网关层 | 把每 key 限流上移到入口（nginx `limit_req` / APISIX / Kong / 云 LB），manager 不再做 | 与副本数解耦、流量入口最早生效、不引入新存储 | 429 与 `X-RateLimit-*` 由网关产出，语义变弱；每 key 差异化配额需要网关支持动态配置；key 在 header 里是密文，只能按"整串"分桶（但这已足够） | 部署本来就有网关，且团队能改它 |
| B Redis | `internal/apikey` 的 `Limiter` 换成 Redis 令牌桶（Lua 原子脚本） | 语义精确、可按 key 差异化配额、不依赖入口组件、响应头自己控 | 新增 Redis 依赖与运维面；每请求一次网络往返（可比 DB 查询更快） | 需要精确的每 key 配额与 429 语义 |

两条都不选也能撑一阵：配置里把 `rate_limit.rps/burst` 除以副本数（静态分摊）。它真的只是过渡——扩缩容或重启时有拖尾抖动（同一账号可能短暂拿到多份配额），所以要和"已降配额"的台账一起记。

选型判据就三个：① 部署里有没有可改的网关；② 需不需要每 key 不同配额；③ 愿不愿意为一个计数器引入 Redis。三条都不满足时，路径 A 的成本最低。

## 7. 落地与验证

### 7.1 阶段拆分（每阶段可独立上线、独立验证）

**阶段 1：能创建、能调用（只读）**

- 表 + 领域层 + 仓储 + 认证中间件 + **完整的**路由→scope 表（V3 从第一天就有意义，表里每条路由都要有归属）+ scope 目录 API（FR-10，先从只读 scope 起步）；前端先只提供只读预设（`agent:read`/`model:read`/`data:read`/`billing:read`）
- 管理 API：创建 / 列表 / 撤销 / scope 目录（4 个端点，契约见 §5.3）+ `cmd/manager/swagger.go` 注解（跑 `make swagger`，生成物一起提交）
- 前端：删掉 `MOCK_KEYS`，接真接口；创建后展示一次 + 复制；列表掩码
- 开关默认 `false`，内部灰度打开
- 验证：G1、G2、G3 + V1/V2/V3（只读路由部分）

**阶段 2：能写、能限、能看用量**

- 补齐写 scope（`agent:write`/`device:*`/`model:write`/`data:write`/`mcp:*`）
- 每 key 限流、`last_used_at`/`call_count` 聚合刷库（增量写法）、429 响应头
- 控制台：分组、搜索、预设按钮与描述提示（目录 API 在阶段 1 已就位）；列表加计数/最后使用列
- 验证：FR-5/6/8 + G4 + 端到端回归（用 key 跑一次音色克隆，确认计费扣在 owner 账户上）

**不在本期**：设备凭证、`/v1` 入口、审计表、IP 白名单、自动轮换（§8）。

### 7.2 兼容策略

- 既有 JWT 路由语义零变化：只在资源路由组把 `middleware.JWT` 换成 `middleware.Auth`，`/api/auth/*` 仍用纯 JWT。
- 老客户端（控制台 old build、既有脚本）不带 `ox_sk_` 前缀的 Bearer 一律走原 JWT 分支，行为不变。
- 数据：只新增一张表，无迁移、无回填。

### 7.3 开关、灰度、回滚

| 动作 | 具体步骤 |
| --- | --- |
| 开关 | `manager.yaml` 加 `apikey.enabled`（默认 `false`）；`false` 时管理路由回 503、key 中间件不挂载（对齐 `billing`/`payment` 既有的"svc 为 nil 就 503"约定，`cmd/manager/server.go:73-89`） |
| 灰度 | 先在测试环境开；生产只对管理员账号开放创建（阶段 1 用 `is_admin` 收口），跑一周无异常再放开 |
| 回滚 | ① 关闸：`apikey.enabled: false` + 重启 manager → 所有 key 立即失效（用 key 的集成会断，需事先通知）；② 数据无需回滚：`api_keys` 表留着，重新打开即恢复；③ 若要真正撤回功能，取下表：`DROP TABLE api_keys;`（无外键依赖） |

### 7.4 验证方式

| 编号 | 方式 | 覆盖 |
| --- | --- | --- |
| V1 | 单元测试（`internal/apikey`） | 生成/解析/CRC/摘要；scope 判定；限流桶边界；撤销与过期判定；**明文不出现在任何返回结构里** |
| V2 | 单测 + 日志断言 | 用 `httptest` 跑完整中间件链，断言响应体与日志中不含完整串（G2/R3） |
| V3 | **双向路由覆盖性测试** | 正向：遍历 `router.Routes()`，每条 `/api/*` 要么在 scope 表里，要么在"key 不可达"白名单里；反向：目录里每个 scope 要么被至少一条路由使用，要么标了 `Deprecated`。两个方向任一不满足即失败（G5/R2） |
| V4 | handler 集成测试（真 store，`*_test.go` 内联 mock 遵循仓库约定） | 创建→调用→撤销→再调用的全链路；跨账号不可见；403 的 `required`/`granted` |
| V5 | 端到端脚本（`curl`） | G1：同一条请求分别用 JWT 与 key，响应逐字节一致 |
| V6 | 计费回归 | 用 key 调一次会扣费的接口，确认扣在 key 的 `user_id` 对应账户上（验证 D2 的"零耦合打通"） |

上线后看什么（判断成功）：

| 指标 | 期望 | 偏离时的动作 |
| --- | --- | --- |
| 用 key 的请求占比 | 稳定增长，且不出现 `401 invalid` 占比 > 5% | 占比过高说明文档/前端引导有问题；`invalid` 高说明有客户端在用错格式（去查日志里的 `lookup` 前缀） |
| 403 分布（按 `required` scope 聚合） | 集中在 `*:write` | 说明 scope 切得过细或前端预设不合理 → 考虑合并/加预设 |
| 429 次数 | 接近 0 | 单账号集中出现 → 配额偏紧或有人在滥用，取两者之一调整 |
| 校验 p99 | ≤ 2ms | 与 DB 延迟同向上涨 → 触发 N3 的重新评估 |
| 撤销调用次数 / 创建数 | 无异常聚集 | 短时间内大量撤销 → 怀疑泄露事件，走 §4 来源 7 的响应顺序 |

## 8. 未决问题

| # | 问题 | 阻塞谁 | 谁能定 | 最晚什么时候要有答案 |
| --- | --- | --- | --- | --- |
| Q1 | ~~§5.3 的 12 个 scope 是否就是 P1 全量？~~ | — | — | **已关闭 2026-09-25：按 §5.3 全量落地** |
| Q2 | 是否需要可恢复的"停用"状态（而不是只有撤销/过期两个终态）？ | 阻塞状态机与表结构（影响是否需要多一个字段与两条迁移边） | 产品 | 阶段 2 开始前 |
| Q3 | 是否需要 key 维度的用量报表（把 `api_key_id` 写进计费用量事件的维度）？ | 阻塞计费侧的小改动与新报表 | 产品 + 计费负责人 | 阶段 2 结束前 |
| Q4 | 设备凭证（P2）的形态：高熵随机串起步，还是直接上非对称（设备侧密钥对）？传输与轮换流程谁定？ | 阻塞 WS 握手鉴权与固件侧改造 | 架构 + 客户端负责人 | 下次动数据面之前 |
| Q5 | 多副本什么时候到、限流的最终落点选哪条路径（§6 的 A 网关 / B Redis）？ | 阻塞 `Limiter` 的实现选择，不阻塞 P1 | 运维 + 架构 | 副本数变更之前 |
| Q6 | 是否登记 GitHub secret scanning 合作方（决定校验段与前缀的长期收益） | 不阻塞实现，只影响 D3 的收益兑现 | 产品/安全 | 任意 |
| Q7 | `api_keys` 行的保留策略（软删行是否要归档/清理） | 不阻塞 | 后端 | 上线 6 个月后复盘 |
| Q8 | key 的 IP 白名单是否需要（Stripe 有此能力） | 阻塞一个字段与一段中间件 | 产品 | 阶段 2 之后 |
| Q9 | 新增 scope 的准入标准（什么情况才值得单独一个 scope，而不是归到已有的）与目录定期收敛流程 | 不阻塞 P1；不回答的话目录会膨胀到没人能选对 | 产品 + 后端 | 目录超过 ~30 条之前 |
| Q10 | 第三段命名（`resource:action:sub`）什么时候启用？ | 不阻塞 P1；它是一条一次性决定（一旦开始用就停不下来） | 后端 | 目录超过 ~30 条之前 |

已关闭：Q1（scope 清单）；Q5 的部署形态部分（当前单副本、**未来会多副本**）。这两条的结论已写回 §1（D8/D9）、§3.2、§5.3、§6，见文末变更记录。

## 附录

### 术语表

| 术语 | 含义 |
| --- | --- |
| lookup | key 串里的公开段，落库为明文、建唯一索引，用于 O(1) 查找；它不是秘密 |
| scope | 授权单位，`resource:action` 形式，例如 `agent:write`；deny-by-default，不支持通配 |
| deny-by-default | 未显式授予的权限一律没有；空 scope 集合等于没有权限 |
| 校验段（CRC） | key 串末段的校验码，用于不打库就拒绝畸形输入、以及让外部扫描器少报误判 |
| principal | 本次请求的身份来源：`jwt`（人）或 `api_key`（机器）；两者产出的 `userID` 语义相同 |
| 设备凭证 | 与 API key 并列但不同类的凭证，用于设备连接数据面（本期不做） |

### 来源（全部于 2026-09-25 访问）

1. Behind GitHub's new authentication token formats — https://github.blog/engineering/platform-security/behind-githubs-new-authentication-token-formats/（2021-04-05）
2. Stripe Docs: API keys — https://docs.stripe.com/keys
3. seamapi/prefixed-api-key — https://github.com/seamapi/prefixed-api-key
4. Zuplo: API key best practices — https://zuplo.com/blog/api-key-best-practices.md ；Zuplo: API Key Authentication Best Practices — https://zuplo.com/blog/api-key-authentication
5. apikeys.guide: Hashing & Storage — https://apikeys.guide/docs/security/hashing-and-storage（2026-05-05）
6. OWASP Cheat Sheet Series: Password Storage — https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html
7. Truffle Security: Deleting leaked API keys isn't a solution — https://trufflesecurity.com/blog/remediate-leaked-api-keys-with-key-rotation（2023-08-03）
8. GitGuardian: What to do after leaking Credential and API keys — https://blog.gitguardian.com/leaking-secrets-on-github-what-to-do/（2022-12-20）
9. OWASP IoT Security Verification Standard V2 — https://owasp.org/IoT-Security-Verification-Standard-ISVS/en/V2-User_Space_Application_Requirements.html
10. nRF Cloud: Security and credentials — https://docs.nrfcloud.com/docs/legacy-nrfcloud/device-security.md ；AWS IoT: Authorizing direct calls to AWS services — https://docs.aws.amazon.com/iot/latest/developerguide/authorizing-direct-aws.html
11. Layers docs: API keys — https://layers.com/docs/api/concepts/api-keys
12. scalablesystem.dev: Build an API Key Management System — https://scalablesystem.dev/system-design/build-an-api-key-management-system ；Let's Build Solutions: Designing API Authentication for B2B SaaS — https://letsbuildsolutions.com/blog/web-engineering/designing-api-authentication-for-b2b-saas-api-keys-oauth-client-credentials-and-scoped-access-tokens/（2026-04-18）
13. Safeguard.sh: API Key Format Design — https://safeguard.sh/resources/blog/designing-an-api-key-that-can-be-found-when-it-leaks（2026-09-18）

### 变更记录

| 日期 | 变更 | 原因 |
| --- | --- | --- |
| 2026-09-25 | 初稿 | 从控制台 mock 页（`web/manager/src/pages/ApiKeysPage.tsx`）反推后端设计；确认 P1 范围只到 `/api/*` |
| 2026-09-25 | 依评审结论修订：① scope 清单确认为 P1 全量（Q1 关闭）；② 多副本从假设改为已确定的演进（Q5），限流接口化 + 两条演进路径 + 计数改增量写法；③ 针对"scope 会持续变多"新增 D9 与目录/预设设计（N9、FR-10、双向覆盖测试、新增 Q9/Q10）；④ 撤掉目录里的 `Costs` 字段；⑤ **补上 §5.3「HTTP API」一节**，按"curl 优先"写（索引表 + 4 个可执行示例 + 失败响应），并据此定下"凭证不能自我复制"：管理面只接 JWT，`/api/api-keys*` 进不可达白名单；⑥ 补 key 串形状规范（base58 字母表、CRC 计算口径、Digest 覆盖整串） | 评审反馈：scope 数量会增长；部署未来会多副本；凭证目录不该携带计费事实；**设计文档漏了 HTTP API 契约**；接口契约用可执行示例比表格好读 |
| 2026-09-25 | 按本文（含上面第 ⑤⑥ 条）实现：`internal/apikey` + `internal/store/api_key.go` + `cmd/manager/middleware/{apikey,scope_table}.go` + `cmd/manager/handler/apikey.go` + 控制台页面，并跑 `make swagger` 把 4 个端点写进 `docs/manager/`。三处落地选择：① scope 目录接口额外下发 `groups`（分组展示名）与 `can_create`（灰度期可见性），新增 scope 或分组不需要前端发版；② 新增配置 `apikey.admin_only` 落地 §7.3 的"生产只对管理员开放创建"；③ `internal/apikey.Service` 依赖本包的 `Store` 接口（实现仍在 `internal/store/api_key.go`），以便无 DB 的单测内联假实现。①③ 都是"响应字段只增"与仓储可替换范围内的增量，不改授权语义、存储形态与撤销时效 | 实现与设计一致；接口字段的新增是设计允许的方向，单测与双向覆盖性测试已钉住行为 |
