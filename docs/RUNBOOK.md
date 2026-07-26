# NodeHarvest 运维手册

## 服务拓扑

生产推荐 API 与 Worker 分离：

```text
Caddy/WAF → nodeharvest-server → PostgreSQL
                           ├──→ Redis
                           ├──→ MinIO/S3
                           └──→ durable queue → nodeharvest-worker
```

单机 systemd 可使用 embedded worker；多副本部署必须将
`NODE_HARVEST_EMBEDDED_WORKERS=0`，由独立 Worker 消费队列。

## 日常检查

```bash
curl --fail http://127.0.0.1:8080/api/health
curl --fail http://127.0.0.1:8080/api/ready
curl --fail http://127.0.0.1:8080/api/version
curl --fail -H "Authorization: Bearer $SUB_TOKEN" \
  http://127.0.0.1:8080/sub/meta
```

`/api/health` 展示数据库、Redis、发布缓存、GeoDB、源异常数与运行任务；
`/api/ready` 还验证依赖连接和导出目录可写性。编排平台应使用 ready 作为流量门禁。

systemd：

```bash
systemctl status nodeharvest
journalctl -u nodeharvest --since "30 min ago"
systemctl restart nodeharvest
```

Compose：

```bash
docker compose -f deploy/compose.prod.yml ps
docker compose -f deploy/compose.prod.yml logs --since=30m nodeharvest worker
```

## 任务与队列

控制台“任务中心”显示 Job、持久化 Task、Worker 租约、重试次数和事件流。

```bash
curl -H "Authorization: Bearer $ADMIN" \
  http://127.0.0.1:8080/api/admin/queue
curl -H "Authorization: Bearer $ADMIN" \
  "http://127.0.0.1:8080/api/admin/tasks?status=dead"
curl -X POST -H "Authorization: Bearer $ADMIN" \
  http://127.0.0.1:8080/api/admin/tasks/JOB_ID/cancel
```

排查顺序：

1. `queued` 持续增长：确认 Worker 在线、数据库一致、租约轮询无错误。
2. `running` 长时间不动：检查事件流与 Worker 日志中的 `job_id`、`request_id`、`trace_id`。
3. `dead` 增长：读取 `last_error`，修复根因后重新触发任务；不要直接改库状态。
4. 租约过期：任务会自动回队列，达到最大尝试次数后进入死信。

## 源治理

源连续失败达到阈值后进入冷却；成功探测会更新延迟、成功率、贡献度和健康分。

```bash
curl -H "Authorization: Bearer $ADMIN" \
  "http://127.0.0.1:8080/api/sources?sort=health"
curl -X POST -H "Authorization: Bearer $ADMIN" \
  http://127.0.0.1:8080/api/admin/sources/SOURCE/probe
```

手工停源属于系统级动作，只有 default 租户 admin 可执行，并写入审计。

## 告警

Prometheus 规则位于 `deploy/prometheus/rules.yml`，覆盖：

- 订阅 5xx 比例和 P95 延迟；
- 关键异常、HQ 供给不足和死信队列；
- 磁盘使用率、端点不可达和证书到期。

应用异常在 `/api/admin/alerts` 中持久化，可确认和解决；配置 Webhook 后发送
`X-NodeHarvest-Signature: sha256=...` HMAC 签名。

## 备份

备份默认强制使用 age 加密，并同时验证 SQLite、PostgreSQL dump、gzip、tar 和 SHA-256。

```bash
export APP_DIR=/opt/nodeharvest
export BACKUP_AGE_RECIPIENT="age1..."
export BACKUP_KEEP_DAYS=14
export BACKUP_KEEP_COUNT=14
bash deploy/backup.sh
```

PostgreSQL 模式还需导出 `NODE_HARVEST_DATABASE_URL` 并安装 `pg_dump`/`pg_restore`。
备份目录应复制到异机或对象存储，age 私钥不得与备份共存。

## 恢复演练

恢复会校验 checksum、解密、拒绝路径穿越、验证 SQLite/PG dump，并将旧
`data`/`output` 移入带时间戳的回退目录。

```bash
export RESTORE_CONFIRM=YES
export BACKUP_AGE_IDENTITY=/secure/path/age.key
export APP_DIR=/opt/nodeharvest
bash deploy/restore.sh /opt/nodeharvest/backups/nh-backup-YYYYMMDD-HHMMSS.tar.gz.age
```

恢复后必须运行：

```bash
BASE_URL=http://127.0.0.1:8080 SUB_TOKEN="$SUB_TOKEN" bash deploy/smoke.sh
```

每季度至少执行一次隔离环境恢复演练并记录 RTO/RPO。

## 原子发布与回滚

CI 产物目录应包含 server、worker、migrate、CLI 四个二进制及 `web/` 静态目录：

```bash
APP_DIR=/opt/nodeharvest bash deploy/release.sh VERSION /path/to/artifact
```

脚本写入 `/opt/nodeharvest/releases/VERSION`，原子切换 `current` 软链接，
重启并检查 readiness；失败时自动切回上一版本。

显式回滚：

```bash
APP_DIR=/opt/nodeharvest bash deploy/rollback.sh VERSION
# 不传 VERSION 时选择当前版本之外最新的 release
```

Helm 在 install/upgrade 前运行 `nodeharvest-migrate`；其他部署由 server/worker
启动时执行同一套幂等迁移。数据库迁移保持向后兼容；若未来引入破坏性迁移，
必须先提供独立回退迁移。

## 事故处理

### 订阅 5xx

1. 查 `/api/health` 与 `/api/ready`。
2. 查 `nh_sub_requests_total`、`nh_sub_latency_seconds` 和应用日志。
3. 若数据库/Redis异常，发布内存及磁盘最后版本仍应可用；确认对象存储快照。
4. 必要时回滚 release，不删除当前数据目录。

### HQ 数量骤降

1. 查看 `high-quality-drop`、`all-quality-probes-failed` 告警详情。
2. 按健康分检查源，确认网络或上游是否同时故障。
3. 对少量代表源执行单源探测。
4. 不要在根因未知时降低评分阈值并直接发布。

### 凭证泄漏

1. 在 Token 页面立即停用或删除受影响 Token。
2. 若为 master/admin/session secret，替换部署 secret 并滚动重启。
3. 检查审计、访问日志和今日用量。
4. 对曾进入 Git 的密钥执行提供商侧轮换；历史清理需单独授权和协作。
