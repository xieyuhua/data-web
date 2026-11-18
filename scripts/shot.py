#!/usr/bin/env python3
"""Capture screenshots at exact viewports via the Chrome DevTools Protocol.

Requires an Edge/Chrome started with --remote-debugging-port (default 9222).
Prints viewport metrics and any elements overflowing horizontally, which is the
fastest way to catch mobile layout regressions.

Usage:
    python scripts/shot.py --url http://127.0.0.1:2345/login --out shot.png --width 390 --height 844
"""
import argparse
import base64
import json
import sys
import time

import urllib.request
import websocket


def find_page(port):
    pages = json.load(urllib.request.urlopen(f"http://127.0.0.1:{port}/json/list"))
    for p in pages:
        if p["type"] == "page" and p.get("webSocketDebuggerUrl"):
            return p
    raise SystemExit("no debuggable page found")


class DevTools:
    def __init__(self, ws_url):
        self.ws = websocket.create_connection(ws_url, timeout=30)
        self.seq = 0

    def cmd(self, method, params=None):
        self.seq += 1
        self.ws.send(json.dumps({"id": self.seq, "method": method, "params": params or {}}))
        while True:
            msg = json.loads(self.ws.recv())
            if msg.get("id") == self.seq:
                if "error" in msg:
                    raise RuntimeError(msg["error"])
                return msg.get("result", {})

    def close(self):
        self.ws.close()


OVERFLOW_JS = """
(() => {
  const vw = window.innerWidth;
  const wide = [];
  document.querySelectorAll('body *').forEach((el) => {
    const r = el.getBoundingClientRect();
    if (r.width > 0 && r.right > vw + 1) {
      wide.push({
        tag: el.tagName,
        cls: (el.className || '').toString().slice(0, 70),
        right: Math.round(r.right),
        width: Math.round(r.width),
      });
    }
  });
  return JSON.stringify({
    innerWidth: vw,
    scrollWidth: document.documentElement.scrollWidth,
    overflow: wide.slice(0, 10),
  });
})()
"""


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--width", type=int, default=390)
    ap.add_argument("--height", type=int, default=844)
    ap.add_argument("--port", type=int, default=9222)
    ap.add_argument("--wait", type=float, default=3.0)
    ap.add_argument("--full-page", action="store_true")
    args = ap.parse_args()

    page = find_page(args.port)
    dev = DevTools(page["webSocketDebuggerUrl"])
    try:
        dev.cmd("Page.enable")
        dev.cmd("Emulation.setDeviceMetricsOverride", {
            "width": args.width,
            "height": args.height,
            "deviceScaleFactor": 1,
            "mobile": args.width < 768,
        })
        dev.cmd("Page.navigate", {"url": args.url})
        time.sleep(args.wait)

        metrics = dev.cmd("Runtime.evaluate", {
            "expression": OVERFLOW_JS, "returnByValue": True,
        })["result"]["value"]
        print(metrics)

        params = {"format": "png"}
        if args.full_page:
            layout = dev.cmd("Page.getLayoutMetrics")["cssContentSize"]
            params["clip"] = {
                "x": 0, "y": 0,
                "width": layout["width"], "height": layout["height"],
                "scale": 1,
            }
        shot = dev.cmd("Page.captureScreenshot", params)
        with open(args.out, "wb") as f:
            f.write(base64.b64decode(shot["data"]))
        print(f"saved {args.out} ({args.width}x{args.height})")
    finally:
        dev.close()


if __name__ == "__main__":
    sys.exit(main())
