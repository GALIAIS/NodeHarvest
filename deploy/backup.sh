#!/bin/bash
set -euo pipefail

APP="${APP_DIR:-/opt/nodeharvest}"
DEST="${BACKUP_DIR:-/opt/nodeharvest/backups}"
KEEP_DAYS="${BACKUP_KEEP_DAYS:-14}"
KEEP_COUNT="${BACKUP_KEEP_COUNT:-14}"
ENCRYPT="${BACKUP_ENCRYPT:-1}"
RECIPIENT="${BACKUP_AGE_RECIPIENT:-}"
STAMP=$(TZ=Asia/Shanghai date +%Y%m%d-%H%M%S)

case "$KEEP_DAYS" in ''|*[!0-9]*) echo "BACKUP_KEEP_DAYS must be a non-negative integer" >&2; exit 1 ;; esac
case "$KEEP_COUNT" in ''|*[!0-9]*) echo "BACKUP_KEEP_COUNT must be a positive integer" >&2; exit 1 ;; esac
if [ "$KEEP_COUNT" -lt 1 ]; then
  echo "BACKUP_KEEP_COUNT must be at least 1" >&2
  exit 1
fi
if [ "$ENCRYPT" = "1" ] && { [ -z "$RECIPIENT" ] || ! command -v age >/dev/null 2>&1; }; then
  echo "encrypted backup requires age and BACKUP_AGE_RECIPIENT" >&2
  exit 1
fi

mkdir -p "$DEST"
DEST=$(cd "$DEST" && pwd -P)
APP=$(cd "$APP" && pwd -P)
if [ "$DEST" = "/" ] || [ "$APP" = "/" ] || [ "$DEST" = "$APP" ]; then
  echo "unsafe app or backup directory" >&2
  exit 1
fi
TMP=$(mktemp -d "$DEST/.backup-$STAMP.XXXXXX")
case "$TMP" in "$DEST"/.backup-*) ;; *) echo "unsafe temp path: $TMP" >&2; exit 1 ;; esac
cleanup() { rm -rf -- "$TMP"; }
trap cleanup EXIT

mkdir -p "$TMP/data" "$TMP/output" "$TMP/configs" "$TMP/database"
if [ -d "$APP/data" ]; then cp -a "$APP/data/." "$TMP/data/"; fi
if [ -d "$APP/output" ]; then cp -a "$APP/output/." "$TMP/output/"; fi
if [ -f "$APP/configs/config.yaml" ]; then cp -a "$APP/configs/config.yaml" "$TMP/configs/config.yaml"; fi

DB="$APP/data/nodeharvest.db"
if command -v sqlite3 >/dev/null 2>&1 && [ -f "$DB" ]; then
  test "$(sqlite3 "$DB" 'PRAGMA quick_check;')" = "ok"
  rm -f -- "$TMP/data/nodeharvest.db" "$TMP/data/nodeharvest.db-wal" "$TMP/data/nodeharvest.db-shm"
  sqlite3 "$DB" ".backup '$TMP/data/nodeharvest.db'"
  test "$(sqlite3 "$TMP/data/nodeharvest.db" 'PRAGMA quick_check;')" = "ok"
fi
if [ -n "${NODE_HARVEST_DATABASE_URL:-}" ]; then
  command -v pg_dump >/dev/null 2>&1 || { echo "pg_dump is required for PostgreSQL backup" >&2; exit 1; }
  pg_dump --format=custom --no-owner --file="$TMP/database/postgres.dump" "$NODE_HARVEST_DATABASE_URL"
  pg_restore --list "$TMP/database/postgres.dump" >/dev/null
fi

ARCHIVE="$DEST/nh-backup-$STAMP.tar.gz"
tar czf "$ARCHIVE" -C "$TMP" data output configs database
gzip -t "$ARCHIVE"
tar tzf "$ARCHIVE" >/dev/null
if [ "$ENCRYPT" = "1" ]; then
  age -r "$RECIPIENT" -o "$ARCHIVE.age" "$ARCHIVE"
  rm -f -- "$ARCHIVE"
  ARCHIVE="$ARCHIVE.age"
fi
sha256sum "$ARCHIVE" >"$ARCHIVE.sha256"

find "$DEST" -maxdepth 1 -type f \
  \( -name 'nh-backup-*.tar.gz' -o -name 'nh-backup-*.tar.gz.age' -o -name 'nh-backup-*.sha256' \) \
  -mtime "+$KEEP_DAYS" -delete
mapfile -t old < <(find "$DEST" -maxdepth 1 -type f \
  \( -name 'nh-backup-*.tar.gz' -o -name 'nh-backup-*.tar.gz.age' \) -printf '%T@ %p\n' |
  sort -rn | tail -n "+$((KEEP_COUNT + 1))" | cut -d' ' -f2-)
for archive in "${old[@]}"; do
  rm -f -- "$archive" "$archive.sha256"
done

echo "backup: $ARCHIVE"
echo "verify: sha256sum -c $ARCHIVE.sha256"
