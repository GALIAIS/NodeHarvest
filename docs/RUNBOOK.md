# node-hunter 运维手册（企业版）

## 服务

```bash
systemctl status node-hunter
journalctl -u node-hunter -f
systemctl restart node-hunter
```

路径：`/opt/node-hunter`  
环境：`/opt/node-hunter/.env`

## 关键探针

| 路径 | 用途 |
|------|------|
| `/api/health` | 存活 + 版本 + geo/sqlite/publish |
| `/api/ready` | 就绪（编排用） |
| `/api/version` | 构建信息 |
| `/metrics` | Prometheus |
| `/sub/*` | 订阅 |

## 订阅

```
https://node.galiais.com/sub/base64?token=TOKEN
Authorization: Bearer TOKEN
```

管理 Token（创建/吊销）：

```bash
# 列表
curl -H "Authorization: Bearer $ADMIN" https://node.galiais.com/api/admin/tokens
# 创建
curl -X POST -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"name":"friend","days":30,"countries":["US","JP"]}' \
  https://node.galiais.com/api/admin/tokens
# 刷新预渲染
curl -X POST -H "Authorization: Bearer $ADMIN" \
  https://node.galiais.com/api/admin/publish/refresh
```

`ADMIN` 默认与 `NODE_HUNTER_TOKEN` 相同，或单独设 `NODE_HUNTER_ADMIN_TOKEN`。

## 手动任务

```bash
curl -X POST http://127.0.0.1:8080/api/jobs/full \
  -H 'Content-Type: application/json' \
  -d '{"skip_ai":true,"max_test":1200,"rounds":2}'
curl -X POST http://127.0.0.1:8080/api/jobs/cancel \
  -H "Authorization: Bearer $ADMIN"
```

## 备份

```bash
APP_DIR=/opt/node-hunter bash /opt/node-hunter/deploy/backup.sh
# 或 cron:
# 15 4 * * * APP_DIR=/opt/node-hunter /opt/node-hunter/deploy/backup.sh
```

## 回滚

```bash
cp /opt/node-hunter/bin/node-hunter-server.bak /opt/node-hunter/bin/node-hunter-server
systemctl restart node-hunter
```

## 告警建议

- `nh_nodes_hq` < 50
- `nh_job_runs_total{status="failed"}` 增长率
- `/api/health` 非 200
- 磁盘 > 85%
