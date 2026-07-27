# NodeHarvest 安全基线

## 身份与权限

- 控制台只使用本地账户密码，密码以 bcrypt 保存；不接受 Token、Bearer 或外部身份提供商登录。
- 会话为 HS256 签名 JWT，放入 HttpOnly、SameSite=Lax Cookie；生产必须使用 HTTPS。
- 角色为 `viewer`、`operator`、`admin`，所有任务、Token、用户与审计读取按租户隔离。
- 系统级源与热配置仅允许 `default` 租户 admin 修改。
- 未登录调用仅允许健康检查、登录会话和脱敏仪表盘；其余 API、导出与所有操作均要求会话 Cookie。

## 订阅凭证

- 数据库 Token 使用 bcrypt 存储，只通过 8 字符前缀定位候选，明文仅创建时返回。
- 支持启停、删除、过期、国家 ACL、协议 ACL、每 Token RPS、原子日配额和流量统计。
- master Token 通过 `NODE_HARVEST_TOKEN` 注入，不写入仓库。
- 默认只接受 `Authorization: Bearer` 或 `X-Sub-Token`。
- `security.allow_query_token` 默认关闭；URL Token 会泄漏至浏览历史、Referer、代理与 CDN 日志。

## 管理面隔离

- 使用 `auth.admin_host` / `auth.public_host` 分离域名。
- 使用 `auth.admin_cidrs` 将管理面限制在 VPN、办公网或 Cloudflare Access 出口。
- `deploy/Caddyfile.prod` 的公网域名只允许订阅和健康检查路径。
- `/metrics` 与可选 pprof 属于管理面，不应直接暴露公网。

## 信任边界

- 所有 JSON body 有大小上限、单值解析和字段校验。
- URL、CIDR、租户、角色、协议、权重、配额和运行时配置在入口验证。
- 源抓取限制重定向、响应大小、超时和私网/危险目标；日志会脱敏 URL 凭证。
- 真实拨测为每节点隔离的临时 sing-box/xray 进程，受上下文超时和强制清理约束。
- 对象存储 key 使用受限片段，不接受任意路径。

## 密钥注入

生产 Secret 至少包括：

```text
NODE_HARVEST_TOKEN
NODE_HARVEST_SESSION_SECRET
NODE_HARVEST_BOOTSTRAP_PASSWORD_HASH
NODE_HARVEST_ALERT_WEBHOOK_SECRET
NODE_HARVEST_OBJECT_STORE_ACCESS_KEY
NODE_HARVEST_OBJECT_STORE_SECRET_KEY
NODE_HARVEST_DATABASE_URL
NODE_HARVEST_REDIS_URL
```

不得把真实值写入 YAML、Compose、Helm values、日志或 CI artifact。Kubernetes 使用
`secrets.existingSecret` 引用外部 Secret；云环境优先使用 Secret Manager/External Secrets。

## Webhook

异常 Webhook body 为持久化告警 JSON。若配置 secret，请验证：

```text
X-NodeHarvest-Signature = "sha256=" + hex(HMAC-SHA256(raw_body, secret))
```

接收端应使用常量时间比较、防重放策略和 HTTPS。

## 浏览器与代理

- 生产强制 HTTPS/HSTS、`nosniff`、`no-referrer`，隐藏 Server header。
- 登录端点有独立 IP+租户+账户速率限制。
- CORS 只允许显式配置的管理端 origin。
- 可信代理列表必须与真实反向代理网段一致，否则不要信任转发来源 IP。

## 供应链

CI 执行：

- gofmt、Go race test、unit/integration test、vet；
- frontend lint/build 和生产依赖 `npm audit --omit=dev --audit-level=high`；
- govulncheck、gosec；
- Gitleaks 当前工作树扫描、Helm lint/template；
- SPDX JSON SBOM、容器镜像构建、固定 SHA 的 Grype 高危扫描和容器 smoke。

Dockerfile 的基础镜像固定多架构 digest；sing-box 1.13.12 固定到上游 commit，
使用当前 Go 安全补丁和已修复依赖从源码静态构建，只启用出站拨测所需标签。
独立安装脚本对官方 amd64/arm64 归档校验 SHA-256。ESLint/Next 的开发期工具链
由锁文件固定；生产依赖审计与最终运行镜像扫描均为阻断式检查。

TLS 证书验证默认开启。只有采集 URI 显式携带 `insecure=true`、
`allowInsecure=1` 或 `skip-cert-verify=true` 时，真实拨测和 Clash 导出才保留
兼容性跳过；这类节点应在源治理中优先淘汰。

## 数据保护

- PostgreSQL/SQLite 保存凭证哈希、操作审计和租户数据；限制数据库网络与账户权限。
- 备份默认使用 age 加密，生成 SHA-256，恢复前验证数据库和 archive。
- age 私钥与备份分离保存；定期恢复演练。
- 审计默认保留 90 天，Job/指标默认 30 天，可按法规调整。

## 发布前检查

- [ ] 已替换所有默认/示例密钥，session secret 至少 32 个随机字符。
- [ ] 管理域名受 VPN/Access/IP allowlist 保护。
- [ ] 公网匿名浏览器只能访问仪表盘；`/api/admin`、`/metrics`、`/debug` 和其余管理 API 必须返回 401/403。
- [ ] 数据库、Redis、对象存储未直接暴露公网。
- [ ] `allow_query_token=false`，除非客户端兼容性明确要求。
- [ ] CORS、trusted proxies、admin CIDRs 使用精确列表。
- [ ] 加密备份、异地复制和恢复演练已验证。
- [ ] govulncheck、gosec、npm audit、镜像扫描和 SBOM 通过。
- [ ] 泄漏过的历史密钥已在提供商侧轮换，而非只从当前文件删除。
