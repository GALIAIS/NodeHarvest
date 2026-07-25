# node-hunter

代理节点 **采集 → 智能测速 → AI 可达筛选 → 导出** 一体化工具。

- **后端**：Go API（异步任务、多轮质量评分、AI 站点探测）
- **前端**：Next.js + Tailwind CSS + shadcn 风格组件

> 仅供学习与网络研究。请遵守当地法律法规与目标站点服务条款。

## 功能

- 多源订阅拉取（URI / Base64 / Clash YAML）
- 协议：vmess / vless / trojan / ss / ssr / hysteria2 / tuic
- 智能质量：多轮 TCP、抖动、成功率、TLS 握手、S/A/B/C/D/F 定级
- AI 站点探测：ChatGPT / Gemini / Claude / Grok / OpenAI / Copilot / Perplexity
  - 启发模式（默认）
  - 真实代理模式（配置本地 SOCKS5）
- Web 控制台：仪表盘 / 节点库 / 任务 / 源 / AI 矩阵 / 导出
- 导出：raw / base64 / clash / json + AI 友好列表
- **定时任务**：可配置间隔自动采集 + 测速 + 清理低质量节点
- **公开订阅**：`/sub/raw|base64|clash` 供远程客户端拉取（可 Token）
- **VPS 部署**：Docker Compose / systemd，见 [deploy/DEPLOY.md](deploy/DEPLOY.md)

## 架构

```
node-hunter/
  cmd/node-hunter/   # CLI
  cmd/server/        # HTTP API :8080 + /sub 订阅 + scheduler
  web/               # Next.js 控制台 :3000
  configs/           # 订阅源与阈值 / schedule / publish
  internal/          # 业务逻辑（含 scheduler）
  data/              # 节点快照 snapshot.json
  output/            # 导出文件（含 sub.txt / clash.yaml）
  deploy/            # Docker / systemd / Caddy
  bin/               # Linux/macOS 构建产物（start.sh 自动生成）
  start.sh / stop.sh # Linux / Alpine / Minis 一键启停
  start.ps1          # Windows 一键启动
```

## 环境要求

| 组件 | 版本建议 |
|------|----------|
| Go | **1.22+**（Alpine 3.21 自带 1.23.9 可用；`go.mod` 已固定 `go 1.23.0`） |
| Node.js | 20+ / 22 LTS |
| npm | 10+ |
| OS | Linux aarch64/x86_64、macOS、Windows |

Minis / Alpine 安装示例：

```bash
apk add go nodejs npm git curl
```

> 注意：Android 挂载的 `/storage/emulated/0/...`（`/var/minis/mounts/...`）通常 **不支持 flock**，
> `go mod` / 部分构建会失败。请把项目放到可写可 flock 的路径，例如：
> `/var/minis/workspace/node-hunter`（本仓库已按此路径验证）。

## 快速开始（Linux / Alpine / Minis）

### 一键启动

```bash
cd /var/minis/workspace/node-hunter   # 或你的项目路径
chmod +x start.sh stop.sh run-server.sh
./start.sh
```

- 后端：`http://127.0.0.1:8080`
- 前端：`http://127.0.0.1:3000`
- 停止：`./stop.sh`

仅 API：

```bash
./run-server.sh
# 或
./bin/node-hunter-server -addr 127.0.0.1:8080 -config configs/config.yaml
```

### 手动构建

```bash
export CGO_ENABLED=0
export GOPROXY=https://proxy.golang.org,direct
go mod tidy
go build -o bin/node-hunter-server ./cmd/server
go build -o bin/node-hunter ./cmd/node-hunter

cd web
npm install --no-fund --no-audit
# 若从 Windows 拷来 node_modules，请先删掉再装：
# rm -rf node_modules && npm install
npm run dev -- -H 127.0.0.1 -p 3000
```

浏览器打开：http://127.0.0.1:3000  
前端通过 rewrite 把 `/api/*` 代理到 `http://127.0.0.1:8080`（可用 `API_ORIGIN` 覆盖）。

### 推荐操作流

1. 仪表盘点 **一键全流程**
2. 等待任务完成（任务中心看进度）
3. 节点库筛选「仅高质量」
4. 导出页下载订阅，或取 `output/nodes-latest.*`

### CLI（可选）

```bash
./bin/node-hunter -skip-test                 # 只采集
./bin/node-hunter -c 128 -max-nodes 200
```

## Windows

```powershell
.\start.ps1
# 或
go build -o node-hunter-server.exe ./cmd/server
.\node-hunter-server.exe -addr :8080 -config configs\config.yaml
cd web; npm install; npm run dev
```

## AI 真实代理探测（可选）

1. 用 xray / sing-box 把候选节点挂到本地 SOCKS5（如 `127.0.0.1:1080`）
2. 启动后端前设置：

```bash
export NODE_HUNTER_SOCKS5=127.0.0.1:1080
./bin/node-hunter-server -addr :8080
```

PowerShell：

```powershell
$env:NODE_HUNTER_SOCKS5 = "127.0.0.1:1080"
.\node-hunter-server.exe
```

或在 `POST /api/jobs/ai` body：`{"socks5":"127.0.0.1:1080"}`

无 SOCKS5 时为启发模式，用于筛候选，不保证经节点可访问 AI。

## 定时任务 + 高质量筛选

`configs/config.yaml`：

```yaml
schedule:
  enabled: true
  interval_min: 180   # 每 3 小时
  job: full           # full | fetch | quality
  skip_ai: true
  max_test: 1200
  rounds: 2

filter:
  min_score: 70
  max_nodes: 500
  prune_after_quality: true
  sort_by: score
```

测速后会按 `prune_after_quality` 清理死亡节点，再按分数/延迟导出高质量池。

## 远程订阅（分发给其他用户）

```yaml
publish:
  enabled: true
  token: "your-secret"     # 或环境变量 NODE_HUNTER_TOKEN
  path_prefix: "/sub"
  min_score: 70
  max_nodes: 500
  public_url: "https://sub.example.com"
```

| 客户端 | URL |
|--------|-----|
| v2rayN / v2rayNG | `https://你的域名/sub/base64?token=TOKEN` |
| Clash Meta | `https://你的域名/sub/clash?token=TOKEN` |
| 原始 URI | `https://你的域名/sub/raw?token=TOKEN` |

也支持 `Authorization: Bearer TOKEN`。任务完成后磁盘固定文件：`output/sub.txt`、`output/sub.base64`、`output/clash.yaml`。

## VPS 一键部署

详见 **[deploy/DEPLOY.md](deploy/DEPLOY.md)**。

```bash
export NODE_HUNTER_TOKEN="$(openssl rand -hex 16)"
export NODE_HUNTER_PUBLIC_URL="https://sub.example.com"
docker compose -f deploy/docker-compose.yml up -d --build
```

## 主要 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/stats` | 仪表盘统计 |
| GET | `/api/nodes` | 节点列表（q/protocol/grade/hq/alive/ai/min_score） |
| POST | `/api/jobs/full` | 采集+测速+AI（`skip_ai` 可跳过 AI） |
| POST | `/api/jobs/fetch` | 仅采集 |
| POST | `/api/jobs/quality` | 智能测速 |
| POST | `/api/jobs/ai` | AI 探测 |
| GET | `/api/export/raw` | 导出 URI |
| GET | `/api/export/base64` | 导出 base64 订阅 |
| GET | `/api/export/clash` | 导出 Clash YAML |
| GET | `/api/schedule` | 定时任务状态 |
| GET | `/sub` | 订阅索引（links） |
| GET | `/sub/raw` | 公开 URI 订阅 |
| GET | `/sub/base64` | 公开 base64 订阅 |
| GET | `/sub/clash` | 公开 Clash 订阅 |

## 环境变量

| 变量 | 说明 |
|------|------|
| `API_ORIGIN` | Next.js rewrite 目标，默认 `http://127.0.0.1:8080` |
| `API_ADDR` | `start.sh` 后端监听，默认 `127.0.0.1:8080` |
| `WEB_HOST` / `WEB_PORT` | 前端监听，默认 `127.0.0.1` / `3000` |
| `NODE_HUNTER_SOCKS5` | AI 真实探测 SOCKS5 地址 |
| `NODE_HUNTER_TOKEN` | 公开订阅 Token |
| `NODE_HUNTER_PUBLIC_URL` | 对外 URL 前缀（写入 /sub 索引） |
| `NODE_HUNTER_SCHEDULE` | `1`/`0` 强制开/关定时 |
| `CGO_ENABLED` | 默认 `0`（静态链接，适合 Alpine musl） |
| `GOPROXY` | 默认 `https://proxy.golang.org,direct` |

## Minis 注意事项

1. **工作目录**：优先用 `/var/minis/workspace/node-hunter`，不要在 mount 盘直接 `go mod tidy`。
2. **前端 native**：Windows 拷来的 `web/node_modules` 只有 `swc-win32-*`，在 aarch64 Alpine 上必须重装以拿到 `swc-linux-arm64-musl`。
3. **并发测速**：手机/沙箱网络并发过高可能触发限流，可在 `configs/config.yaml` 把 `app.concurrency` 调到 `16~32`。
4. **后台进程**：`start.sh` 后端用 `nohup` + 日志重定向；App 挂起后长任务可能被系统回收，重要任务请保持前台会话。

## License

MIT
