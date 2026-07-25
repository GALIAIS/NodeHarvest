#!/bin/sh
# Node Hunter 一键启动：后端 :8080 + Next.js :3000
# 适配 Alpine/Linux/Minis (aarch64)
set -eu

ROOT="$(CDPATH= cd -- "$(dirname "$0")" && pwd)"
cd "$ROOT"

API_ADDR="${API_ADDR:-127.0.0.1:8080}"
WEB_HOST="${WEB_HOST:-127.0.0.1}"
WEB_PORT="${WEB_PORT:-3000}"
CONFIG="${CONFIG:-configs/config.yaml}"
DATA_DIR="${DATA_DIR:-data}"
LOG_DIR="${LOG_DIR:-logs}"

mkdir -p "$LOG_DIR" bin "$DATA_DIR" output

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "缺少命令: $1" >&2
    exit 1
  }
}

need_cmd go
need_cmd node
need_cmd npm
need_cmd curl

export CGO_ENABLED="${CGO_ENABLED:-0}"
export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"
export GO111MODULE=on

echo "==> 构建后端 (linux/$(go env GOARCH))..."
go mod tidy
go build -o bin/node-hunter-server ./cmd/server
go build -o bin/node-hunter ./cmd/node-hunter

port_in_use() {
  port="$1"
  if command -v ss >/dev/null 2>&1; then
    ss -lnt 2>/dev/null | grep -qE "[:.]${port}[[:space:]]"
    return $?
  fi
  # BusyBox netstat 或 curl 探测
  if command -v netstat >/dev/null 2>&1; then
    netstat -lnt 2>/dev/null | grep -qE "[:.]${port}[[:space:]]"
    return $?
  fi
  # 已有监听时 curl 会立刻连上
  curl -fsS --connect-timeout 0.2 "http://127.0.0.1:${port}/" >/dev/null 2>&1
}

free_port() {
  port="$1"
  # 仅杀本项目相关进程，避免误杀 Sub-Store 等占用 3000 的服务
  pkill -f "node-hunter-server.*:${port}" 2>/dev/null || true
  pkill -f "next dev.*-p ${port}" 2>/dev/null || true
  if [ -f "$LOG_DIR/server.pid" ] && [ "$port" = "${API_ADDR##*:}" ]; then
    old="$(cat "$LOG_DIR/server.pid" 2>/dev/null || true)"
    [ -n "${old:-}" ] && kill "$old" 2>/dev/null || true
  fi
}

pick_web_port() {
  preferred="$1"
  if ! port_in_use "$preferred"; then
    echo "$preferred"
    return
  fi
  # 3000 常被 Sub-Store 等占用，自动换端口
  for p in 3737 3030 3001 5173 8081; do
    if ! port_in_use "$p"; then
      echo "警告: 端口 ${preferred} 已被占用，前端改用 ${p}" >&2
      echo "$p"
      return
    fi
  done
  echo "无可用前端端口" >&2
  exit 1
}

echo "==> 检查端口..."
free_port "${API_ADDR##*:}"
WEB_PORT="$(pick_web_port "$WEB_PORT")"
API_ORIGIN="${API_ORIGIN:-http://${API_ADDR}}"
sleep 0.2


if [ -f "$LOG_DIR/server.pid" ]; then
  old="$(cat "$LOG_DIR/server.pid" 2>/dev/null || true)"
  [ -n "${old:-}" ] && kill "$old" 2>/dev/null || true
  rm -f "$LOG_DIR/server.pid"
fi

echo "==> 启动 API: http://${API_ADDR}"
# 后台启动时必须重定向，避免 shell 退出后 SIGPIPE
nohup ./bin/node-hunter-server \
  -addr "$API_ADDR" \
  -config "$CONFIG" \
  -data "$DATA_DIR" \
  >"$LOG_DIR/server.log" 2>&1 &
echo $! >"$LOG_DIR/server.pid"

# 健康检查
ok=0
i=0
while [ "$i" -lt 30 ]; do
  if curl -fsS "http://${API_ADDR}/api/health" >/dev/null 2>&1; then
    ok=1
    break
  fi
  i=$((i + 1))
  sleep 0.2
done
if [ "$ok" -ne 1 ]; then
  echo "后端启动失败，日志：" >&2
  tail -n 40 "$LOG_DIR/server.log" >&2 || true
  exit 1
fi
echo "    健康检查 OK  (pid $(cat "$LOG_DIR/server.pid"))"

echo "==> 准备前端依赖..."
cd "$ROOT/web"
if [ ! -d node_modules ] || [ ! -e node_modules/next/dist/bin/next ]; then
  npm install --no-fund --no-audit
fi
# 若只装了 win32 SWC，强制补 linux arm64 原生包
if [ ! -d node_modules/@next/swc-linux-arm64-gnu ] && [ ! -d node_modules/@next/swc-linux-arm64-musl ]; then
  echo "    检测到缺少 linux SWC，重新 npm install..."
  rm -rf node_modules
  npm install --no-fund --no-audit
fi

export API_ORIGIN
echo ""
echo "打开浏览器: http://${WEB_HOST}:${WEB_PORT}"
echo "API 健康检查: http://${API_ADDR}/api/health"
echo "后端日志: $ROOT/$LOG_DIR/server.log"
echo "停止: $ROOT/stop.sh  或 Ctrl+C（仅停前端，后端需 stop.sh）"
echo ""

# 前台跑前端，便于看日志；Ctrl+C 后可选手动 stop
exec npm run dev -- -H "$WEB_HOST" -p "$WEB_PORT"
