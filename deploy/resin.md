# Resin 代理池（与 node-hunter verified 对接）

## 架构

```
node-hunter (定时 full → verified 订阅)
        │
        │  https://node.galiais.com/sub/raw?token=...
        ▼
Resin (Docker) :12260
  · HTTP 正向代理
  · SOCKS5 正向代理
  · 粘性会话 / 健康检查 / Web UI
```

## 部署位置（HK VPS）

| 项 | 路径/值 |
|---|---|
| 源码 / compose | `/root/Resin` |
| 环境变量 | `/root/Resin/.env` |
| 集成信息（含 token） | `/opt/resin/INTEGRATION.env` |
| 使用说明 | `/opt/resin/USAGE.txt` |
| 监听端口 | **12260** |
| 容器 | `docker ps -f name=resin` |

订阅：`node-hunter-verified`，每 **10 分钟** 刷新一次 node-hunter 的 raw 订阅。

## 常用命令

```bash
cd /root/Resin
docker compose ps
docker compose logs -f --tail=100
docker compose restart

# 读取凭据
set -a; source /opt/resin/INTEGRATION.env; set +a

# 健康
curl -sS http://127.0.0.1:$RESIN_PORT/healthz

# HTTP 正向代理
curl -x http://127.0.0.1:$RESIN_PORT -U ":$RESIN_PROXY_TOKEN" https://api.ipify.org

# SOCKS5
curl --proxy socks5h://127.0.0.1:$RESIN_PORT -U "Default:$RESIN_PROXY_TOKEN" https://api.ipify.org

# 粘性（同一账号尽量同一出口）
curl -x http://127.0.0.1:$RESIN_PORT -U "Default.user_tom:$RESIN_PROXY_TOKEN" https://api.ipify.org
```

## 管理后台

浏览器打开：`http://<VPS_IP>:12260`  
登录密码：`RESIN_ADMIN_TOKEN`（见 `/opt/resin/INTEGRATION.env`）。

订阅管理 API（需 Admin Bearer）：

```bash
curl -sS -H "Authorization: Bearer $RESIN_ADMIN_TOKEN" \
  http://127.0.0.1:$RESIN_PORT/api/v1/subscriptions
```

## 安全建议

1. 不要把 `INTEGRATION.env` / `.env` 提交到 Git。
2. 防火墙尽量只放行你自己的 IP 访问 `12260`（或走 Cloudflare Tunnel / VPN）。
3. 公网暴露时务必使用强 `RESIN_PROXY_TOKEN`。
4. node-hunter 订阅 token 已写入 Resin 订阅 URL，轮换 hunter token 后需更新订阅。

## 与 node-hunter 的关系

- hunter 负责：采集、测速、真拨、产出 verified 列表。
- Resin 负责：把 verified 节点变成**统一代理入口**，做熔断与粘性。
- 不改变 hunter 定时逻辑；Resin 只消费 `/sub`。
