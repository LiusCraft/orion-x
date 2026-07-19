# 单机部署

本目录部署三个服务：manager API、wsserver 和 PostgreSQL（含 pgvector）。
manager 前端由 Nginx 托管；dogset 前端继续使用 GitHub Pages，不在此部署中。

## 前提

- 一台 Linux x86_64 服务器，已安装 Docker Engine 和 Docker Compose plugin。
- 对外放行 TCP `80`；生产环境还应通过反向代理或证书管理工具提供 TCP `443`。
- 服务器可访问 GitHub Container Registry（`ghcr.io`）。如果包保持私有，还需要一个具有 `read:packages` 权限的 GitHub fine-grained PAT。

`wsserver` 依赖 ONNX Runtime 与 Opus。镜像已在构建阶段装入对应 Linux x86_64 运行库，因此无需在宿主机安装 Go、ONNX Runtime 或音频库。VAD 模型不进入镜像，Compose 将仓库的 `models/` 目录以只读 bind volume 挂载到 wsserver 的 `/app/models`；部署目录必须保留该目录及其模型文件。

## 首次上线

GitHub Actions 在推送 `main`、创建 `v*` tag 或手动触发时，分别构建并发布 `runtime`（manager、wsserver）和 `nginx`（manager 前端）镜像到 GHCR。服务器不编译源代码，只需保留 `deploy/`、`models/` 和 `deploy/.env`。

首次部署时，在仓库根目录执行：

```bash
cp deploy/.env.example deploy/.env
# 编辑 deploy/.env，替换 POSTGRES_PASSWORD、JWT_SECRET、ADMIN_PASSWORD。
# POSTGRES_PASSWORD 请使用 URL 安全字符（字母、数字、-、_）。
# 私有 GHCR 包：docker login ghcr.io -u <GitHub 用户名>
docker compose --env-file deploy/.env -f deploy/docker-compose.yml up -d
```

生产环境建议把 `ORION_X_TAG` 设为发布的版本 tag（例如 `v1.2.3`）或对应的 `sha-...` 标签，避免跟随 `latest` 自动变更。

检查服务：

```bash
docker compose --env-file deploy/.env -f deploy/docker-compose.yml ps
curl -f http://127.0.0.1/healthz
docker compose --env-file deploy/.env -f deploy/docker-compose.yml exec wsserver \
  curl -f http://127.0.0.1:8081/healthz
docker compose --env-file deploy/.env -f deploy/docker-compose.yml logs -f manager wsserver
```

浏览器访问 `http://<服务器域名或 IP>/`。首次登录使用 `ADMIN_USERNAME` 和 `ADMIN_PASSWORD`。管理员仅在数据库中不存在该用户名时创建；首次登录后请在 UI 中修改密码，并从 `deploy/.env` 删除 `ADMIN_PASSWORD`（重新创建容器不会重置已有账号）。

设备连接地址为 `ws://<服务器域名>/ws`；启用 HTTPS 后必须改为 `wss://<服务器域名>/ws`。

## HTTPS

此 Compose 文件只占用 HTTP 80 端口，以便适配已有的 Caddy、Nginx 或云负载均衡器。上线公网前，应让 TLS 终止层把 HTTPS/WSS 请求转发到 `127.0.0.1:${HTTP_PORT}`，并在前端与设备中统一使用域名和 `https`/`wss`。不要把 `postgres`、manager `9090` 或 wsserver `8080` 直接映射到宿主机端口。

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
