# Manager 配置单一来源与敏感值加解密注入设计

> 状态：待评审 | 日期：2026-10-09 | 范围：manager 配置加载、deploy 模板、示例配置与文档 | 关联：`cmd/manager/config.go` TODO、[#82](https://github.com/LiusCraft/orion-x/issues/82)、`docs/auth-email-verification-design.md` §3.1 / 评审记录 2026-10-03、`docs/superpowers/specs/2026-09-19-resource-storage-design.md` §9.1

## 评审面（≤3 屏）

### 1. 结论与边界

结论：

1. **删除 `applyManagerEnv`**。manager 进程除 `-config` 指定的配置文件外不再读取任何环境变量作为配置；同一个值只有一个来源。
2. **敏感值独立加密注入**：新增独立的加密 secrets 文件（`manager.secrets.yaml`）+ 密钥环（`manager.key`），启动时解密后写回配置结构体。算法用标准库 AES-256-GCM（值格式带版本、key id，AAD 绑定字段名），不引入新依赖。
3. **fail closed**：secrets 文件/密钥环读取失败、任意一个 `ENC[...]` 解密失败、同一敏感项在两个文件同时出现，一律启动失败（`exit 1`），**不回退明文、不回退环境变量**；非敏感的缺失行为保持现状（DB/JWT 缺失启动失败，支付/存储等可选能力降级）。
4. **迁移可回退**：新镜像上线的同时准备配置文件；回滚 = 切回旧镜像 tag + 旧 compose（旧镜像继续读 env，`.env` 在过渡期不删）。

边界：

- In：manager 进程配置加载、`deploy/` 模板与 runbook、`manager.example.yaml`、README 与相关设计文档同步。
- Out：其它服务（wsserver `MANAGER_TOKEN`、voicebot 等）的 env 迁移；DB 内既有数据（`device_channels.config`、`tg_bot_token`）的加密；KMS/Vault 集成；配置热加载与不重启轮换。以上如需要另行开 issue。

### 2. 骨架

部署侧文件布局（compose 单机部署）：

```text
deploy/
  manager.yaml             # 非敏感配置：server / logging / storage 地址 / github client_id / payment pid…
                           # 敏感字段留空；可提交、可备份
  manager.secrets.yaml     # 敏感值，值形如 ENC[v1:<key_id>:<base64>]；0600，不进仓库
  manager.key              # 密钥环（active + 退休密钥）；0600，不进仓库
```

`manager.yaml` 用 bind mount 挂到 `/app/data/manager.yaml`（默认 `-config` 路径）；两个 secrets 文件走 compose 顶层 `secrets:`，挂到 `/run/secrets/`（宿主文件保持 0600，容器内 0444 只读）。

启动时序：

```text
manager
 └─ loadManagerConfig(-config)
     ├─ 读 / 反序列化 manager.yaml
     ├─ secrets.file 为空 → 直接使用文件里的敏感值，逐项告警「明文存储」
     └─ secrets.file 非空
         ├─ loadKeyring(key_file)：解析、校验 active 存在、每个 key 为 32 字节
         ├─ loadSecretsFile(file)：校验 version、未知逻辑键报错（防拼写错误）
         ├─ 遍历敏感字段注册表（§3.5）：
         │   ├─ 主配置与 secrets 文件同时有值 → 报错（双来源）
         │   ├─ 值以 ENC[ 开头 → 解密（失败即报错）；写入配置字段
         │   └─ 值非 ENC → 写入配置字段并告警（迁移期容忍，verify 会列出）
         └─ 主配置里仍存在的明文敏感值 → 逐项告警
```

### 3. 契约

#### 3.1 `manager.yaml` 新增 `secrets` 段

```yaml
secrets:
  file: "/run/secrets/manager.secrets.yaml"  # 留空 = 不启用（本地开发直接把敏感值写在本文）
  key_file: "/run/secrets/manager.key"       # file 非空时必填
```

路径不是敏感值，放在主配置文件里；不提供默认路径，避免"文件没挂上却静默降级"。

#### 3.2 `manager.secrets.yaml`

```yaml
version: 1
values:
  database.dsn: "ENC[v1:20261009:AbCd...]"   # base64(nonce(12B) || AES-256-GCM 密文+tag)
  jwt.secret: "ENC[v1:20261009:...]"
  internal.token: "CHANGE_ME"                # 非 ENC 值 = 明文：启动告警，verify 列为未加密
```

- 未知 `version`、未知逻辑键、空值以外的格式错误 → 启动失败。
- 空字符串/纯空白视为未配置（与现有"空 = 未设置"语义一致）。

#### 3.3 `manager.key`（密钥环）

```yaml
version: 1
active: "20261009"                  # 加密只用 active；解密允许任意已登记 key
keys:
  "20261009": "base64(32B)"         # 当前密钥
  "20260601": "base64(32B)"         # 退休密钥：只解密，不加密
```

#### 3.4 `ENC` 值格式

`ENC[v1:<key_id>:<base64(nonce||ciphertext)>]`

- `v1`：AES-256-GCM，随机 96-bit nonce，AAD = 逻辑键名（如 `database.dsn`），阻止把密文挪到别的字段。
- `key_id`：`[A-Za-z0-9._-]{1,64}`，与密钥环的 key 对应。
- 拒绝：未知版本、未知 key id、base64/长度非法、GCM 校验失败、AAD 不匹配。
- 版本位保留给未来换算法（如 age/KMS envelope），不需要改文件结构。

#### 3.5 敏感字段注册表

| 逻辑键 | 配置字段 | 现状缺失行为（不变） |
| --- | --- | --- |
| `database.dsn` | `database.dsn` | 启动失败 |
| `jwt.secret` | `jwt.secret` | 启动失败 |
| `admin.password` | `admin.password` | 不创建管理员（已有账号则无影响） |
| `smtp.password` | `smtp.password` | 构造 mailer，发送时认证失败 |
| `redis.password` | `redis.password` | 连接失败（启用邮箱验证时启动探活失败） |
| `github_oauth.client_secret` | `github_oauth.client_secret` | OAuth provider 不注册 |
| `internal.token` | `internal.token` | 空 = `/internal/billing/*` 拒绝一切请求 |
| `payment.key` | `payment.key` | 支付初始化失败 → 充值 503，其余不受影响 |
| `storage.access_key` | `storage.access_key` | 对象存储构造失败 → 资源接口 503 |
| `storage.secret_key` | `storage.secret_key` | 同上 |

`admin.username`、`github_oauth.client_id`、`storage.endpoint/bucket` 等保持非敏感，留在主配置文件。

#### 3.6 子命令（复用 manager 二进制，不改 Dockerfile）

`manager` 无参数时行为不变（compose `command: ["manager"]` 不受影响）；新增：

| 命令 | 作用 |
| --- | --- |
| `manager secrets init [-key-file …] [-id …]` | `crypto/rand` 生成 32 字节密钥并写密钥环（0600），拒绝覆盖已有文件 |
| `manager secrets encrypt [-in …] [-key-file …]` | 把 secrets 文件里的明文值加密成 `ENC[...]`，原子写回（tmp+rename） |
| `manager secrets verify [-config …]` | 按启动路径完整校验：解密、双来源、未加密项、缺失必填项；有错退出非零 |
| `manager secrets rotate [-key-file …] [-new-id …]` | 生成新 active key，先写"新+旧"密钥环，再用新 key 重加密全部值 |
| `manager secrets prune -key-file … -id <old>` | 删除退休密钥；仍有密文引用该 key 时拒绝 |
| `manager secrets import-env`（可选，迁移辅助） | 读 15 个历史环境变量，直接输出加密后的 secrets 文件 |

#### 3.7 失败路径与 fail-closed 矩阵

| 场景 | 行为 |
| --- | --- |
| `secrets.file` 已配置但文件不存在/不可读 | 启动失败 |
| `key_file` 缺失、非法、key 长度 != 32B | 启动失败 |
| 任意 `ENC[...]` 解密失败 | 启动失败，错误含逻辑键名（不含明文） |
| 同一逻辑键在主配置与 secrets 文件都有值 | 启动失败，提示从 `manager.yaml` 删除 |
| secrets 文件里的值未加密 | 启动告警，正常启动；`verify` 退出非零 |
| 主配置里的敏感值未加密（未启用 secrets） | 启动告警，正常启动（本地开发） |
| 可选项缺失（支付/存储/token…） | 维持现状的按能力降级，见 §3.5 |

### 4. 决策与风险

- **D1 独立 secrets 文件，而不是在主配置里内联 `ENC[...]`**：主配置文件可以放心提交/备份/评审；轮换只碰小文件；容器里可以用更严的挂载方式；"敏感字段在主配置留空"让双来源检查可以做成硬错误。
- **D2 标准库 AES-256-GCM，而不是 age/sops/KMS**：仓库刚在邮箱验证评审里定了"不引入新依赖"；age/sops 是外部工具，解密后明文仍要落盘或进 env；Vault/KMS 对单机 compose 部署是运维负担。GCM 封装面很小（nonce + key id + AAD），用单测钉死。版本位预留未来切换。
- **D3 双来源直接报错，而不是"secrets 覆盖主配置"**：本 issue 的动机就是消灭双轨；覆盖语义只是把 env 换成了文件，排障还是要查两处。
- **D4 明文容忍但告警 + `verify`**：迁移期不能要求一次到位；但生产 runbook 把"verify 无告警"列为验收项。
- **D5 轮换采用"先加新 key、再重加密、最后清旧 key"的顺序**：任意时刻崩溃，密钥环里新旧都在，文件始终可解；`prune` 用密文引用检查兜底。

风险：

| 风险 | 缓解 |
| --- | --- |
| `manager.key` 丢失 = 所有敏感值不可恢复 | 备份 secrets 文件时必须同时离线备份 key；runbook 明确"key 与密文分开存放" |
| `manager.key` 泄露 | 轮换 runbook（§F.3）；泄露的旧 key 无法解新密文，但历史备份仍有风险 |
| 进程内存/日志里的明文 | 沿用现有约束：不打印配置值；错误信息只含逻辑键名 |
| compose `secrets:` 容器内是 0444 | 仅容器内可见；宿主文件 0600，`.gitignore` 兜底，runbook 有 chmod 步骤 |
| DSN 里的 DB 密码与 `.env` 的 `POSTGRES_PASSWORD` 双份维护 | 见 §5 未决问题 |

### 5. 未决问题

1. **DSN 密码双份维护**：compose 的 postgres 容器继续用 `.env` 的 `POSTGRES_PASSWORD`，而 manager 的完整 DSN 加密在 secrets 文件里。改数据库密码要改两处。可选方案：`deploy.sh` 增加一个只读 `.env`、调用 `manager secrets encrypt` 重写 DSN 的辅助命令（部署期工具，不改变"运行期只读文件"）。倾向做，但可以放到迁移 runbook 落地之后再补。
2. **是否需要 bridge release**：方案按"单次发布 + runbook"设计（迁移量只有 3 个敏感值）。如果维护者想更保守，可以先发一个"支持 secrets 文件 + env 覆盖仍生效并告警"的版本，下一版再删 `applyManagerEnv`；代价是多一轮发布。
3. secrets 文件是否纳入常规备份（倾向：纳入，但 key 单独离线保管，不放在同一份备份介质）。
4. `import-env` 是否值得做：默认部署里其实只有 `DB_DSN`、`JWT_SECRET`、`ADMIN_PASSWORD` 三个敏感值，手工填写成本很低；倾向列为可选。

## 支撑材料

### A. 背景与现状（展开）

`loadManagerConfig`（`cmd/manager/config.go:321`）在 `yaml.Unmarshal` 之后调用 `applyManagerEnv`（`cmd/manager/config.go:342`），15 个环境变量按"非空即覆盖"写回配置，TODO 位于 `cmd/manager/config.go:340`。

| 现状 | 位置 |
| --- | --- |
| 15 个 env 覆盖的唯一读取点 | `cmd/manager/config.go:342-391` |
| manager 启动配置加载与必填校验 | `cmd/manager/main.go:43-64` |
| 管理员首启创建（依赖 `admin.password`） | `cmd/manager/main.go:122-144` |
| `internal.token` 空 = 拒绝一切 | `cmd/manager/middleware/internal.go:19`（InternalAuth）、`cmd/manager/main.go:184-186` |
| 支付/存储/邮件等按能力降级 | `cmd/manager/main.go:163-169、193、283-319、342-357` |
| 生产部署只走 env（compose → manager env） | `deploy/docker-compose.yml:21-27`、`deploy/.env.example:5-8` |
| 邮箱验证评审约定"本期不新增环境变量，配置只读文件" | `docs/auth-email-verification-design.md:199、259` |
| storage 设计把 env 覆盖当既定能力 | `docs/superpowers/specs/2026-09-19-resource-storage-design.md:347-348、355` |
| README 把 env 覆盖写成使用方式 | `README.md:84、94-95、101` |

代码与文档已有的不一致：`GITHUB_CLIENT_ID/SECRET/REDIRECT_URL`、`INTERNAL_TOKEN`、`EPAY_KEY`、`STORAGE_*` 在代码里支持覆盖，但 deploy 模板从未注入；README 只写了其中一部分。本方案一次性收敛。

### B. 需求与约束（展开）

| 编号 | 需求 | 判定方式 |
| --- | --- | --- |
| FR-1 | 删除 `applyManagerEnv`，manager 不再读 env 配置 | 回归单测：`t.Setenv` 全部 15 个变量后 `loadManagerConfig` 仍返回文件值 |
| FR-2 | 敏感值从独立加密文件注入，10 项注册表覆盖 DB/JWT/SMTP/支付/internal/存储/Redis/GitHub/admin | `internal/secrets` 单测 + config 单测 + 本地 E2E |
| FR-3 | 解密失败/密钥缺失/双来源 = 启动失败，不回退 | 单测 + 手工：坏密文、错 key、双写字段分别启动 |
| FR-4 | 密钥轮换流程可用、崩溃安全 | `rotate`/`prune` 单测（含中间态可解）+ runbook 演练 |
| FR-5 | 现有部署平滑升级、可回滚 | runbook 演练：旧 compose → 新 compose → 回滚旧 tag |
| FR-6 | 部署模板、示例、README、设计文档同步，无残留 env 说法 | 文档 diff 审查 + `grep` 检查 |

约束：不引入新依赖（stdlib 加密）；不改其它服务；默认路径 `-config data/manager.yaml` 不变；`manager` 无子命令时行为不变。

### C. 业界调研

| 做法 | 可借鉴 | 为什么不直接用 |
| --- | --- | --- |
| 12-factor：配置走 env | 部署一致性好 | 与仓库已有约定冲突（`auth-email-verification-design.md` 评审记录），且敏感值裸奔在 env/进程表/`docker inspect` 里 |
| sops / age 外部工具解密 | 成熟、可审计、多接收人 | 需要部署机额外安装工具与密钥；解密产物要么落盘（换了个地方的明文）要么再进 env |
| Vault / 云 KMS | 轮换、审计、最小权限完备 | 单机 compose 自托管部署没有配套基础设施；引入可用性单点 |
| KMS envelope encryption | 数据密钥 + 主密钥分层 | 作为 `v1` 之后的演进方向（值格式已留版本位） |
| OWASP Secrets Management Cheat Sheet | 静态加密、密钥轮换、最小权限、与代码分离 | 原则全采纳；具体落地按自托管约束裁剪 |

### D. 落地与验证

分阶段（每阶段可独立评审/合并）：

1. `internal/secrets` 包：`keyring.go`（解析/校验）、`cipher.go`（`ENC` 编解码、GCM）、单测。
2. `cmd/manager/config.go` 接入：`SecretsConfig`、注册表、解密注入、双来源检查、明文告警；**删除 `applyManagerEnv` 与 TODO**；补 FR-1 回归测试。
3. `cmd/manager/secrets_cmd.go`：`init/encrypt/verify/rotate/prune` 子命令 + 单测（`import-env` 可选）。
4. `deploy/`：`manager.example.yaml`、`manager.secrets.example.yaml` 新模板；compose 挂载改造；`.env.example` 瘦身；`.gitignore` 增加三个文件；`deploy/README.md` 重写首次上线 + 轮换 + 回滚三节。
5. 文档同步：`manager.example.yaml` 注释、`README.md:84/94-95/101/261`、`docs/auth-email-verification-design.md`（§A 表、§5、约束、变更记录）、`docs/superpowers/specs/2026-09-19-resource-storage-design.md:355`、`docs/billing-design.md:575`（注明 manager 侧来源变化、wsserver 不变）。
6. E2E 演练：本地 compose 起一套 → 按 runbook 迁移 → `verify` 通过 → 轮换 → 回滚。

关键单测清单：

- `internal/secrets`：加解密往返；nonce 不重复；AAD 不匹配失败；密文篡改失败；错 key 失败；未知 key id/version/base64 失败；密钥环 active 缺失、key 非 32B、重复 id；rotate 在"密钥环已写、密文未换"的中间态仍可解；prune 拒绝被引用的 key。
- `cmd/manager`：双来源报错；secrets 明文值告警且生效；`secrets.file` 缺失/key 缺失报错；未启用 secrets 时明文生效；15 个 env 全部被忽略（FR-1）。
- 子命令：`init` 拒绝覆盖；`encrypt` 幂等（已是 ENC 的不重复加密）；`verify` 退出码。

### E. 失败路径（展开）

- **key 文件丢失**：所有 `ENC` 值不可解 → 启动失败；恢复途径 = 找回 key 文件备份，或从主配置/旧 `.env` 重新录入并重新 `init`+`encrypt`（密文不可逆，只能重录）。
- **secrets 文件损坏/被截断**：YAML 解析或解密失败 → 启动失败；`rotate` 原子写（tmp+rename）保证不会写坏原文件。
- **轮换中途崩溃**：顺序保证密钥环先含新旧两把 key，再换密文；任意中间态都至少有一把可解，重启 `rotate` 幂等。
- **回滚后新文件遗留**：旧镜像不读 secrets 文件，无害；再次升级时文件还在，可直接复用。
- **误把敏感值留在 `manager.yaml` 且启用了 secrets**：若 secrets 文件里也有该项 → 启动失败提示删除；若 secrets 文件没有 → 告警明文。
- **`internal.token` 与 wsserver 不一致**：manager 侧从 secrets 文件读，wsserver 侧（本期不改）继续从 env `MANAGER_TOKEN` 读；部署文档要求同一个值两处注入，否则 `/internal/billing/*` 全部 401（fail closed，现状即如此）。

### F. runbook

#### F.1 现有部署迁移（单次发布方案）

1. 备份 `deploy/` 目录与数据库 dump；记录当前 `ORION_X_TAG`。
2. 准备密钥：`manager secrets init -key-file deploy/manager.key`（可在本机用 `make build-manager` 的产物或新镜像一次性容器执行）。
3. 生成 `deploy/manager.yaml`：从 `deploy/manager.example.yaml` 复制，敏感字段留空，填非敏感项（`admin.username`、`logging`、`storage` 地址等）。
4. 生成 `deploy/manager.secrets.yaml`：复制 `deploy/manager.secrets.example.yaml`，填入当前值——旧容器里可用 `docker compose exec manager printenv DB_DSN JWT_SECRET ADMIN_PASSWORD` 取值——然后 `manager secrets encrypt`。
5. `chmod 600 deploy/manager.key deploy/manager.secrets.yaml`；确认 `.gitignore` 已覆盖。
6. 切镜像：更新 `ORION_X_TAG`，`docker compose pull && up -d`；compose 已不再向 manager 注入配置 env。
7. 验证：`curl -f /healthz`、登录控制台、日志无明文告警、`docker compose exec manager manager secrets verify` 通过。

#### F.2 密钥轮换

1. `manager secrets rotate -key-file deploy/manager.key -new-id <YYYYMMDD>`（旧 key 保留为退休密钥）。
2. `docker compose up -d` 重启 manager（配置只在启动时读，无热加载）。
3. `verify` 通过后，`manager secrets prune -key-file deploy/manager.key -id <旧 id>`，再次重启。

#### F.3 紧急轮换（key 疑似泄露）

同 F.2 立即执行；`rotate` 完成后所有密文已用新 key 重加密，泄露的旧 key 不能解当前文件。随后评估历史备份、日志中是否出现过 key 文件，按需重录所有敏感值（如 DB/JWT 全部换新）。

#### F.4 回滚

1. 恢复旧 `deploy/`（`git checkout <上一版本> -- deploy/`），`ORION_X_TAG` 指回旧 tag，`docker compose up -d`。
2. `.env` 中 `JWT_SECRET`/`ADMIN_PASSWORD`/`POSTGRES_*` 在确认新方案稳定前不要删除。
3. 回滚后 secrets 文件/key 留在服务器无副作用，下次升级直接复用。

#### F.5 文档同步清单（落地阶段执行）

| 文件 | 动作 |
| --- | --- |
| `cmd/manager/config.go` | 删 `applyManagerEnv` 与 TODO，接入 secrets 注入 |
| `deploy/docker-compose.yml` | manager 去 environment；加 `secrets:` 与配置文件挂载 |
| `deploy/.env.example`、`deploy/README.md` | 去掉 manager 敏感 env；重写首次上线/轮换/回滚 |
| `deploy/manager.example.yaml`、`manager.secrets.example.yaml` | 新增部署模板 |
| `manager.example.yaml` | `secrets` 段（注释说明）+ 敏感字段注释改为"填 secrets 文件" |
| `README.md:84/94-95/101/261` | 删除 env 覆盖说法，改述 secrets 文件与密钥管理 |
| `docs/auth-email-verification-design.md:181、199、259` | §A 现状表更新、约束更新、变更记录追加 |
| `docs/superpowers/specs/2026-09-19-resource-storage-design.md:347-348、355` | 标注 env 覆盖已被本方案取代 |
| `docs/billing-design.md:575` | 注明 manager 侧 `internal.token` 来源变化，wsserver 不变 |
| `docs/wsserver-apikey-auth-design.md:91` | 示例中 `INTERNAL_TOKEN` 注明仅 wsserver/调试用 |

变更记录：

- 2026-10-09：初稿。按 issue #82 期望给出"删 env 覆盖 + 独立加密 secrets 注入 + 迁移回滚"方案；未实现，待评审后再进入编码。
