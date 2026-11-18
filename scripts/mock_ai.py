#!/usr/bin/env python3
"""Minimal OpenAI-compatible chat completions mock for local testing.

POST /v1/chat/completions returns deterministic answers based on the prompt so
the SQL-generation, parameter-suggestion, metadata and completion flows can be
exercised without a real model.

Run: python scripts/mock_ai.py [--port 9998]
Then set in 系统配置 → AI:
  baseUrl = http://127.0.0.1:9998/v1
  model   = mock-model
  apiKey  = anything
"""
import argparse
import json
import re
from http.server import BaseHTTPRequestHandler, HTTPServer

SAMPLE_SQL = (
    "SELECT r.region, "
    "STRFTIME('%Y-%m', s.order_date) AS month, "
    "SUM(s.amount) AS sales_amount, "
    "SUM(s.profit) AS profit_amount "
    "FROM sales s JOIN regions r ON r.id = s.region_id "
    "WHERE s.order_date >= {{from_date}} AND s.order_date <= {{to_date}} "
    "GROUP BY r.region, STRFTIME('%Y-%m', s.order_date) "
    "ORDER BY month, r.region"
)


def answer_for(prompt: str) -> str:
    p = prompt.lower()

    # Health check
    if "health check" in p or p.strip() == "ping":
        return "ok"

    # Parameter definitions
    if "参数定义" in prompt or "name, label" in prompt:
        names = sorted(set(re.findall(r"\{\{([a-z_][a-z0-9_]*)\}\}", prompt)))
        out = []
        for n in names:
            typ = "date" if ("date" in n or "month" in n) else (
                "number" if ("amount" in n or "limit" in n) else "string")
            out.append({
                "name": n,
                "label": n.replace("_", " ").title(),
                "type": typ,
                "required": True,
                "defaultValue": "2026-01-01" if typ == "date" else "",
            })
        return json.dumps(out, ensure_ascii=False)

    # Dataset name / description
    if '"name"' in prompt and "description" in prompt:
        return json.dumps({
            "name": "区域月度销售分析",
            "description": "按区域与月份汇总销售额和利润，支持日期区间筛选",
        }, ensure_ascii=False)

    # Inline completion
    if "继续补全" in prompt or "只补全后续片段" in prompt:
        return ", COUNT(*) AS order_count"

    # SQL generation
    if "需求" in prompt or "只输出 sql" in p:
        return SAMPLE_SQL

    return SAMPLE_SQL


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def _json(self, payload, status=200):
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        raw = self.rfile.read(length) if length else b"{}"
        try:
            payload = json.loads(raw)
        except Exception:
            payload = {}

        if self.path.rstrip("/").endswith("/chat/completions"):
            messages = payload.get("messages", [])
            user = next((m.get("content", "") for m in messages if m.get("role") == "user"), "")
            content = answer_for(user)
            return self._json({
                "id": "mock-1",
                "object": "chat.completion",
                "model": payload.get("model", "mock-model"),
                "choices": [{
                    "index": 0,
                    "message": {"role": "assistant", "content": content},
                    "finish_reason": "stop",
                }],
                "usage": {"prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150},
            })
        return self._json({"error": {"message": "not found"}}, 404)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--port", type=int, default=9998)
    args = ap.parse_args()
    srv = HTTPServer(("127.0.0.1", args.port), Handler)
    print(f"mock AI on http://127.0.0.1:{args.port}/v1  (model: mock-model)")
    srv.serve_forever()


if __name__ == "__main__":
    main()
