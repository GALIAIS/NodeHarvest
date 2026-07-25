#!/bin/bash
# 备份 data/ output/ 配置（脱敏 .env 中可选手动）
set -euo pipefail
APP="${APP_DIR:-/opt/node-hunter}"
DEST="${BACKUP_DIR:-/opt/node-hunter/backups}"
STAMP=$(TZ=Asia/Shanghai date +%Y%m%d-%H%M%S)
mkdir -p "$DEST"
TAR="$DEST/nh-backup-$STAMP.tar.gz"
tar czf "$TAR" \
  -C "$APP" \
  data \
  output \
  configs/config.yaml \
  --exclude='data/*.tmp' \
  --exclude='output/*.tmp' 2>/dev/null || \
tar czf "$TAR" -C "$APP" data output configs
# 保留最近 14 份
ls -1t "$DEST"/nh-backup-*.tar.gz 2>/dev/null | tail -n +15 | xargs -r rm -f
echo "backup: $TAR"
