#!/bin/bash
set -euo pipefail

ARCHIVE="${1:-}"
APP="${APP_DIR:-/opt/nodeharvest}"
IDENTITY="${BACKUP_AGE_IDENTITY:-}"
CONFIRM="${RESTORE_CONFIRM:-}"
RESTORE_CONFIG="${RESTORE_CONFIG:-0}"
SERVICE="${SERVICE_NAME:-nodeharvest}"
STOPPED=0

if [ -z "$ARCHIVE" ] || [ "$CONFIRM" != "YES" ]; then
  echo "usage: RESTORE_CONFIRM=YES BACKUP_AGE_IDENTITY=/path/key $0 backup.tar.gz[.age]" >&2
  exit 1
fi
ARCHIVE=$(realpath "$ARCHIVE")
APP=$(realpath "$APP")
if [ "$APP" = "/" ] || [ ! -f "$ARCHIVE" ] || [ ! -f "$ARCHIVE.sha256" ]; then
  echo "unsafe app path or missing archive/checksum" >&2
  exit 1
fi
(cd "$(dirname "$ARCHIVE")" && sha256sum -c "$(basename "$ARCHIVE").sha256")

TMP=$(mktemp -d "${TMPDIR:-/tmp}/nodeharvest-restore.XXXXXX")
cleanup() {
  rm -rf -- "$TMP"
  if [ "$STOPPED" = "1" ]; then systemctl start "$SERVICE" || true; fi
}
trap cleanup EXIT
TAR="$ARCHIVE"
if [[ "$ARCHIVE" = *.age ]]; then
  command -v age >/dev/null 2>&1 || { echo "age is required" >&2; exit 1; }
  [ -n "$IDENTITY" ] || { echo "BACKUP_AGE_IDENTITY is required" >&2; exit 1; }
  TAR="$TMP/backup.tar.gz"
  age -d -i "$IDENTITY" -o "$TAR" "$ARCHIVE"
fi
gzip -t "$TAR"
while IFS= read -r entry; do
  case "$entry" in /*|../*|*/../*|*/..) echo "unsafe archive entry: $entry" >&2; exit 1 ;; esac
done < <(tar tzf "$TAR")
tar xzf "$TAR" -C "$TMP"
if command -v sqlite3 >/dev/null 2>&1 && [ -f "$TMP/data/nodeharvest.db" ]; then
  test "$(sqlite3 "$TMP/data/nodeharvest.db" 'PRAGMA quick_check;')" = "ok"
fi
if [ -f "$TMP/database/postgres.dump" ]; then
  [ -n "${NODE_HARVEST_DATABASE_URL:-}" ] || { echo "NODE_HARVEST_DATABASE_URL is required" >&2; exit 1; }
  pg_restore --list "$TMP/database/postgres.dump" >/dev/null
fi

STAMP=$(date +%Y%m%d-%H%M%S)
ROLLBACK="$APP/restore-rollback-$STAMP"
if systemctl is-active --quiet "$SERVICE"; then
  systemctl stop "$SERVICE"
  STOPPED=1
fi
mkdir -p "$ROLLBACK"
for name in data output; do
  if [ -e "$APP/$name" ]; then mv -- "$APP/$name" "$ROLLBACK/$name"; fi
  if [ -d "$TMP/$name" ]; then mv -- "$TMP/$name" "$APP/$name"; else mkdir -p "$APP/$name"; fi
done
if [ "$RESTORE_CONFIG" = "1" ] && [ -f "$TMP/configs/config.yaml" ]; then
  mkdir -p "$ROLLBACK/configs" "$APP/configs"
  if [ -f "$APP/configs/config.yaml" ]; then
    mv -- "$APP/configs/config.yaml" "$ROLLBACK/configs/config.yaml"
  fi
  mv -- "$TMP/configs/config.yaml" "$APP/configs/config.yaml"
fi
if [ -f "$TMP/database/postgres.dump" ]; then
  pg_restore --clean --if-exists --no-owner --dbname="$NODE_HARVEST_DATABASE_URL" "$TMP/database/postgres.dump"
fi
if [ "$STOPPED" = "1" ]; then
  systemctl start "$SERVICE"
  STOPPED=0
fi
echo "restore complete; previous files: $ROLLBACK"
