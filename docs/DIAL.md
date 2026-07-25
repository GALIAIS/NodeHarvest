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
  max_nodes: 80
  after_quality: false      # true=full/quality 后自动真测
  after_quality_max: 40
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

## 候选选取（2.1.1+）

纯按 TCP score 排序会优先测到 **CDN 假活**（延迟 2–5ms、TLS 通但代理不可用）。
`pickDialCandidates` 会：

- 从大池（约 max_dial×25）中选
- 压低超低 TCP 延迟节点权重
- 按 **国家 + 协议** 配额分散，同 server 限流
- 非 `force` 时跳过已 `dial-fail` / 已 `verified`，优先测新候选

## 建议节奏

- 定时 full：仍用 TCP 粗筛（快）
- `after_quality: true` 后自动对分散 TopN 精筛
- 手动：`curl -X POST .../api/jobs/dial -d '{"max_dial":80,"force":false}'`

## 限制

- 当前引擎实现以 **sing-box** 为主
- 部分冷门传输（如复杂 xhttp）可能跳过 transport
- SSR/TUIC 暂不拨测
- 并发过高会占满 CPU/端口，建议 concurrency ≤ 8
