#!/bin/sh
# 停止 node-hunter 后端 / 前端
set -eu
ROOT="$(CDPATH= cd -- "$(dirname "$0")" && pwd)"
LOG_DIR="${LOG_DIR:-$ROOT/logs}"
API_PORT="${API_PORT:-8080}"

if [ -f "$LOG_DIR/server.pid" ]; then
  pid="$(cat "$LOG_DIR/server.pid" 2>/dev/null || true)"
  if [ -n "${pid:-}" ]; then
    kill "$pid" 2>/dev/null || true
    echo "stopped server pid $pid"
  fi
  rm -f "$LOG_DIR/server.pid"
fi
if [ -f "$LOG_DIR/web.pid" ]; then
  wpid="$(cat "$LOG_DIR/web.pid" 2>/dev/null || true)"
  if [ -n "${wpid:-}" ]; then
    kill "$wpid" 2>/dev/null || true
    # next 会再拉子进程
    pkill -P "$wpid" 2>/dev/null || true
    echo "stopped web pid $wpid"
  fi
  rm -f "$LOG_DIR/web.pid"
fi

# 只杀本项目，不碰占用 3000 的其它服务
pkill -f 'node-hunter-server' 2>/dev/null || true
pkill -f 'next dev.*node-hunter|/web/node_modules/next|next dev -H' 2>/dev/null || true
# 更精确：cwd 在本项目下的 next
pkill -f "${ROOT}/web" 2>/dev/null || true

echo "node-hunter stopped (API :${API_PORT} + Next.js)"

