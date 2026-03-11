# Manager + Web 部署（宿主机 Nginx 转发）

这个编排只负责起服务，并把端口绑定到 `127.0.0.1`。域名、证书、路由都由宿主机现有 Nginx 管理。

## 1) 准备环境变量

```bash
cp deploy/manager-web.env.example deploy/manager-web.env
```

编辑 `deploy/manager-web.env`，至少替换：

- `MANAGER_DB_PASSWORD`
- `MANAGER_ACCESS_KEY_CIPHER_SECRET`
- `MANAGER_AUTH_JWT_SECRET`

可选端口（仅本机可访问）：

- `WEB_BIND_PORT`（默认 `18080`）
- `MANAGER_BIND_PORT`（默认 `18081`）

## 2) 启动服务

```bash
docker compose --env-file deploy/manager-web.env -f deploy/docker-compose.manager-web.yml up -d --build
```

默认监听：

- `web` -> `127.0.0.1:18080`
- `manager` -> `127.0.0.1:18081`

## 3) 配置宿主机 Nginx

参考模板：`deploy/nginx.manager-web.template.conf`

- `server_name` 按你的站点域名配置
- 证书路径按你现网根域名证书路径复用
- `/` 反代到 `http://127.0.0.1:<WEB_BIND_PORT>`（默认 `18080`）
- `/api/` 反代到 `http://127.0.0.1:<MANAGER_BIND_PORT>`（默认 `18081`）

## 4) 验证

```bash
docker compose --env-file deploy/manager-web.env -f deploy/docker-compose.manager-web.yml ps
docker compose --env-file deploy/manager-web.env -f deploy/docker-compose.manager-web.yml logs -f manager web
```
