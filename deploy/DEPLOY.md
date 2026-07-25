# node-hunter VPS 部署指南

目标：在 VPS 上定时采集 → 测速筛选高质量节点 → 对外提供订阅链接，供其他用户在 v2rayN / Clash / sing-box 中远程拉取。

## 架构

```
定时 schedule (默认 3h)
    → fetch 多源订阅
    → quality 测速 + prune 清理死亡节点
    → export 高质量列表
    → 发布 /sub/raw | /sub/base64 | /sub/clash
用户客户端 ──HTTP──▶ VPS:8080/sub/...
```

## 快速：Docker Compose

```bash
cd node-hunter
export NODE_HUNTER_TOKEN="$(openssl rand -hex 16)"
export NODE_HUNTER_PUBLIC_URL="https://sub.example.com"
docker compose -f deploy/docker-compose.yml up -d --build
```

验证：

```bash
curl -sS http://127.0.0.1:8080/api/health
curl -sS "http://127.0.0.1:8080/sub?token=$NODE_HUNTER_TOKEN"
curl -sS "http://127.0.0.1:8080/sub/base64?token=$NODE_HUNTER_TOKEN" | head -c 80
```

## 二进制 + systemd

```bash
# 构建
CGO_ENABLED=0 go build -o bin/node-hunter-server ./cmd/server

# 安装
sudo useradd -r -s /usr/sbin/nologin nodehunter || true
sudo mkdir -p /opt/node-hunter
sudo cp -a bin configs /opt/node-hunter/
sudo mkdir -p /opt/node-hunter/data /opt/node-hunter/output
sudo chown -R nodehunter:nodehunter /opt/node-hunter

# 改 token / 域名
sudo cp deploy/node-hunter.service /etc/systemd/system/
sudo sed -i 's/change-me-please/你的随机token/' /etc/systemd/system/node-hunter.service
sudo sed -i 's|https://sub.example.com|https://你的域名|' /etc/systemd/system/node-hunter.service

sudo systemctl daemon-reload
sudo systemctl enable --now node-hunter
sudo systemctl status node-hunter
```

Nginx 反代示例：

```nginx
server {
  listen 443 ssl http2;
  server_name sub.example.com;
  # ssl_certificate ...;

  location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Host $host;
  }
}
```

## 配置要点（configs/config.yaml）

```yaml
schedule:
  enabled: true
  interval_min: 180
  job: full
  skip_ai: true
  max_test: 1200
  rounds: 2

filter:
  min_score: 70
  max_nodes: 500
  prune_after_quality: true
  sort_by: score

publish:
  enabled: true
  token: ""          # 或用环境变量 NODE_HUNTER_TOKEN
  path_prefix: "/sub"
  min_score: 70
  max_nodes: 500
  public_url: "https://sub.example.com"
```

环境变量优先级高于 yaml：

| 变量 | 作用 |
|------|------|
| `NODE_HUNTER_TOKEN` | 订阅令牌 |
| `NODE_HUNTER_PUBLIC_URL` | 对外 URL 前缀 |
| `NODE_HUNTER_SCHEDULE` | `1`/`0` 强制开/关定时 |
| `NODE_HUNTER_SOCKS5` | AI 真实代理探测 |

## 用户侧订阅地址

假设域名 `https://sub.example.com`，token 为 `TOKEN`：

| 客户端 | 地址 |
|--------|------|
| v2rayN / v2rayNG / shadowrocket | `https://sub.example.com/sub/base64?token=TOKEN` |
| Clash / Mihomo / OpenClash | `https://sub.example.com/sub/clash?token=TOKEN` |
| 原始 URI 列表 | `https://sub.example.com/sub/raw?token=TOKEN` |
| 索引 JSON | `https://sub.example.com/sub?token=TOKEN` |

也支持 Header：`Authorization: Bearer TOKEN` 或 `X-Sub-Token: TOKEN`。

任务完成后磁盘还有固定文件（可直接用 Nginx 静态托管 `output/`）：

- `output/sub.txt` / `output/sub.base64` / `output/clash.yaml`
- `output/nodes-latest.*`

## 运维建议

1. **务必设置 token**，避免订阅被扫。
2. 带宽紧时关掉超大源：`ndsphonemy-all`、`leon406-vless`。
3. 测速并发 `app.concurrency` 在小 VPS 上建议 32–64。
4. 查看定时：`curl -s localhost:8080/api/schedule`。
5. 手动触发：`curl -X POST localhost:8080/api/jobs/full -d '{"skip_ai":true,"max_test":800}'`。
6. 仅供学习研究；请遵守当地法律与目标站点条款。
