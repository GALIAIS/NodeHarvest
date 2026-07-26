# NodeHarvest 容器镜像

NodeHarvest 使用 GitHub Actions 构建并发布
`ghcr.io/galiais/nodeharvest`。工作流沿用 S2AX 的发布方式：主分支维护滚动
镜像，版本标签生成不可变镜像和 GitHub Release，手动运行时允许指定源码
引用及额外标签。

## 自动发布

| 触发条件 | 发布结果 |
|---|---|
| 推送到 `main` | `ghcr.io/galiais/nodeharvest:edge` |
| 推送 `v*` Git 标签 | 与 Git 标签同名的镜像，并创建 GitHub Release |
| 手动运行 | 从 `source_ref` 构建，可通过 `image_tag` 发布额外标签 |

`edge` 是可变标签。主分支产生新镜像后，工作流会清理已经失去标签的旧 GHCR
版本，避免包列表持续积累无用条目。

## 手动发布

在 GitHub 的 Actions 页面选择 `Docker Image`，或者使用 GitHub CLI：

```bash
gh workflow run docker-image.yml \
  --repo GALIAIS/NodeHarvest \
  --ref main \
  -f source_ref=main \
  -f image_tag=staging
```

## 拉取和运行

```bash
docker pull ghcr.io/galiais/nodeharvest:edge

docker run --name nodeharvest --rm -p 8080:8080 \
  -e NODE_HARVEST_TOKEN="replace-with-subscription-secret" \
  -e NODE_HARVEST_ADMIN_TOKEN="replace-with-admin-secret" \
  -e NODE_HARVEST_SESSION_SECRET="at-least-32-random-characters" \
  ghcr.io/galiais/nodeharvest:edge
```

生产环境应固定版本标签或镜像 digest，不应直接依赖 `edge`。

## 构建内容

工作流使用 Buildx 构建 `linux/amd64` 镜像，并缓存构建层。镜像包含：

- 静态导出的 Next.js 管理控制台；
- NodeHarvest server、worker、migrate 和 CLI；
- 固定源码提交构建的 sing-box；
- 非 root 运行用户及健康检查。

构建时从根目录 `VERSION` 注入应用版本，并把 Git 提交 SHA 注入运行时版本
信息。为保持与 S2AX 相同的单平台包结构，工作流关闭 provenance 和 SBOM
附加清单；CI 仍会单独生成源码 SBOM 并执行镜像漏洞扫描。

## 本地复现

```bash
docker build \
  --file deploy/Dockerfile \
  --build-arg VERSION="$(tr -d '\r\n' < VERSION)" \
  --build-arg COMMIT="$(git rev-parse HEAD)" \
  --tag nodeharvest:local \
  .
```

构建完成后运行：

```bash
docker run -d --name nodeharvest-smoke -p 127.0.0.1:18080:8080 \
  -e NODE_HARVEST_TOKEN="local-subscription-token" \
  -e NODE_HARVEST_ADMIN_TOKEN="local-admin-token" \
  -e NODE_HARVEST_SESSION_SECRET="local-session-secret-at-least-32-bytes" \
  -e NODE_HARVEST_SCHEDULE=0 \
  nodeharvest:local

BASE_URL=http://127.0.0.1:18080 \
SUB_TOKEN=local-subscription-token \
bash deploy/smoke.sh
```
