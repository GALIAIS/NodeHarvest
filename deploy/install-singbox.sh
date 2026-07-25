#!/bin/bash
# 在 VPS 上安装 sing-box 供 node-hunter 真实协议拨测使用
set -euo pipefail
APP="${APP_DIR:-/opt/node-hunter}"
VER="${SINGBOX_VERSION:-1.11.7}"
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported arch: $ARCH"; exit 1 ;;
esac
mkdir -p "$APP/bin" /tmp/sb-install
cd /tmp/sb-install
URL="https://github.com/SagerNet/sing-box/releases/download/v${VER}/sing-box-${VER}-linux-${ARCH}.tar.gz"
echo "download $URL"
if ! curl -fL --retry 3 -o sb.tgz "$URL"; then
  curl -fL --retry 3 -o sb.tgz "https://ghproxy.net/${URL}"
fi
tar xzf sb.tgz
install -m 755 sing-box-*/sing-box "$APP/bin/sing-box"
"$APP/bin/sing-box" version | head -3
# env for service
ENVF="$APP/.env"
touch "$ENVF"
grep -q NODE_HUNTER_SINGBOX "$ENVF" 2>/dev/null || echo "NODE_HUNTER_SINGBOX=$APP/bin/sing-box" >> "$ENVF"
# patch config dial.bin if present
if [ -f "$APP/configs/config.yaml" ]; then
  sed -i 's|bin: ""|bin: "'"$APP/bin/sing-box"'"|' "$APP/configs/config.yaml" || true
fi
chown -R nodehunter:nodehunter "$APP/bin/sing-box" 2>/dev/null || true
echo "OK: $APP/bin/sing-box"
