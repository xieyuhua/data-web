#!/usr/bin/env python3
"""企业微信 API mock with a realistic multi-level organization tree.

Endpoints (prefix /cgi-bin):
  GET /cgi-bin/gettoken                      -> access_token
  GET /cgi-bin/auth/getuserinfo?code=...     -> wecom userid
  GET /cgi-bin/department/list               -> department tree
  GET /cgi-bin/user/list                     -> member list

Department tree:
  1 总公司
    2 研发中心
      3 后端组        4 前端组        5 测试组        6 数据平台组
    7 销售中心
      8 华东销售部    9 华南销售部   10 华北销售部
    11 市场部       12 财务部        13 人力资源部    14 客户成功部

Run: python scripts/mock_wecom.py [--port 9999]
Point `wecom.apiBase` at http://127.0.0.1:9999 and use any code below to log in.
"""
import argparse
import json
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import urlparse, parse_qs

DEPARTMENTS = [
    {"id": 1, "name": "总公司", "parentid": 0, "order": 1},
    {"id": 2, "name": "研发中心", "parentid": 1, "order": 1},
    {"id": 3, "name": "后端组", "parentid": 2, "order": 1},
    {"id": 4, "name": "前端组", "parentid": 2, "order": 2},
    {"id": 5, "name": "测试组", "parentid": 2, "order": 3},
    {"id": 6, "name": "数据平台组", "parentid": 2, "order": 4},
    {"id": 7, "name": "销售中心", "parentid": 1, "order": 2},
    {"id": 8, "name": "华东销售部", "parentid": 7, "order": 1},
    {"id": 9, "name": "华南销售部", "parentid": 7, "order": 2},
    {"id": 10, "name": "华北销售部", "parentid": 7, "order": 3},
    {"id": 11, "name": "市场部", "parentid": 1, "order": 3},
    {"id": 12, "name": "财务部", "parentid": 1, "order": 4},
    {"id": 13, "name": "人力资源部", "parentid": 1, "order": 5},
    {"id": 14, "name": "客户成功部", "parentid": 1, "order": 6},
]

# (userid, name, departments, position, enabled)
MEMBERS = [
    # 管理层
    ("boss", "张明远", [1], "总经理", True),
    ("cto", "李建国", [1, 2], "首席技术官", True),
    # 研发中心
    ("rd_head", "王志强", [2], "研发总监", True),
    ("dev_backend1", "陈涛", [3], "后端工程师", True),
    ("dev_backend2", "刘洋", [3], "高级后端工程师", True),
    ("dev_backend3", "赵磊", [3], "后端工程师", True),
    ("dev_frontend1", "孙悦", [4], "前端工程师", True),
    ("dev_frontend2", "周琳", [4], "高级前端工程师", True),
    ("qa1", "吴静", [5], "测试工程师", True),
    ("qa2", "郑昊", [5], "测试主管", True),
    ("data1", "冯雪", [6], "数据工程师", True),
    ("data2", "陈鹏", [6, 2], "数据架构师", True),
    # 销售中心
    ("sales_head", "许文峰", [7], "销售总监", True),
    ("sales_east1", "何俊", [8], "华东销售经理", True),
    ("sales_east2", "罗敏", [8], "华东销售专员", True),
    ("sales_east3", "高磊", [8], "华东销售专员", True),
    ("sales_south1", "林芳", [9], "华南销售经理", True),
    ("sales_south2", "黄伟", [9], "华南销售专员", True),
    ("sales_north1", "马超", [10], "华北销售经理", True),
    ("sales_north2", "徐婷", [10], "华北销售专员", True),
    # 职能部门
    ("marketing1", "朱倩", [11], "市场经理", True),
    ("marketing2", "胡斌", [11], "内容运营", True),
    ("finance1", "郭丽", [12], "财务主管", True),
    ("finance2", "唐浩", [12], "会计", True),
    ("hr1", "邓娟", [13], "HRBP", True),
    ("hr2", "曹阳", [13], "招聘专员", True),
    ("cs1", "彭宇", [14], "客户成功经理", True),
    ("cs2", "袁莉", [14], "客户成功专员", True),
    # 边界场景
    ("intern1", "宋佳", [4], "前端实习生", False),   # 已离职
    ("auditor", "韩磊", [12], "外部审计顾问", True), # 外部顾问
]


def build_users():
    out = []
    for uid, name, depts, position, enabled in MEMBERS:
        out.append({
            "userid": uid,
            "name": name,
            "department": depts,
            "email": f"{uid}@example.com",
            "position": position,
            "mobile": f"138{abs(hash(uid)) % 100000000:08d}",
            "status": 1 if enabled else 4,
            "enabled": enabled,
        })
    return out


USERS = build_users()
# Captured outbound application messages (for tests and local inspection).
SENT = []
CODES = {f"code-{u['userid']}": u["userid"] for u in USERS}


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
        parsed = urlparse(self.path)
        length = int(self.headers.get("Content-Length", 0))
        raw = self.rfile.read(length) if length else b"{}"
        try:
            payload = json.loads(raw)
        except Exception:
            payload = {}

        if parsed.path == "/cgi-bin/message/send":
            received = payload.get("touser", "")
            msgtype = payload.get("msgtype", "")
            if msgtype == "textcard":
                card = payload.get("textcard", {})
                SENT.append({
                    "type": "textcard", "to": received,
                    "title": card.get("title", ""),
                    "description": card.get("description", ""),
                    "url": card.get("url", ""),
                    "btn": card.get("btntxt", ""),
                })
            elif msgtype == "text":
                SENT.append({
                    "type": "text", "to": received,
                    "content": payload.get("text", {}).get("content", ""),
                })
            else:
                return self._json({"errcode": 40008, "errmsg": "unsupported msgtype"})
            return self._json({"errcode": 0, "errmsg": "ok", "invaliduser": ""})
        if parsed.path == "/cgi-bin/message/clear":
            SENT.clear()
            return self._json({"errcode": 0, "errmsg": "ok"})
        return self._json({"errcode": 404, "errmsg": "not found"}, 404)

    def do_GET(self):
        parsed = urlparse(self.path)
        qs = parse_qs(parsed.query)
        path = parsed.path

        if path == "/cgi-bin/gettoken":
            if qs.get("corpid", [""])[0] == "wwtestcorp":
                return self._json({"errcode": 0, "errmsg": "ok",
                                   "access_token": "mock-token", "expires_in": 7200})
            return self._json({"errcode": 40013, "errmsg": "invalid corpid"})

        if path == "/cgi-bin/auth/getuserinfo":
            uid = CODES.get(qs.get("code", [""])[0])
            if uid:
                return self._json({"errcode": 0, "errmsg": "ok", "userid": uid})
            return self._json({"errcode": 40029, "errmsg": "invalid code"})

        if path == "/cgi-bin/department/list":
            return self._json({"errcode": 0, "errmsg": "ok", "department": DEPARTMENTS})

        if path == "/cgi-bin/user/list":
            return self._json({"errcode": 0, "errmsg": "ok", "userlist": USERS})

        if path == "/cgi-bin/message/list":
            return self._json({"errcode": 0, "errmsg": "ok", "messages": SENT})

        if path == "/cgi-bin/user/get":
            uid = qs.get("userid", [""])[0]
            for u in USERS:
                if u["userid"] == uid:
                    return self._json({"errcode": 0, "errmsg": "ok", **u})
            return self._json({"errcode": 60111, "errmsg": "user not found"})

        return self._json({"errcode": 404, "errmsg": "not found"}, 404)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--port", type=int, default=9999)
    args = parser.parse_args()
    srv = HTTPServer(("127.0.0.1", args.port), Handler)
    print(f"mock wecom on http://127.0.0.1:{args.port}")
    print(f"  departments: {len(DEPARTMENTS)}  members: {len(USERS)}")
    print(f"  sample codes: code-boss, code-rd_head, code-sales_east1, code-data1")
    srv.serve_forever()


if __name__ == "__main__":
    main()
