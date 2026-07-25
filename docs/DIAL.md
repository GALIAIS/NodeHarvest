# 真实协议拨测（sing-box）

## 与 TCP 测速区别

| | quality（现有） | dial（本功能） |
|--|-----------------|----------------|
| 探测 | TCP/TLS 到节点端口 | 拉起 sing-box，按节点协议出站 |
| 结论 | 端口可达 | **代理链路真实可用** |
| 速度 | 快 | 慢（每节点数秒） |
| 标记 | `alive` / `score` | `verified` + `dial` |

## 依赖

```bash
# VPS
bash /opt/node-hunter/deploy/install-singbox.sh
# 或手动放到 /opt/node-hunter/bin/sing-box
export NODE_HUNTER_SINGBOX=/opt/node-hunter/bin/sing-box
systemctl restart node-hunter
```

## 配置 `configs/config.yaml`

```yaml
dial:
  enabled: true
  bin: "/opt/node-hunter/bin/sing-box"
  engine: sing-box
  concurrency: 4
  timeout_sec: 18
  test_url: "https://www.cloudflare.com/cdn-cgi/trace"
  max_nodes: 0              # 0=全部 HQ；>0 则限制单次数量
  batch_size: 200           # 多轮，每批 200
  after_quality: true       # full/quality 后自动真拨
  after_quality_max: 0      # 0=全部 HQ；>0 则只测 TopN
```

## API

```bash
# 状态（是否找到二进制、已验证数量）
curl https://node.galiais.com/api/dial/status

# 启动拨测（对 HQ 存活 Top N）
curl -X POST https://node.galiais.com/api/jobs/dial \
  -H 'Content-Type: application/json' \
  -d '{"max_dial":40,"force":false}'

# 仅看已验证节点
curl 'https://node.galiais.com/api/nodes?verified=1&limit=20'

# 订阅池
curl https://node.galiais.com/api/pools
# verified 池：通过真实拨测的节点
```

## 行为说明

1. 候选：优先 `alive + hq + 支持协议`（ss/vmess/vless/trojan/hy2）
2. 每个节点：写临时 sing-box 配置 → 本地 socks → HTTP GET `test_url`
3. 成功：`verified=true`，`tags` 含 `verified`，分数至少抬到 85
4. 失败：`tags` 含 `dial-fail`，保留 TCP 结果
5. 当 `verified >= 10` 时，**默认 `/sub` 优先发布 verified 池**

## 候选选取（2.1.2+）

默认 **all HQ**：存活且 score≥min_score 的节点，有多少测多少，按 `batch_size`（默认 200）多轮执行。

- `max_nodes: 0` / `after_quality_max: 0` → 全部 HQ
- 非 `force` 时跳过已 `verified`（已知可用）
- 全量模式不跳过 `dial-fail`（会重测）
- 若显式 `max_dial>0`：限量 + 国家/协议分散（压低 CDN 假活权重）

## 建议节奏

- 定时 full：TCP 粗筛 → 自动对全部 HQ 分批真拨
- 手动全量：`curl -X POST .../api/jobs/dial -d '{"all_hq":true,"force":false}'`
- 手动限量：`curl -X POST .../api/jobs/dial -d '{"max_dial":200,"force":false}'`

## 限制

- 当前引擎实现以 **sing-box** 为主
- 部分冷门传输（如复杂 xhttp）可能跳过 transport
- SSR/TUIC 暂不拨测
- 并发过高会占满 CPU/端口，建议 concurrency ≤ 8
