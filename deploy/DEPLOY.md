# NodeHarvest 部署指南

## 选择部署方式

| 场景 | 方式 | 数据层 |
|---|---|---|
| 本地/小型单机 | `deploy/docker-compose.yml` | SQLite |
| 单 VPS 完整生产栈 | `deploy/compose.prod.yml` | PostgreSQL + Redis + MinIO |
| Kubernetes | `deploy/helm/nodeharvest` | 外部 PostgreSQL/Redis/S3 |
| 传统 Linux | systemd + release 脚本 | SQLite 或外部服务 |

## 生产 Compose

准备 DNS：公开订阅域名指向 VPS，管理域名应再叠加 VPN、Cloudflare Access 或 IP allowlist。

```bash
export POSTGRES_PASSWORD="..."
export NODE_HARVEST_TOKEN="..."
export NODE_HARVEST_ADMIN_TOKEN="..."
export NODE_HARVEST_SESSION_SECRET="at-least-32-random-characters"
export OBJECT_STORE_ACCESS_KEY="..."
export OBJECT_STORE_SECRET_KEY="..."
export NODE_HARVEST_IMAGE="ghcr.io/your-org/nodeharvest:stable"
export PUBLIC_DOMAIN="node.example.com"
export ADMIN_DOMAIN="admin.example.com"
docker compose -f deploy/compose.prod.yml config
docker compose -f deploy/compose.prod.yml up -d
```

服务包括 PostgreSQL 17、Redis 8、MinIO、API、Worker、Caddy、OTel Collector、
Jaeger、Prometheus、node-exporter 和 blackbox-exporter。应用容器使用非 root、
只读根文件系统、无 Linux capabilities 和独立数据卷。

验证：

```bash
BASE_URL="https://node.example.com" SUB_TOKEN="$NODE_HARVEST_TOKEN" \
  bash deploy/smoke.sh
docker compose -f deploy/compose.prod.yml ps
```

Jaeger 和 Prometheus 默认只绑定 `127.0.0.1`，通过 SSH 隧道或受保护的管理网访问。

## Kubernetes / Helm

Chart 不创建数据库、Redis 或 Secret。先创建外部依赖和 Secret：

```bash
kubectl -n nodeharvest create secret generic nodeharvest-secrets \
  --from-literal=database-url='postgres://...' \
  --from-literal=redis-url='redis://...' \
  --from-literal=subscription-token='...' \
  --from-literal=admin-token='...' \
  --from-literal=session-secret='...' \
  --from-literal=oidc-client-secret='...' \
  --from-literal=alert-webhook-secret='...' \
  --from-literal=object-store-access-key='...' \
  --from-literal=object-store-secret-key='...'
```

渲染并部署：

```bash
helm lint deploy/helm/nodeharvest
helm template nodeharvest deploy/helm/nodeharvest \
  -f deploy/helm/nodeharvest/values-prod.yaml
helm upgrade --install nodeharvest deploy/helm/nodeharvest \
  --namespace nodeharvest --create-namespace \
  -f deploy/helm/nodeharvest/values-prod.yaml \
  --set image.repository=ghcr.io/your-org/nodeharvest \
  --set image.tag=VERSION \
  --set hosts.public=node.example.com \
  --set hosts.admin=admin.example.com \
  --atomic --wait
```

Chart 包含：

- API 与 Worker Deployment；
- `pre-install`/`pre-upgrade` 数据库迁移 Job；
- public/admin 双 Ingress；
- readiness/liveness/startup probes；
- HPA、PDB、NetworkPolicy、ServiceMonitor；
- 非 root、只读根文件系统和最小 ServiceAccount；
- 可选 ConfigMap 与 PVC。

生产 `persistence.enabled=false` 时节点真相在 PostgreSQL、发布产物在对象存储；
只有选择本地文件持久化时才需要支持多副本写入的 RWX volume。

## systemd 原子发布

构建 artifact：

```bash
CGO_ENABLED=0 go build -trimpath -o dist/nodeharvest-server ./cmd/server
CGO_ENABLED=0 go build -trimpath -o dist/nodeharvest-worker ./cmd/worker
CGO_ENABLED=0 go build -trimpath -o dist/nodeharvest-migrate ./cmd/migrate
CGO_ENABLED=0 go build -trimpath -o dist/nodeharvest ./cmd/nodeharvest
cd web
npm ci
STATIC_EXPORT=1 npm run build
mkdir -p ../dist/web
cp -a out/. ../dist/web/
```

首次安装：

```bash
sudo useradd -r -s /usr/sbin/nologin nodeharvest || true
sudo install -d -o nodeharvest -g nodeharvest /opt/nodeharvest/{data,output,releases}
sudo install -d -m 0750 /etc/nodeharvest
sudo install -m 0600 /dev/null /etc/nodeharvest/nodeharvest.env
sudo install -m 0644 deploy/nodeharvest.service /etc/systemd/system/nodeharvest.service
sudo systemctl daemon-reload
```

将密钥写入 `/etc/nodeharvest/nodeharvest.env` 后发布：

```bash
sudo APP_DIR=/opt/nodeharvest bash deploy/release.sh VERSION ./dist
sudo systemctl enable nodeharvest
```

release 脚本创建 `/opt/nodeharvest/releases/VERSION`、原子切换 `current`，
重启并检查 `/api/ready`；失败自动恢复上一软链接。

回滚：

```bash
sudo APP_DIR=/opt/nodeharvest bash deploy/rollback.sh VERSION
```

## 配置注入

生产环境常用变量：

| 变量 | 作用 |
|---|---|
| `NODE_HARVEST_DATABASE_URL` | PostgreSQL DSN，并自动选择 postgres driver |
| `NODE_HARVEST_REDIS_URL` | Redis URL |
| `NODE_HARVEST_TOKEN` | master 订阅凭证 |
| `NODE_HARVEST_ADMIN_TOKEN` | 应急管理凭证 |
| `NODE_HARVEST_SESSION_SECRET` | 会话签名，至少 32 字符 |
| `NODE_HARVEST_BOOTSTRAP_PASSWORD_HASH` | 本地 bootstrap bcrypt hash |
| `NODE_HARVEST_OIDC_CLIENT_SECRET` | OIDC client secret |
| `NODE_HARVEST_EMBEDDED_WORKERS` | API 进程内 Worker 数；多副本设为 0 |
| `NODE_HARVEST_OBJECT_STORE_ENDPOINT` | S3/MinIO endpoint |
| `NODE_HARVEST_OBJECT_STORE_ACCESS_KEY` | 对象存储 access key |
| `NODE_HARVEST_OBJECT_STORE_SECRET_KEY` | 对象存储 secret |
| `NODE_HARVEST_ALERT_WEBHOOK_SECRET` | 告警 HMAC secret |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP/HTTP endpoint |

非密钥运行配置位于 `configs/config.yaml`。热配置只允许更新发布阈值、源治理阈值
和质量后真实拨测策略；拓扑、凭证、监听和数据库变更需滚动重启。

## 发布流水线

`.github/workflows/ci.yml`：

```text
format → race/unit/integration/vet → frontend lint/build/audit
       → govulncheck/gosec → SBOM/artifact → image → staging smoke
```

`.github/workflows/release.yml` 通过受保护的 production environment 手动批准，
发布 GHCR 多架构镜像、provenance 与 SBOM。

## 备份与恢复

```bash
BACKUP_AGE_RECIPIENT="age1..." APP_DIR=/opt/nodeharvest bash deploy/backup.sh
RESTORE_CONFIRM=YES BACKUP_AGE_IDENTITY=/secure/age.key \
  APP_DIR=/opt/nodeharvest bash deploy/restore.sh /path/backup.tar.gz.age
```

恢复会保留旧目录用于人工回退。完整演练步骤见 `docs/RUNBOOK.md`。
