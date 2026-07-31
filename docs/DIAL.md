# 真实协议拨测（sing-box / xray / Mihomo）

## 与 TCP 测速区别

| | quality（现有） | dial（本功能） |
|--|-----------------|----------------|
| 探测 | TCP/TLS 到节点端口 | 拉起代理核心，按完整节点配置出站 |
| 结论 | 端口可达 | **代理链路真实可用** |
| 速度 | 快 | 慢（每节点数秒） |
| 标记 | `alive` / `score` | `verified` + `dial` |

## 依赖

生产镜像已内置并固定 sing-box 1.13.12 与 Mihomo 1.19.29，Mihomo 官方归档在
构建时校验 SHA-256。非容器部署可设置 `NODE_HARVEST_SINGBOX` 和
`NODE_HARVEST_MIHOMO` 指向自行校验的二进制。

## 配置 `configs/config.yaml`

```yaml
dial:
  enabled: true
  bin: "/opt/nodeharvest/bin/sing-box"
  mihomo_bin: "/opt/nodeharvest/bin/mihomo"
  engine: both
  concurrency: 4
  timeout_sec: 18
  test_url: "https://www.cloudflare.com/cdn-cgi/trace"
  verified_ttl_hours: 6
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

1. 候选：`alive + hq + Mihomo 支持协议`，包括 SS/SSR/VMess/VLESS/Trojan/Hysteria2/TUIC。
2. `both` 依次运行 sing-box 和 Mihomo；每次都通过独立临时目录、本地 SOCKS 和 `test_url` 真测。
3. Mihomo 直接复用订阅导出的同一份代理对象，其结果决定 `verified`；sing-box 结果保存在 `dial.checks` 供对照。
4. 默认订阅和 verified 池只发布 `verified_ttl_hours` 内由 Mihomo 通过的节点，不再用 TCP 测活结果兜底。
5. 失败写入 `dial-fail`；后续成功会移除失败标记，反之亦然。

## 候选选取（2.1.2+）

默认 **all HQ**：存活且 score≥min_score 的节点，有多少测多少，按 `batch_size`（默认 200）多轮执行。

- `max_nodes: 0` / `after_quality_max: 0` → 全部 HQ
- 非 `force` 时跳过 `verified_ttl_hours` 内已有拨测结果的节点
- 成功与失败结果过期后都会重测
- 若显式 `max_dial>0`：限量 + 国家/协议分散（压低 CDN 假活权重）

## 建议节奏

- 定时 full：TCP 粗筛 → 自动对全部 HQ 分批真拨
- 手动全量：`curl -X POST .../api/jobs/dial -d '{"all_hq":true,"force":false}'`
- 手动限量：`curl -X POST .../api/jobs/dial -d '{"max_dial":200,"force":false}'`

## 限制

- Mihomo 是 Clash 订阅的权威结果；sing-box 不支持的 SSR、外部 SS 插件或部分 xhttp 不会否决 Mihomo 成功。
- 节点是否能从中国大陆访问仍受入口 IP、运营商路由、封锁和地域 ACL 影响；VPS 测试不能替代大陆网络测试。
- 并发过高会占满 CPU/端口，建议 concurrency ≤ 8。
