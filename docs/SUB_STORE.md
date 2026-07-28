# Sub-Store 集成指南

NodeHarvest 通过官方 Sub-Store 容器提供完整的订阅转换、组合、过滤、脚本处理、
文件托管、同步和多客户端输出能力。上游程序不做裁剪或重写；NodeHarvest 负责账号
会话、节点 Token、入口导航和生产部署。

## 部署拓扑

Sub-Store 必须使用独立子域。其 Vue history 路由、PWA manifest 和 Service Worker
都以 `/` 为作用域，挂到 NodeHarvest 子路径会与现有路由和缓存作用域冲突。

```text
admin.example.com  ── 账号密码登录 ── NodeHarvest
store.example.com  ── Caddy forward_auth ── Sub-Store :3000
                                          └── /opt/app/data
```

生产 Compose 使用 `xream/sub-store:2.36.22`，并固定多架构镜像摘要
`sha256:a031ebef93d0ce6cc0a2a5043ce997fdcfca1b4cb8bf4f1c0e547eb1d8ba70df`。

## 首次启用

为公开订阅、管理控制台和 Sub-Store 创建 DNS 记录，然后设置：

```bash
export PUBLIC_DOMAIN="node.example.com"
export ADMIN_DOMAIN="admin.example.com"
export SUB_STORE_DOMAIN="store.example.com"
export SESSION_COOKIE_DOMAIN="example.com"
export SUB_STORE_BACKEND_PATH="/replace-with-a-random-path"
```

`SESSION_COOKIE_DOMAIN` 必须是管理域和 Sub-Store 域的共同受控父域，不能使用不属于
自己的公共后缀。`SUB_STORE_BACKEND_PATH` 只能是一段路径；随机路径用于降低扫描噪声，
真正的访问控制仍由签名会话和节点 Token 完成。

验证并启动：

```bash
docker compose -f deploy/compose.prod.yml config
docker compose -f deploy/compose.prod.yml up -d
docker compose -f deploy/compose.prod.yml ps
```

登录 NodeHarvest 后打开侧边栏“订阅工坊”。嵌入页和“全屏打开”指向同一个隔离源站，
浏览器不会把签名会话暴露给 JavaScript。

## 使用 NodeHarvest 节点

在 Sub-Store 中创建单条订阅时，可使用：

```text
https://node.example.com/sub/raw?token=YOUR_NODE_TOKEN
https://node.example.com/sub/base64?token=YOUR_NODE_TOKEN
https://node.example.com/sub/clash?token=YOUR_NODE_TOKEN
```

URL Token 需要显式设置 `NODE_HARVEST_ALLOW_QUERY_TOKEN=1`。保持默认关闭时，在
Sub-Store 的请求设置中使用 `Authorization: Bearer YOUR_NODE_TOKEN` 或
`X-Sub-Token: YOUR_NODE_TOKEN`，避免凭据进入 URL。

Sub-Store 可将这些来源组合成合集，再使用过滤器、正则、脚本操作符和目标格式生成
Stash、mihomo、Surfboard、Surge、Loon、Egern、Shadowrocket、Quantumult X、
sing-box、V2Ray URI 或 JSON 等输出。

## 给订阅客户端使用输出

浏览器内预览 `/share/*` 和 `/download/*` 使用 operator/admin 会话，不消耗节点
Token 配额。把输出复制给外部客户端时，追加独立参数 `nh_token`：

```text
https://store.example.com/share/sub/my-sub?target=ClashMeta&nh_token=YOUR_NODE_TOKEN
https://store.example.com/download/my-file?target=JSON&nh_token=YOUR_NODE_TOKEN
```

已有查询参数时使用 `&nh_token=`。Caddy 先让 NodeHarvest 校验 Token、ACL、RPS、
日配额和吊销状态，随后删除 `nh_token`、`Authorization`、`X-Sub-Token` 与会话 Cookie，
再把请求交给 Sub-Store。Sub-Store 自己的 share token 可继续使用，两层限制互不冲突。

如果未来为 Caddy 开启访问日志，应对 `nh_token` 做日志脱敏；URL Token 仍可能进入
客户端历史，适合无法发送请求头的订阅客户端，并应定期轮换。

## 安全边界

- 管理、API、同步、脚本和普通页面全部要求 default 租户 operator/admin 的会话。
- `/share/*`、`/download/*` 接受 operator/admin 会话或有效节点 Token。
- viewer 不能进入 Sub-Store；未登录用户仍只能查看 NodeHarvest 仪表盘。
- 带国家/协议 ACL 或非 default 租户的 Token 不可读取共享 Sub-Store 输出；应创建专用、
  无节点范围 ACL 的 default 租户 Token，并继续用 RPS、日配额、到期和吊销限制。
- Sub-Store 容器不发布宿主机端口，只允许 Caddy 在内部网络访问。
- 容器使用只读根文件系统、无 Linux capabilities；只有数据卷与 `/tmp` 可写。
- NodeHarvest 的 HttpOnly 会话 Cookie 在转发前删除，Sub-Store 无法读取。
- CORS 仅允许配置的 Sub-Store 与管理源站；`frame-ancestors` 仅允许管理域嵌入。
- WebSocket 与流式响应由 Caddy `reverse_proxy` 原样转发，不启用响应缓冲。

## 数据、备份与升级

Sub-Store 状态保存在 Compose 卷 `sub-store-data` 的 `/opt/app/data`。备份生产栈时要
同时备份该卷；删除或重建容器不会删除卷，执行 `docker compose down -v` 会删除。

升级时先核对 [Sub-Store Releases](https://github.com/sub-store-org/Sub-Store/releases)
和 `xream/sub-store` 镜像摘要，再同时更新：

1. `deploy/compose.prod.yml` 的标签与摘要；
2. `internal/config/config.go` 的展示版本；
3. `configs/config.yaml` 与本文件的版本记录。

更新后运行 Compose 配置校验、健康检查和 NodeHarvest 测试，再滚动重启。

## 故障定位

| 现象 | 检查 |
|---|---|
| Sub-Store 页面 401 | 重新登录；确认 Cookie 父域覆盖管理域与 Sub-Store 域 |
| 页面 403 | 当前账号至少需要 operator 角色 |
| 外部分享链接 401 | 添加 `nh_token` 或订阅请求头；确认 Token 未禁用/过期 |
| URL Token 仍 401 | 设置 `NODE_HARVEST_ALLOW_QUERY_TOKEN=1` 并重启 API |
| iframe 拒绝加载 | 确认通过 Caddy 域名访问，且 `ADMIN_DOMAIN` 与浏览器源站一致 |
| 容器 unhealthy | 检查后端路径一致性及 `sub-store-data` 写权限 |

上游功能与参数以 [Sub-Store 官方仓库](https://github.com/sub-store-org/Sub-Store)
和 [Sub-Store Front-End](https://github.com/sub-store-org/Sub-Store-Front-End) 为准。
