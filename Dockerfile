# ---------- 前端构建 ----------
FROM node:22-alpine AS web-build
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---------- 后端构建 ----------
FROM golang:1.26-alpine AS server-build
WORKDIR /app/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/api ./cmd/api

# ---------- 运行镜像 ----------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 bi && \
    mkdir -p /app/server/data /app/web
WORKDIR /app/server
COPY --from=server-build /out/api /app/server/api
COPY --from=web-build /app/web/dist /app/web/dist
ENV PORT=8486 \
    WEB_DIST_DIR=/app/web/dist \
    APP_TIMEZONE=Asia/Shanghai \
    DB_DRIVER=sqlite \
    DB_DSN=data/bi.db
RUN chown -R bi:bi /app
USER bi
EXPOSE 8486
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s \
    CMD wget -qO- http://127.0.0.1:8486/health || exit 1
ENTRYPOINT ["/app/server/api"]
