#!/bin/sh
# 仅启动 Go API（适合已有前端 / 无 Node 场景）
set -eu
ROOT="$(CDPATH= cd -- "$(dirname "$0")" && pwd)"
cd "$ROOT"
API_ADDR="${API_ADDR:-127.0.0.1:8080}"
CONFIG="${CONFIG:-configs/config.yaml}"
DATA_DIR="${DATA_DIR:-data}"
export CGO_ENABLED="${CGO_ENABLED:-0}"
export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"

mkdir -p bin logs "$DATA_DIR" output
if [ ! -x bin/node-hunter-server ]; then
  go build -o bin/node-hunter-server ./cmd/server
fi
exec ./bin/node-hunter-server -addr "$API_ADDR" -config "$CONFIG" -data "$DATA_DIR"
