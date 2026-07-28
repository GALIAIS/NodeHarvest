# NodeHarvest 部署指南

## 选择部署方式

| 场景 | 方式 | 数据层 |
|---|---|---|
| 本地/小型单机 | `deploy/docker-compose.yml` | SQLite |
| 单 VPS 完整生产栈 | `deploy/compose.prod.yml` | PostgreSQL + Redis + MinIO |
| Kubernetes | `deploy/helm/nodeharvest` | 外部 PostgreSQL/Redis/S3 |
| 传统 Linux | systemd + release 脚本 | SQLite 或外部服务 |

## 生产 Compose

准备 DNS：公开订阅、管理和 Sub-Store 三个域名指向 VPS。管理域名应再叠加 VPN、
Cloudflare Access 或 IP allowlist；Sub-Store 域名由 NodeHarvest 会话网关保护。

```bash
export POSTGRES_PASSWORD="..."
export NODE_HARVEST_TOKEN="..."
export NODE_HARVEST_BOOTSTRAP_PASSWORD_HASH="..."
export NODE_HARVEST_SESSION_SECRET="at-least-32-random-characters"
export OBJECT_STORE_ACCESS_KEY="..."
export OBJECT_STORE_SECRET_KEY="..."
export NODE_HARVEST_IMAGE="ghcr.io/your-org/nodeharvest:stable"
export PUBLIC_DOMAIN="node.example.com"
export ADMIN_DOMAIN="admin.example.com"
export SUB_STORE_DOMAIN="store.example.com"
export SESSION_COOKIE_DOMAIN="example.com"
export SUB_STORE_BACKEND_PATH="/replace-with-a-random-path"
docker compose -f deploy/compose.prod.yml config
docker compose -f deploy/compose.prod.yml up -d
```

服务包括 PostgreSQL 17、Redis 8、MinIO、API、Worker、Sub-Store、Caddy、OTel Collector、
Jaeger、Prometheus、node-exporter 和 blackbox-exporter。应用容器使用非 root、
只读根文件系统、无 Linux capabilities 和独立数据卷。

`SESSION_COOKIE_DOMAIN` 必须是 `ADMIN_DOMAIN` 与 `SUB_STORE_DOMAIN` 的共同受控父域。
Sub-Store 使用独立源站是因为其路由和 Service Worker 以 `/` 为作用域，不能安全挂载到
NodeHarvest 子路径。完整说明见 [Sub-Store 集成指南](../docs/SUB_STORE.md)。

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
  --from-literal=bootstrap-password-hash='...' \
  --from-literal=session-secret='...' \
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
| `NODE_HARVEST_LOCAL_AUTH` | 设为 `1` 启用本地账号密码登录 |
| `NODE_HARVEST_SESSION_SECRET` | 会话签名，至少 32 字符 |
| `SESSION_COOKIE_DOMAIN` | 管理域与 Sub-Store 域共同的 Cookie 父域 |
| `NODE_HARVEST_ADMIN_HOST` | 管理 API 允许的 Host；Compose 由 `ADMIN_DOMAIN` 注入 |
| `NODE_HARVEST_PUBLIC_HOST` | 节点订阅允许的 Host；Compose 由 `PUBLIC_DOMAIN` 注入 |
| `NODE_HARVEST_BOOTSTRAP_PASSWORD_HASH` | 本地 bootstrap bcrypt hash |
| `SUB_STORE_DOMAIN` | Sub-Store 独立 HTTPS 域名 |
| `SUB_STORE_BACKEND_PATH` | Sub-Store 合并后端的单段随机路径（以 `/` 开头） |
| `NODE_HARVEST_EMBEDDED_WORKERS` | API 进程内 Worker 数；多副本设为 0 |
| `NODE_HARVEST_OBJECT_STORE_ENDPOINT` | S3/MinIO endpoint |
| `NODE_HARVEST_OBJECT_STORE_ACCESS_KEY` | 对象存储 access key |
| `NODE_HARVEST_OBJECT_STORE_SECRET_KEY` | 对象存储 secret |
| `NODE_HARVEST_ALERT_WEBHOOK_SECRET` | 告警 HMAC secret |
| `NODE_HARVEST_ALLOW_QUERY_TOKEN` | 是否允许 `?token=` 携带订阅凭据，默认 `0` |
| `NODE_HARVEST_SUB_STORE_*` | Compose 自动注入的 Sub-Store 开关、URL、路径和版本 |
| `NODE_HARVEST_TRUSTED_PROXIES` | 逗号分隔的反代 CIDR；Compose 默认信任内部私网 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP/HTTP endpoint |

非密钥运行配置位于 `configs/config.yaml`。热配置只允许更新发布阈值、源治理阈值
和质量后真实拨测策略；拓扑、凭证、监听和数据库变更需滚动重启。

### 运行配置务必挂载

容器以 `-config configs/config.yaml` 启动，读的是**镜像内置的那份**。若不把宿主机
的配置挂载进去，每次镜像更新都会静默回到镜像里的默认值，而 `security.*` 一类设置
又不在热配置 API 的可改范围内（见 `internal/config/runtime.go`），届时只能重建镜像
才能改动。`deploy/compose.prod.yml` 与 `deploy/docker-compose.yml` 均已声明该只读挂载，
用 `docker run` 手工部署时请自行补上：

```bash
-v /opt/nodeharvest/configs/config.yaml:/app/configs/config.yaml:ro
```

### 订阅凭据的传递方式

`security.allow_query_token` 默认 `false`，此时 `?token=` 会被拒绝（401），客户端需改用
请求头：

```bash
curl -H "X-Sub-Token: $TOKEN" https://<domain>/sub/raw
curl -H "Authorization: Bearer $TOKEN" https://<domain>/sub/raw
```

但多数订阅客户端（Clash、代理池等）只能把凭据放进 URL。这类场景把
`NODE_HARVEST_ALLOW_QUERY_TOKEN` 设为 `1`（或在配置文件里置 `true`）放开，
代价是 token 会进入访问日志、浏览器历史与 Referer，应配合定期轮换。

**症状**：该项被关闭后，所有 `?token=` 订阅立即 401，下游代理池会冻结在上一次成功
拉取的快照上而不报错，容易被误判成"还在正常工作"。

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
