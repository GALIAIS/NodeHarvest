# node-hunter API 镜像（linux/amd64 & arm64）
# 前端控制台可选：本机 npm run dev 或单独反代
FROM golang:1.23-alpine AS backend
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/node-hunter-server ./cmd/server \
 && go build -trimpath -ldflags="-s -w" -o /out/node-hunter ./cmd/node-hunter

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata curl \
 && adduser -D -H -u 1000 hunter
WORKDIR /app
COPY --from=backend /out/node-hunter-server /out/node-hunter /app/bin/
COPY configs /app/configs
RUN mkdir -p /app/data /app/output /app/logs \
 && chown -R hunter:hunter /app
USER hunter
ENV TZ=Asia/Shanghai
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8080/api/health || exit 1
ENTRYPOINT ["/app/bin/node-hunter-server"]
CMD ["-addr", ":8080", "-config", "configs/config.yaml", "-data", "data"]
