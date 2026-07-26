# NodeHarvest Web

NodeHarvest 管理控制台，基于 Next.js 16、React 19 和 TypeScript。

## 开发

先在仓库根目录启动 Go API，再启动前端：

```bash
npm ci
npm run dev
```

默认访问 `http://127.0.0.1:3000`。通过 `API_ORIGIN` 指向不同的 API 地址。

## 验证与静态构建

```bash
npm run lint
STATIC_EXPORT=1 npm run build
```

生产镜像会把静态输出复制到 Go 服务，由同一端口提供控制台与 API。
