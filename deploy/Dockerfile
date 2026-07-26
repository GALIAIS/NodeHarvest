FROM --platform=$BUILDPLATFORM node:24-alpine@sha256:a0b9bf06e4e6193cf7a0f58816cc935ff8c2a908f81e6f1a95432d679c54fbfd AS frontend
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
ENV STATIC_EXPORT=1
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26.5-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS backend
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN for attempt in 1 2 3; do go mod download && exit 0; sleep 5; done; exit 1
COPY cmd ./cmd
COPY internal ./internal
ENV CGO_ENABLED=0
RUN LDFLAGS="-s -w -X github.com/GALIAIS/NodeHarvest/internal/version.Version=${VERSION} -X github.com/GALIAIS/NodeHarvest/internal/version.Commit=${COMMIT}" \
 && GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" go build -trimpath -ldflags="${LDFLAGS}" -o /out/nodeharvest-server ./cmd/server \
 && GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" go build -trimpath -ldflags="${LDFLAGS}" -o /out/nodeharvest-worker ./cmd/worker \
 && GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" go build -trimpath -ldflags="${LDFLAGS}" -o /out/nodeharvest-migrate ./cmd/migrate \
 && GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" go build -trimpath -ldflags="${LDFLAGS}" -o /out/nodeharvest ./cmd/nodeharvest

FROM --platform=$BUILDPLATFORM golang:1.26.5-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS singbox
ARG TARGETOS=linux
ARG TARGETARCH
RUN apk add --no-cache ca-certificates git
WORKDIR /src
RUN git init . \
 && git remote add origin https://github.com/SagerNet/sing-box.git \
 && git fetch --depth 1 origin 1086ab2563320e0da0c23b3a491d8dfa0939dff4 \
 && test "$(git rev-parse FETCH_HEAD)" = 1086ab2563320e0da0c23b3a491d8dfa0939dff4 \
 && git checkout --detach FETCH_HEAD
RUN go get \
      filippo.io/edwards25519@v1.1.1 \
      github.com/go-chi/chi/v5@v5.3.0 \
      github.com/go-jose/go-jose/v4@v4.1.4 \
      golang.org/x/crypto@v0.53.0 \
      golang.org/x/net@v0.56.0 \
      golang.org/x/sys@v0.47.0 \
      golang.org/x/text@v0.40.0 \
      google.golang.org/grpc@v1.82.1
ENV CGO_ENABLED=0
RUN GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" go build -trimpath \
      -tags "with_quic,with_wireguard,with_utls,badlinkname,tfogo_checklinkname0" \
      -ldflags "-X github.com/sagernet/sing-box/constant.Version=1.13.12 -X internal/godebug.defaultGODEBUG=multipathtcp=0 -checklinkname=0 -s -w -buildid=" \
      -o /out/sing-box ./cmd/sing-box

FROM alpine:3.23@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40
RUN apk add --no-cache ca-certificates tzdata curl \
 && adduser -D -H -u 1000 nodeharvest
WORKDIR /app
COPY --from=backend /out/nodeharvest-server /out/nodeharvest-worker /out/nodeharvest-migrate /out/nodeharvest /app/bin/
COPY --from=singbox /out/sing-box /app/bin/sing-box
COPY --from=frontend /web/out /app/web/dist
COPY configs /app/configs
RUN mkdir -p /app/data /app/output /app/logs \
 && chmod 0700 /app/data /app/output /app/logs \
 && chown -R nodeharvest:nodeharvest /app
USER nodeharvest
ENV TZ=Asia/Shanghai
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8080/api/health || exit 1
ENTRYPOINT ["/app/bin/nodeharvest-server"]
CMD ["-addr", ":8080", "-config", "configs/config.yaml", "-data", "data", "-web", "web/dist"]
