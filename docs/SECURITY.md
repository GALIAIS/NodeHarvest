# Security

## Tokens

- **Master** `NODE_HUNTER_TOKEN` / `publish.token`：全量订阅
- **Admin** `NODE_HUNTER_ADMIN_TOKEN`：管理 API；未设则回退 master
- **DB tokens**：`/api/admin/tokens` 创建，哈希存储（SHA-256），支持国家 ACL、过期、启停

## Logging

- Access log 对 `token` query 脱敏为 `xxxx***`
- 禁止把完整 token 打进 journal / CF log 自定义字段

## Network

- 生产经 Cloudflare HTTPS
- 管理接口建议额外 CF Access / IP allowlist
- 限流：`security.sub_rps` / `api_rps`

## Headers

订阅支持：

- `Authorization: Bearer <token>`
- `X-Sub-Token: <token>`
- 可选 `?token=`（`security.allow_query_token`）

## Checklist

- [ ] Token 轮换流程
- [ ] `.env` 权限 600
- [ ] 备份不含明文传播
- [ ] govulncheck / 依赖更新
