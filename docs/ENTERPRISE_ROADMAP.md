# node-hunter 企业级演进路线图

> 目标：从「可用的节点采集测速工具」升级为 **可运营、可观测、可扩展、高可用** 的订阅分发平台。  
> 当前版本（2026-07）：单进程内存库 + 文件快照 + systemd + Cloudflare 反代，已具备定时、筛选、国家分类、Token 订阅。

---

## 0. 现状画像（As-Is）

| 维度 | 现状 | 风险 |
|------|------|------|
| 架构 | 单体 Go HTTP + Next.js 控制台 | 采集/测速/API 争抢同一进程资源 |
| 存储 | 内存 map + `snapshot.json`（截断 ~8k） | 重启丢任务历史；无法水平扩展 |
| 任务 | 全局单飞（同时仅 1 个 job） | 长任务阻塞手动触发 |
| 安全 | 单 Token 查询参数 | Token 易进日志/Referer；无 RBAC |
| 订阅 | 同步现算 SelectPublish | 高并发时 CPU 尖刺 |
| 观测 | slog 文本 + journalctl | 无指标、无链路、无告警 |
| 发布 | 手工 scp 二进制 | 无版本、无回滚流水线 |
| 质量 | TCP/TLS 启发式 + 可选 SOCKS AI | 非真实代理吞吐；误报/漏报 |

---

## 1. 目标画像（To-Be）

```
                    ┌────────────── Cloudflare / WAF ──────────────┐
                    │  node.galiais.com (TLS, rate-limit, bot)     │
                    └───────────────┬──────────────────────────────┘
                                    │
              ┌─────────────────────┼─────────────────────┐
              ▼                     ▼                     ▼
        Admin API              Public /sub            Metrics
        (JWT/RBAC)           (CDN-cacheable)         /metrics
              │                     │                     │
              └──────────┬──────────┴──────────┬──────────┘
                         ▼                     ▼
                   API Gateway / Caddy    Redis (cache+lock)
                         │
          ┌──────────────┼──────────────┐
          ▼              ▼              ▼
     Worker:Fetch   Worker:Quality   Worker:Geo/AI
          │              │              │
          └──────┬───────┴──────┬───────┘
                 ▼              ▼
            PostgreSQL      Object Store
         (nodes/jobs/audit) (subs/artifacts)
```

**企业级验收标准（建议）**

- 可用性：订阅 API 月可用性 ≥ 99.9%
- 延迟：P95 `/sub/base64` < 200ms（缓存命中）
- 安全：Token 不落访问日志；支持轮换；管理面与订阅面分离
- 数据：节点/任务可查询历史 ≥ 30 天
- 发布：一键发布 + 一键回滚；配置变更可审计
- 质量：支持「真实协议拨测」抽样（xray/sing-box 出站）

---

## 2. 分阶段路线（建议 4 个里程碑）

### M1 — 稳基座 ✅ 已落地（2026-07-25 v2.0.0-enterprise）

**目标**：不推翻架构，把生产运维做到「敢睡」。

| 项 | 说明 | 状态 |
|----|------|------|
| 时区统一 | API/订阅/调度一律 `Asia/Shanghai` RFC3339 | ✅ |
| 健康检查 | `/api/health` 含 version、uptime、geo/sqlite/publish | ✅ |
| 就绪探针 | `/api/ready` | ✅ |
| 优雅退出 | SIGTERM 停调度、cancel job、flush 订阅缓存 | ✅ |
| 订阅预渲染 | `internal/publish` 原子缓存 + 磁盘 `sub.*` | ✅ |
| ETag / Cache-Control | 订阅响应 ETag + max-age | ✅ |
| Token 加固 | Bearer / X-Sub-Token；query 可关；日志脱敏 | ✅ |
| 限流 | sub_rps / api_rps 令牌桶 | ✅ |
| 配置校验 | 启动 validateConfig | ✅ |
| 备份 | `deploy/backup.sh` + cron 04:15 | ✅ |
| 版本号 | `/api/version` + ldflags | ✅ |
| Metrics | `/metrics` Prometheus 文本 | ✅ |

**交付物**：`GET /api/version`、预渲染订阅、限流中间件、部署 runbook。

### M2/M3/M4 — 同版本已实现精简落地

| 能力 | 实现 |
|------|------|
| SQLite 持久化 | jobs / tokens / audit / job_events（`data/node-hunter.db`） |
| 多 Token | `/api/admin/tokens` CRUD，国家 ACL、过期 |
| 审计 | `/api/admin/audit` |
| 评分 v2 | `quality.ApplyV2` 多因子可解释分 |
| 订阅池 | `/api/pools`（global-hq / ai-friendly / low-latency） |
| 文档 | `docs/RUNBOOK.md` `docs/SECURITY.md` |

---

### M2 — 数据与任务平台（2–4 周）

**目标**：可查询、可回放、可并行。

1. **持久化**
   - PostgreSQL：`nodes`、`node_metrics`、`jobs`、`job_events`、`sources`、`audit_logs`
   - 热数据 Redis：最新 HQ 列表、国家聚合、订阅 blob、分布式锁
2. **任务队列**
   - 用 Redis Stream / NATS / asynq 拆分：`fetch` / `quality` / `geo` / `ai` / `export`
   - 支持并发 worker、重试、死信队列、优先级（手动 > 定时）
3. **节点模型增强**
   - `first_seen` / `last_seen` / `fail_streak` / `success_streak`
   - 历史延迟分位（p50/p95）按天聚合
   - 国家、ASN、协议、入口类型（直连/CDN）
4. **去重升级**
   - 指纹：协议+server+port+uuid/password + 传输层参数
   - 可选「同 IP 多端口」折叠策略

**交付物**：迁移脚本、worker 二进制或同仓 subcommand、Admin 任务中心可看事件流。

---

### M3 — 企业安全与多租户（2–3 周）

**目标**：能卖、能管、能追责。

| 能力 | 设计要点 |
|------|----------|
| 管理认证 | OIDC / JWT；本地 admin 用户 bootstrap |
| RBAC | `viewer` / `operator` / `admin` |
| 订阅凭证 | 多 Token：名称、配额、过期、允许国家、允许协议、速率 |
| 审计 | 谁改了 config、谁触发 full、谁导出 |
| 密钥管理 | Token 哈希存储（bcrypt/argon2）；仅创建时回显明文 |
| 管理/订阅分离 | `admin.example.com` vs `node.example.com`；路径 ACL |
| 合规 | 配置开关：禁用特定国家源；ToS/免责声明页 |

**API 草图**

```
POST /api/v1/auth/login
GET  /api/v1/admin/tokens
POST /api/v1/admin/tokens
DELETE /api/v1/admin/tokens/{id}
GET  /api/v1/admin/audit?from=&to=
```

公开订阅保持：

```
GET /sub/{format}?token=  或  Authorization: Bearer
```

---

### M4 — 质量真测与智能运营（3–6 周）

**目标**：分数可信，运营可决策。

1. **真实协议拨测（抽样）**
   - Worker 拉起 xray/sing-box 临时入站/出站，测 HTTP 下载 100KB / TLS 握手 / 指定 AI 域名
   - 并发隔离（每节点独立端口或进程池），超时强杀
2. **评分模型 v2**
   - 多因子：连通率、延迟、抖动、TLS、真实 HTTP、协议完整度、稳定性（7 日）
   - 可配置权重 YAML；分数可解释（返回 breakdown）
3. **智能池**
   - 池类型：`global-hq` / `ai-friendly` / `streaming` / `by-country` / `low-latency`
   - 每个池独立订阅 URL 与刷新策略
4. **源治理**
   - 源健康分：连续失败自动降权/禁用
   - 源贡献度：提供 HQ 节点占比
5. **异常检测**
   - HQ 数量骤降、单国家占比异常、测速全灭 → 告警

---

## 3. 架构模块清单（目标仓库形态）

```
node-hunter/
  cmd/
    server/          # API + 订阅（无重任务）
    worker/          # 消费队列执行 fetch/quality/...
    node-hunter/     # CLI 运维工具
  internal/
    api/ v1/
    auth/
    billing/         # 可选：配额
    domain/          # 实体与仓储接口
    geo/
    quality/ v2/
    dialer/          # 真实拨测
    publish/         # 订阅渲染与缓存
    scheduler/
    observ/          # metrics tracing
  web/               # 控制台：仪表盘/源/任务/Token/审计
  deploy/
    helm/ 或 compose.prod.yml
    prometheus/rules.yml
  docs/
    ENTERPRISE_ROADMAP.md
    RUNBOOK.md
    SECURITY.md
```

---

## 4. 可观测性（必须有）

### 指标（Prometheus）

- `nh_nodes_total{grade,protocol,country,alive}`
- `nh_job_duration_seconds{type,status}`
- `nh_sub_requests_total{format,code,token_id}`
- `nh_sub_latency_seconds`
- `nh_source_fetch_errors_total{source}`
- `nh_geo_annotate_total{result}`

### 日志

- JSON 结构化；`request_id` / `job_id` 贯通
- 订阅日志：**禁止**打印完整 token（只打 token_id 前 6 位 hash）

### 链路

- OpenTelemetry → Tempo/Jaeger（job 跨 worker）

### 告警

- 订阅 5xx > 1% / 5m
- 定时 job 失败连续 2 次
- HQ 节点 < 阈值
- 磁盘 > 85% / 证书到期

---

## 5. 性能与稳定性设计

| 场景 | 策略 |
|------|------|
| 订阅风暴 | 预渲染 + 内存原子指针 + CF 缓存；进程内 singleflight |
| 测速打满 CPU | worker 独立部署；`GOMAXPROCS` 与并发配置分离 |
| 大源 OOM | 流式解析、按源上限、全局 unique 布隆/分片 |
| 重启 | snapshot 或 DB 热加载；`/sub` 仍可服务最后一版产物 |
| 发布 | 双缓冲二进制 + systemd；健康检查通过再切流量 |

**订阅热路径伪代码**

```go
// job 结束
blob := renderAllFormats(hqNodes)
atomic.StorePointer(&liveSub, blob)
writeFileAtomic("output/sub.base64", blob.Base64)

// HTTP
b := atomic.LoadPointer(&liveSub)
if ifNoneMatch == b.ETag { 304 }
write(b.Base64)
```

---

## 6. 安全基线（生产检查表）

- [ ] 管理 API 不暴露公网，或强制 mTLS / VPN / CF Access  
- [ ] 订阅 Token 可轮换、可吊销、可过期  
- [ ] HTTPS only；HSTS  
- [ ] 安全响应头（CSP 对控制台）  
- [ ] 依赖扫描（govulncheck、npm audit）  
- [ ] 密钥不进 git；`.env` 权限 600  
- [ ] 速率限制 + 防爆破  
- [ ] 备份加密；恢复演练每年/每季一次  
- [ ] 免责声明与用途限制（合规）  

---

## 7. 产品功能补全（控制台）

1. **仪表盘**：HQ 趋势图、国家地图/条形图、源健康、下次调度倒计时  
2. **节点库**：国家/协议/分数/AI 多维筛选；批量导出  
3. **源管理**：在线启用/禁用、探测单源、贡献排行  
4. **任务中心**：事件时间线、日志 tail、取消任务  
5. **Token 管理**：创建/吊销/配额/按国家 ACL  
6. **系统**：配置热更新（安全项需确认）、版本、GeoDB 状态  

---

## 8. 发布与环境

| 环境 | 用途 |
|------|------|
| dev | 本地 Minis / docker compose |
| staging | 小流量真实源，验证迁移 |
| prod | 当前 VPS + CF；远期双活 |

**CI 建议**

```
lint → test → build linux/amd64 → sbom → image/bin artifact
         → deploy staging → smoke (/health /sub)
         → manual approve → prod rolling
```

---

## 9. 90 天执行建议（务实排序）

### 第 1–2 周（立刻值回票价）

1. 订阅预渲染 + ETag + singleflight  
2. `/api/ready`、version、优雅退出  
3. Token 日志脱敏 + 基础限流  
4. Prometheus metrics + 1–2 条告警  
5. 自动备份 `data/output`  

### 第 3–6 周

6. PostgreSQL 持久化节点与 job  
7. 任务队列 + worker 分离  
8. 多 Token 与简单 Admin 登录  

### 第 7–12 周

9. 真实拨测抽样  
10. 评分 v2 + 多订阅池  
11. 控制台产品化（国家图、Token 页、审计）  
12. Helm/双环境与回滚演练  

---

## 10. 非目标（避免过早复杂）

- 不做「全球 Anycast 自建」——CF 足够  
- 不做完整计费中台——先配额与 Token  
- 不做全量真实拨测——先 5–10% 抽样  
- 不把控制台与订阅强绑同域——便于安全隔离  

---

## 11. 成功指标（KPI）

| KPI | 当前粗估 | 3 个月目标 |
|-----|----------|------------|
| 订阅 P95 延迟 | 未测（现算） | < 200ms 缓存命中 |
| 定时任务成功率 | 人工观察 | ≥ 99% |
| HQ 池日波动 | 大 | 告警可解释 |
| 故障恢复 RTO | 手工 scp | < 15 min 回滚 |
| Token 泄露响应 | 改 env | < 5 min 吊销 |

---

## 12. 与当前部署的衔接

- 域名：`https://node.galiais.com`  
- 进程：`/opt/node-hunter` + systemd  
- 下一步最小改动路径：**不拆服务**，先做 M1 预渲染/限流/metrics，再引入 Postgres/Redis。  

---

*文档版本：2026-07-25 · 与代码同步演进时更新本文件。*
