# 单机部署

本目录部署三个服务：manager API、wsserver 和 PostgreSQL（含 pgvector）。
manager 前端由 Nginx 托管；dogset 前端继续使用 GitHub Pages，不在此部署中。

## 前提

- 一台 Linux x86_64 服务器，已安装 Docker Engine 和 Docker Compose plugin。
- 对外放行 TCP `80`；生产环境还应通过反向代理或证书管理工具提供 TCP `443`。
- 服务器可访问 GitHub Container Registry（`ghcr.io`）。如果包保持私有，还需要一个具有 `read:packages` 权限的 GitHub fine-grained PAT。

`wsserver` 依赖 ONNX Runtime 与 Opus。镜像已在构建阶段装入对应 Linux x86_64 运行库，因此无需在宿主机安装 Go、ONNX Runtime 或音频库。VAD 模型不进入镜像，Compose 将仓库的 `models/` 目录以只读 bind volume 挂载到 wsserver 的 `/app/models`；部署目录必须保留该目录及其模型文件。

## 首次上线

GitHub Actions 仅在 GitHub Release 发布时构建并发布镜像；Release 必须关联版本 tag。工作流分别发布 `runtime`（manager、wsserver）和 `nginx`（manager 前端）镜像到 GHCR。服务器不编译源代码，只需保留 `deploy/`（含 `deploy/.env` 与 `deploy/manager.yaml`）和 `models/`。

首次部署时，在仓库根目录执行：

```bash
cp deploy/.env.example deploy/.env
# 编辑 deploy/.env，替换 POSTGRES_PASSWORD。
# POSTGRES_PASSWORD 请使用 URL 安全字符（字母、数字、-、_）。
./deploy/deploy.sh init-config
# 编辑 deploy/manager.yaml：
#   - database.dsn 使用与 POSTGRES_PASSWORD 相同的密码
#     （postgres://orion:<POSTGRES_PASSWORD>@postgres:5432/orionx?sslmode=disable）
#   - jwt.secret 换成一段长随机串
#   - admin.password 设置首次登录密码（创建管理员后可从配置文件删除）
#   - 其余按需填写（smtp / internal.token / payment / storage 等）
# 私有 GHCR 包：docker login ghcr.io -u <GitHub 用户名>
docker compose --env-file deploy/.env -f deploy/docker-compose.yml up -d
```

`manager.yaml` 含数据库密码等敏感值，请限制权限（`chmod 600 deploy/manager.yaml`；容器进程以 uid 10001 运行，必要时 `chown 10001:10001`），并确保它不进入版本库（已在 `.gitignore` 中忽略）。manager 只从该文件读取配置，不再支持环境变量覆盖；文件缺失或权限不可读时容器会直接启动失败。

生产环境建议把 `ORION_X_TAG` 设为发布的版本 tag（例如 `v1.2.3`）或对应的 `sha-...` 标签，避免跟随 `latest` 自动变更。

检查服务：

```bash
docker compose --env-file deploy/.env -f deploy/docker-compose.yml ps
curl -f http://127.0.0.1/healthz
docker compose --env-file deploy/.env -f deploy/docker-compose.yml exec wsserver \
  curl -f http://127.0.0.1:8081/healthz
docker compose --env-file deploy/.env -f deploy/docker-compose.yml logs -f manager wsserver
```

浏览器访问 `http://<服务器域名或 IP>/`。首次登录使用 `deploy/manager.yaml` 中 `admin.username` 和 `admin.password` 配置的账号。管理员仅在数据库中不存在该用户名时创建；首次登录后请在 UI 中修改密码，并从 `deploy/manager.yaml` 删除 `admin.password`（重新创建容器不会重置已有账号）。

设备连接地址为 `ws://<服务器域名>/ws`；启用 HTTPS 后必须改为 `wss://<服务器域名>/ws`。

## HTTPS

此 Compose 文件只占用 HTTP 80 端口，以便适配已有的 Caddy、Nginx 或云负载均衡器。上线公网前，应让 TLS 终止层把 HTTPS/WSS 请求转发到 `127.0.0.1:${HTTP_PORT}`，并在前端与设备中统一使用域名和 `https`/`wss`。不要把 `postgres`、manager `9090` 或 wsserver `8080` 直接映射到宿主机端口。

## 从旧版升级（环境变量 → 配置文件）

旧版 manager 会用 `deploy/.env` 中的 `DB_DSN`、`JWT_SECRET`、`ADMIN_USERNAME`、`ADMIN_PASSWORD`、`LOG_LEVEL` 等环境变量覆盖配置文件；新版本已移除该逻辑，配置只从 `deploy/manager.yaml` 读取。平滑升级步骤：

1. 更新代码后先执行 `./deploy/deploy.sh init-config` 生成 `deploy/manager.yaml`；
2. 把旧 `.env` 的值搬过去：`database.dsn` 用旧 `DB_DSN`（主机名指向 `postgres:5432`）、`jwt.secret` 用旧 `JWT_SECRET`、`admin.*` 用旧 `ADMIN_USERNAME`/`ADMIN_PASSWORD`、`logging.level` 用旧 `LOG_LEVEL`；按需补齐 `github_oauth`、`internal.token`、`payment`、`storage` 等段；
3. 确认 `deploy/manager.yaml` 权限后按上面的更新流程重建容器。

为保留回滚窗口，升级验证通过前不要删除 `deploy/.env` 里的旧变量——旧镜像 + 旧版 compose 仍需要它们。回滚时把镜像 tag 与 `deploy/docker-compose.yml` 一并退到旧版本（`git checkout <旧版本 tag> -- deploy/docker-compose.yml`），旧变量还在 `.env` 里即可直接启动。验证稳定后再删除 `.env` 中的 `DB_DSN`/`JWT_SECRET`/`ADMIN_USERNAME`/`ADMIN_PASSWORD`（`LOG_LEVEL` 仍供 wsserver 使用，保留）。

注意：过渡期内旧环境变量对新版本不再生效，`deploy/manager.yaml` 才是唯一配置来源。

## 更新与备份

发布新镜像后，在服务器拉取并重建容器：

```bash
docker compose --env-file deploy/.env -f deploy/docker-compose.yml pull
docker compose --env-file deploy/.env -f deploy/docker-compose.yml up -d
```

备份 PostgreSQL：

```bash
docker compose --env-file deploy/.env -f deploy/docker-compose.yml exec -T postgres \
  pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" > orionx-$(date +%F).sql
```

执行备份命令前，先在当前 shell 中导出与 `deploy/.env` 相同的 `POSTGRES_USER`、`POSTGRES_DB`，或将命令中的变量替换为实际值。数据库数据保存在 Docker volume `postgres-data`；不要在未完成备份时删除该 volume。

`deploy/manager.yaml` 含数据库密码、JWT 密钥与管理凭据等敏感值，必须与数据库一起备份（建议加密存放）；迁移或重建服务器时缺少该文件，manager 将无法启动。
