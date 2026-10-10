#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

CMD=${1:-help}

case "$CMD" in
  init-config)
    if [ -f manager.yaml ]; then
      echo "deploy/manager.yaml already exists; not overwriting"
      exit 0
    fi
    cp ../manager.example.yaml manager.yaml
    chmod 600 manager.yaml
    echo "created deploy/manager.yaml (mode 0600)"
    echo "fill in database.dsn (same password as POSTGRES_PASSWORD), jwt.secret, admin.password and any optional sections before starting manager"
    ;;
  update)
    docker compose pull
    docker compose up -d --force-recreate
    docker compose ps
    ;;
  restart)
    docker compose restart
    docker compose ps
    ;;
  status|ps)
    docker compose ps
    ;;
  logs)
    shift || true
    docker compose logs --tail=50 -f "$@"
    ;;
  down)
    docker compose down
    ;;
  *)
    echo "Usage: $0 <command>"
    echo ""
    echo "Commands:"
    echo "  init-config    从 manager.example.yaml 生成 deploy/manager.yaml（已存在则跳过）"
    echo "  update         拉取最新镜像并重建服务"
    echo "  restart        重启服务"
    echo "  status|ps      查看服务状态"
    echo "  logs [service] 查看日志（可指定服务名）"
    echo "  down           停止并删除服务"
    ;;
esac
