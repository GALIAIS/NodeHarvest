# NodeHarvest

面向生产的代理节点采集、真实拨测、质量治理与订阅分发平台。

> 仅在合法且已获授权的场景使用。采集端点来自独立第三方，不提供可用性、安全性或合法性保证。

## 已实现能力

- 131 个完整采集源目录，支持 URI、Base64、Clash YAML，带优先级、大小上限、健康分、自动冷却和单源探测。
- vmess、vless、trojan、ss、ssr、hysteria2、tuic 解析、去重、来源追踪和可选端口折叠。
- TCP/TLS 多轮质量测试，以及 sing-box / xray / Mihomo 独立进程真实代理 HTTP 下载测试。
- 可解释评分 v2：延迟、成功率、7 日稳定性、TLS、HTTP、吞吐量。
- SQLite 零服务运行；生产可切 PostgreSQL、Redis、MinIO/S3。
- 持久化优先级队列、租约、重试、死信、取消、独立 Worker 和事件时间线。
- 本地 bcrypt 账户、签名会话、viewer/operator/admin RBAC、多租户隔离。
- bcrypt 订阅 Token、国家/协议 ACL、RPS、日请求配额、流量统计、到期和吊销。
- global-hq、verified、streaming、AI-friendly、low-latency 及国家分区订阅。
- 完整接入 Sub-Store：订阅组合、过滤、脚本处理、格式转换、文件托管和多客户端输出。
- Prometheus、OpenTelemetry、Jaeger、结构化 request/job/trace ID、异常生命周期和签名 Webhook。
- 完整管理控制台：趋势、国家分布、源治理、任务事件、Token、用户、审计、告警、热配置和合规页。
- CI、SBOM、安全扫描、可复现镜像、Compose、Helm、加密备份、验证恢复、原子发布和回滚。

完整 API 定义见 [docs/openapi.yaml](docs/openapi.yaml)。

## 架构

```text
Public /sub ── Caddy/WAF ── API replicas ── Redis cache/lock
                                  │
Admin console ── local account/RBAC ─┤
                                  ├── PostgreSQL
Scheduler/API ── durable queue ───┤
                                  ├── Worker replicas ── sing-box/Mihomo
                                  └── S3/MinIO artifacts

Operator session ── Caddy auth ── Sub-Store (isolated origin + persistent data)

Prometheus ← /metrics       OTLP collector → Jaeger
```

单机模式保留相同执行路径：SQLite + 进程内缓存 + embedded worker，不要求外部服务。

## 环境

| 组件 | 版本 |
|---|---|
| Go | 1.26.5 |
| Node.js | 24（构建控制台） |
| Docker | Compose v2（生产栈） |
| Sub-Store | 2.36.22（生产 Compose） |
| 可选 | PostgreSQL 17、Redis 8、S3/MinIO、sing-box 1.13.12、Mihomo 1.19.29 或 xray |

## 本地启动

Windows：

```powershell
go run ./cmd/server -addr :8080 -config configs/config.yaml
cd web
npm ci
npm run dev
```

Linux/macOS：

```bash
go run ./cmd/server -addr :8080 -config configs/config.yaml
cd web
npm ci
npm run dev
```

打开 `http://127.0.0.1:3000`。开发模式通过 `API_ORIGIN` 将 `/api/*`
转发至 Go 服务；生产镜像使用静态导出并由 Go 服务同源托管。

默认配置启用 SQLite 和持久化队列。控制台不接受 Token 登录：管理面仅接受
本地账号密码，会话保存在 HttpOnly Cookie；订阅 Token 只用于 `/sub` 节点订阅。
先生成 bcrypt hash（例如 `htpasswd -nbBC 12 admin '你的长密码' | cut -d: -f2`），再启动：

```bash
export NODE_HARVEST_TOKEN="replace-with-subscription-secret"
export NODE_HARVEST_LOCAL_AUTH=1
export NODE_HARVEST_BOOTSTRAP_PASSWORD_HASH='$2y$...'
export NODE_HARVEST_SESSION_SECRET="at-least-32-random-characters"
go run ./cmd/server -addr :8080 -config configs/config.yaml
```

## 构建

```bash
go build -trimpath -o dist/nodeharvest-server ./cmd/server
go build -trimpath -o dist/nodeharvest-worker ./cmd/worker
go build -trimpath -o dist/nodeharvest-migrate ./cmd/migrate
go build -trimpath -o dist/nodeharvest ./cmd/nodeharvest
cd web
npm ci
npm run lint
STATIC_EXPORT=1 npm run build
```

## 容器镜像

`main` 分支自动发布 `ghcr.io/galiais/nodeharvest:edge`；`v*` 标签发布同名
版本镜像并创建 GitHub Release，也可从 Actions 手动指定源码引用和镜像标签。

```bash
docker pull ghcr.io/galiais/nodeharvest:edge
docker run --rm -p 8080:8080 \
  -e NODE_HARVEST_TOKEN="replace-with-subscription-secret" \
  -e NODE_HARVEST_LOCAL_AUTH=1 \
  -e NODE_HARVEST_BOOTSTRAP_PASSWORD_HASH="$NODE_HARVEST_BOOTSTRAP_PASSWORD_HASH" \
  -e NODE_HARVEST_SESSION_SECRET="at-least-32-random-characters" \
  ghcr.io/galiais/nodeharvest:edge
```

构建与标签规则见 [容器镜像说明](docs/CONTAINER_IMAGE.md)。

## 生产 Compose

`deploy/compose.prod.yml` 包含 PostgreSQL、Redis、MinIO、API、Worker、Sub-Store、Caddy、
Prometheus、Jaeger、OTel Collector、node-exporter 和 blackbox-exporter。

```bash
export POSTGRES_PASSWORD="..."
export NODE_HARVEST_TOKEN="..."
export NODE_HARVEST_BOOTSTRAP_PASSWORD_HASH="..."
export NODE_HARVEST_SESSION_SECRET="..."
export OBJECT_STORE_ACCESS_KEY="..."
export OBJECT_STORE_SECRET_KEY="..."
export NODE_HARVEST_IMAGE="ghcr.io/your-org/nodeharvest:stable"
export PUBLIC_DOMAIN="node.example.com"
export ADMIN_DOMAIN="admin.example.com"
export SUB_STORE_DOMAIN="store.example.com"
export SESSION_COOKIE_DOMAIN="example.com"
export SUB_STORE_BACKEND_PATH="/replace-with-a-random-path"
docker compose -f deploy/compose.prod.yml up -d
```

未登录浏览器仅可查看仪表盘；管理域名承载登录后的控制台及管理 API，`/sub*` 只接受
订阅 Token。详细步骤见
[deploy/DEPLOY.md](deploy/DEPLOY.md)。

Sub-Store 使用独立子域承载完整上游界面，operator/admin 的账号会话保护全部管理请求；
复制给订阅客户端的 `/share`、`/download` 地址仍需 NodeHarvest 节点 Token。配置、使用、
备份与安全边界见 [Sub-Store 集成指南](docs/SUB_STORE.md)。

## 订阅

```text
GET /sub/raw
GET /sub/base64
GET /sub/clash
GET /sub/pool/{key}/{raw|base64|clash}
GET /sub/country/{code}/{raw|base64|clash}
```

默认使用 `Authorization: Bearer TOKEN` 或 `X-Sub-Token: TOKEN`。URL
`?token=` 默认关闭，避免凭证进入浏览历史、Referer 和代理日志。

## 管理与权限

| 角色 | 能力 |
|---|---|
| viewer | 查看租户任务、事件、队列和系统告警 |
| operator | 启动/取消任务、探测源、刷新发布、处理告警 |
| admin | 管理 Token、用户及系统热配置 |

管理面可通过 `auth.admin_host` 与 `auth.admin_cidrs` 隔离。默认租户的 admin
才能修改系统级源和热配置。

## 数据与部署文档

- [运维手册](docs/RUNBOOK.md)
- [安全基线](docs/SECURITY.md)
- [部署指南](deploy/DEPLOY.md)
- [容器镜像](docs/CONTAINER_IMAGE.md)
- [Sub-Store 集成](docs/SUB_STORE.md)
- [第三方组件声明](THIRD_PARTY_NOTICES.md)
- [企业路线图（已完成）](docs/ENTERPRISE_ROADMAP.md)
- [OpenAPI 3.1](docs/openapi.yaml)

## 许可证

NodeHarvest 采用 [GNU Affero General Public License v3.0 or later](LICENSE)。
如果修改后通过网络向用户提供服务，需要向这些用户提供对应的修改后源码。
Sub-Store 仍按其 AGPL-3.0 独立授权，版本与对应源码见
[第三方组件声明](THIRD_PARTY_NOTICES.md)。
