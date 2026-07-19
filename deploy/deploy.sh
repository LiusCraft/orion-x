#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

CMD=${1:-help}

case "$CMD" in
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
    echo "  update         拉取最新镜像并重建服务"
    echo "  restart        重启服务"
    echo "  status|ps      查看服务状态"
    echo "  logs [service] 查看日志（可指定服务名）"
    echo "  down           停止并删除服务"
    ;;
esac
