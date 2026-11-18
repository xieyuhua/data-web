#!/usr/bin/env bash
# 生产构建：先构建前端到 internal/webui/dist，再编译内嵌 UI 的单二进制。
set -e
cd "$(dirname "$0")/.."

echo "==> 构建前端"
cd web
if [ -f package-lock.json ]; then npm ci; else npm install; fi
npm run build
cd ..

echo "==> 构建后端"
go build -trimpath -ldflags="-s -w" -o bi.exe ./cmd/server
echo "==> 完成: $(pwd)/bi.exe"
