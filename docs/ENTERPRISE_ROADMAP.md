# NodeHarvest 企业级路线图

状态：**仓库范围全部完成**（2026-07-26）。

## 验收矩阵

| 里程碑 | 能力 | 状态 | 主要交付 |
|---|---|---|---|
| M1 稳基座 | 探针、优雅退出、原子发布缓存、ETag、限流、备份、指标 | 完成 | `internal/publish`、`internal/metrics`、`deploy/backup.sh` |
| M2 数据与任务 | PostgreSQL、Redis、节点历史、持久化队列、独立 Worker、死信 | 完成 | `internal/db`、`internal/hotcache`、`cmd/worker`、migrations |
| M3 企业安全 | 本地账户、签名会话、RBAC、多租户、Token 配额与 ACL、审计、热配置 | 完成 | `internal/auth`、管理 API、管理控制台 |
| M4 质量运营 | sing-box/xray 真测、吞吐量、评分 v2、智能池、源治理、异常检测 | 完成 | `internal/dialer`、`internal/quality`、`internal/pools` |
| Operations | OTel、告警、CI、SBOM、Compose、Helm、发布/回滚/恢复 | 完成 | `.github/workflows`、`deploy/` |
| Product | 趋势、国家、源控制、事件流、Token、用户、审计、告警、系统、合规 | 完成 | `web/src/app` |
| API | 管理、运营、分发 API 与机器可读契约 | 完成 | `docs/openapi.yaml` |

## 目标架构

```text
                    Cloudflare / WAF / VPN
                       │ public     │ admin
                       ▼            ▼
                    Caddy / Ingress
                           │
                   ┌──── API replicas ────┐
                   │         │            │
             Redis cache  PostgreSQL   S3/MinIO
                   │         │
                   └── durable queue ── Worker replicas
                                          │
                                    sing-box / xray

Prometheus ← metrics        OTLP Collector → Jaeger
```

SQLite、进程内缓存和 embedded worker 仍作为零服务开发/单机路径，业务执行逻辑与
分布式部署共用。

## 完成标准

- 订阅热路径使用原子预渲染缓存、ETag、Cache-Control、Redis 和对象存储快照。
- 节点、观测、Job、事件、Task、源、用户、Token、审计、配置版本和告警持久化。
- Worker 使用数据库租约、优先级、指数退避、取消和死信，多进程可安全竞争。
- 管理面与订阅面可按 host/CIDR 隔离；RBAC 和租户边界覆盖全部管理路径。
- 订阅凭证 bcrypt 存储，具备 ACL、RPS、日配额、流量、过期和即时吊销。
- 真实协议拨测记录 HTTP 延迟、字节和吞吐量；评分含 7 日稳定性并返回 breakdown。
- 智能池和国家分区提供 raw/Base64/Clash 独立 URL。
- request ID、job ID 和 W3C trace context 贯穿 API、队列与 Worker。
- 告警覆盖服务 SLO、供给异常、死信、磁盘、端点与证书，并支持应用生命周期/Webhook。
- CI 覆盖测试、集成、静态检查、依赖扫描、SBOM、镜像和 staging smoke。
- Compose/Helm/systemd 均具备探针、Secret 注入和可恢复发布路径。

## 运营验收目标

以下为上线后的 SLO，不是仅靠仓库代码即可证明的结果：

| 指标 | 目标 |
|---|---|
| 订阅月可用性 | ≥ 99.9% |
| 缓存命中 `/sub/base64` P95 | < 200 ms |
| 定时任务成功率 | ≥ 99% |
| Job/节点历史 | ≥ 30 天 |
| Token 泄漏吊销时间 | < 5 分钟 |
| 单版本回滚 RTO | < 15 分钟 |

Prometheus 规则和仪表盘数据已就绪；必须在 staging/prod 运行观测窗口后验收这些 SLO。

## 仓库外动作

以下动作需要资产所有者授权，不能由代码提交替代：

1. 轮换曾暴露的真实 Token、数据库和对象存储凭证。
2. 若密钥进入 Git 历史，协调所有协作者后执行历史重写和强制推送。
3. 创建 DNS、WAF/Access 策略、证书、外部数据库、Redis、S3 bucket 和备份异地副本。
4. 部署 staging，执行真实源/真实拨测 smoke、容量测试、故障注入和恢复演练。
5. 通过受保护环境审批生产发布，并依据真实流量调整 HPA、Worker 并发和告警阈值。
