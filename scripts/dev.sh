#!/usr/bin/env bash
# 开发模式：后端 dev tag（不嵌入前端），前端 Vite 代理 /api。
set -e
cd "$(dirname "$0")/.."

if [ ! -d web/node_modules ]; then
  (cd web && npm install)
fi

echo "后端: http://localhost:2345  （go run -tags dev）"
echo "前端: http://localhost:5173  （npm run dev）"
echo "分别开两个终端执行以下命令："
echo "  go run -tags dev ./cmd/server"
echo "  cd web && npm run dev"
