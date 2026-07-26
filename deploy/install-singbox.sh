#!/bin/bash
# 在 VPS 上安装 sing-box 供 NodeHarvest 真实协议拨测使用
set -euo pipefail
APP="${APP_DIR:-/opt/nodeharvest}"
VER="${SINGBOX_VERSION:-1.13.12}"
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported arch: $ARCH"; exit 1 ;;
esac
mkdir -p "$APP/bin"
APP=$(cd "$APP" && pwd -P)
if [ "$APP" = "/" ]; then
  echo "unsafe app directory"
  exit 1
fi
TMP=$(mktemp -d /tmp/nodeharvest-singbox.XXXXXX)
case "$TMP" in
  /tmp/nodeharvest-singbox.*) ;;
  *) echo "unsafe temp path: $TMP"; exit 1 ;;
esac
cleanup() { rm -rf -- "$TMP"; }
trap cleanup EXIT
cd "$TMP"
URL="https://github.com/SagerNet/sing-box/releases/download/v${VER}/sing-box-${VER}-linux-${ARCH}.tar.gz"
echo "download $URL"
curl --proto '=https' --tlsv1.2 -fL --retry 3 -o sb.tgz "$URL"
EXPECTED="${SINGBOX_SHA256:-}"
if [ -z "$EXPECTED" ] && [ "$VER" = "1.13.12" ]; then
  case "$ARCH" in
    amd64) EXPECTED=1540533adb3df24f5ad5f14b5c7ca3dbc2401b10a1c1eb278fcadcada47ec6c4 ;;
    arm64) EXPECTED=1ffa3b48ad6fa98f9fd810482e39bdd5b6157782ef11ce37d67bdcfd9338547a ;;
  esac
fi
if [ -z "$EXPECTED" ]; then
  echo "SINGBOX_SHA256 is required for non-default versions" >&2
  exit 1
fi
printf '%s  %s\n' "$EXPECTED" sb.tgz | sha256sum -c -
gzip -t sb.tgz
tar xzf sb.tgz
install -m 755 sing-box-*/sing-box "$APP/bin/sing-box"
"$APP/bin/sing-box" version | head -3
# patch config dial.bin if present
if [ -f "$APP/configs/config.yaml" ]; then
  sed -i 's|bin: ""|bin: "'"$APP/bin/sing-box"'"|' "$APP/configs/config.yaml" || true
fi
chown nodeharvest:nodeharvest "$APP/bin/sing-box" 2>/dev/null || true
echo "OK: $APP/bin/sing-box"
