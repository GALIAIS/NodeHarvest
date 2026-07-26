#!/bin/bash
set -euo pipefail

VERSION="${1:-}"
ARTIFACT_DIR="${2:-}"
APP="${APP_DIR:-/opt/nodeharvest}"
SERVICE="${SERVICE_NAME:-nodeharvest}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:8080/api/ready}"

[[ "$VERSION" =~ ^[A-Za-z0-9._-]+$ ]] || { echo "invalid version" >&2; exit 1; }
ARTIFACT_DIR=$(realpath "$ARTIFACT_DIR")
APP=$(realpath "$APP")
[ "$APP" != "/" ] || { echo "unsafe APP_DIR" >&2; exit 1; }
for binary in nodeharvest-server nodeharvest-worker nodeharvest; do
  [ -f "$ARTIFACT_DIR/$binary" ] || { echo "missing $binary" >&2; exit 1; }
done
if [ -f "$ARTIFACT_DIR/SHA256SUMS" ]; then
  (cd "$ARTIFACT_DIR" && sha256sum -c SHA256SUMS)
fi

RELEASE="$APP/releases/$VERSION"
[ ! -e "$RELEASE" ] || { echo "release already exists: $RELEASE" >&2; exit 1; }
mkdir -p "$RELEASE/bin" "$APP/releases"
install -m 0755 "$ARTIFACT_DIR/nodeharvest-server" "$RELEASE/bin/"
install -m 0755 "$ARTIFACT_DIR/nodeharvest-worker" "$RELEASE/bin/"
install -m 0755 "$ARTIFACT_DIR/nodeharvest" "$RELEASE/bin/"
if [ -d "$ARTIFACT_DIR/web" ]; then cp -a "$ARTIFACT_DIR/web" "$RELEASE/web"; fi

PREVIOUS=""
if [ -L "$APP/current" ]; then PREVIOUS=$(readlink -f "$APP/current"); fi
ln -s "$RELEASE" "$APP/current.next"
mv -Tf "$APP/current.next" "$APP/current"
if ! systemctl restart "$SERVICE" || ! curl --fail --silent --show-error --retry 12 --retry-delay 5 "$HEALTH_URL" >/dev/null; then
  if [ -n "$PREVIOUS" ]; then
    ln -s "$PREVIOUS" "$APP/current.rollback"
    mv -Tf "$APP/current.rollback" "$APP/current"
    systemctl restart "$SERVICE"
  fi
  echo "release failed and previous target was restored" >&2
  exit 1
fi
echo "released: $VERSION"
