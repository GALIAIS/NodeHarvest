#!/bin/bash
set -euo pipefail

VERSION="${1:-}"
APP="${APP_DIR:-/opt/nodeharvest}"
SERVICE="${SERVICE_NAME:-nodeharvest}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:8080/api/ready}"
APP=$(realpath "$APP")
[ "$APP" != "/" ] || { echo "unsafe APP_DIR" >&2; exit 1; }

if [ -n "$VERSION" ]; then
  [[ "$VERSION" =~ ^[A-Za-z0-9._-]+$ ]] || { echo "invalid version" >&2; exit 1; }
  TARGET="$APP/releases/$VERSION"
else
  CURRENT=$(readlink -f "$APP/current")
  TARGET=$(find "$APP/releases" -mindepth 1 -maxdepth 1 -type d ! -path "$CURRENT" -printf '%T@ %p\n' |
    sort -rn | head -n 1 | cut -d' ' -f2-)
fi
[ -x "$TARGET/bin/nodeharvest-server" ] || { echo "rollback release not found" >&2; exit 1; }
ln -s "$TARGET" "$APP/current.rollback"
mv -Tf "$APP/current.rollback" "$APP/current"
systemctl restart "$SERVICE"
curl --fail --silent --show-error --retry 12 --retry-delay 5 "$HEALTH_URL" >/dev/null
echo "rolled back to: $(basename "$TARGET")"
