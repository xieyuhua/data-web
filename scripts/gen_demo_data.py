#!/usr/bin/env python3
"""Generate a rich demo SQLite database for Biaoge BI.

Creates a star-ish schema (dimensions + fact) with deterministic data:
  regions, products, customers, sales (fact), targets

Usage:
    python scripts/gen_demo_data.py [output_path] [--scale N]

Defaults to data/demo_sales.db with scale 1 (~10k orders). Scale 5 produces
~60k orders; scale 10 ~120k. Re-running rewrites the file.
"""
import argparse
import os
import random
import sqlite3
import sys
from datetime import date, timedelta

SEED = 20260101
DAY_START = date(2025, 1, 1)
DAY_END = date(2026, 11, 15)
SCALE = 1

REGIONS = [
    ("华东", "上海", "East"),
    ("华东", "杭州", "East"),
    ("华东", "南京", "East"),
    ("华南", "广州", "South"),
    ("华南", "深圳", "South"),
    ("华北", "北京", "North"),
    ("华北", "天津", "North"),
    ("西南", "成都", "Southwest"),
    ("西南", "重庆", "Southwest"),
    ("东北", "沈阳", "Northeast"),
    ("东北", "大连", "Northeast"),
    ("华中", "武汉", "Central"),
]

CATEGORIES = {
    "笔记本电脑": ["轻薄本", "游戏本", "商务本"],
    "显示器": ["27寸", "32寸", "带鱼屏"],
    "外设": ["键盘", "鼠标", "耳机"],
    "配件": ["扩展坞", "充电器", "数据线"],
    "办公设备": ["打印机", "扫描仪"],
}

CHANNELS = ["线上商城", "线下门店", "企业直销", "分销商", "海外代理"]
SEGMENTS = ["企业客户", "个人消费者", "政府机构", "教育行业", "医疗机构", "互联网公司"]
SALES_REPS = [f"销售{i:02d}" for i in range(1, 41)]


def build(path: str) -> None:
    global SCALE
    random.seed(SEED)
    if os.path.exists(path):
        os.remove(path)
    parent = os.path.dirname(path)
    if parent:
        os.makedirs(parent, exist_ok=True)

    con = sqlite3.connect(path)
    cur = con.cursor()
    cur.executescript(
        """
        PRAGMA journal_mode=WAL;
        CREATE TABLE regions (
            id INTEGER PRIMARY KEY,
            region TEXT NOT NULL,
            city TEXT NOT NULL,
            area_group TEXT NOT NULL
        );
        CREATE TABLE products (
            id INTEGER PRIMARY KEY,
            name TEXT NOT NULL,
            category TEXT NOT NULL,
            sub_category TEXT NOT NULL,
            unit_price REAL NOT NULL,
            cost REAL NOT NULL,
            launch_date TEXT NOT NULL
        );
        CREATE TABLE customers (
            id INTEGER PRIMARY KEY,
            name TEXT NOT NULL,
            segment TEXT NOT NULL,
            region_id INTEGER NOT NULL,
            channel TEXT NOT NULL,
            created_at TEXT NOT NULL
        );
        CREATE TABLE sales (
            id INTEGER PRIMARY KEY,
            order_no TEXT NOT NULL,
            order_date TEXT NOT NULL,
            product_id INTEGER NOT NULL,
            customer_id INTEGER NOT NULL,
            region_id INTEGER NOT NULL,
            channel TEXT NOT NULL,
            sales_rep TEXT NOT NULL,
            quantity INTEGER NOT NULL,
            unit_price REAL NOT NULL,
            amount REAL NOT NULL,
            cost REAL NOT NULL,
            profit REAL NOT NULL
        );
        CREATE TABLE targets (
            id INTEGER PRIMARY KEY,
            month TEXT NOT NULL,
            region TEXT NOT NULL,
            target_amount REAL NOT NULL
        );
        -- 目标调整单：同一月份/区域可有多条调整（加/减），用于计算「调整后目标」
        CREATE TABLE target_adjustments (
            id INTEGER PRIMARY KEY,
            month TEXT NOT NULL,
            region TEXT NOT NULL,
            adjust_amount REAL NOT NULL,
            reason TEXT NOT NULL,
            approved_at TEXT NOT NULL
        );
        CREATE INDEX idx_sales_date ON sales(order_date);
        CREATE INDEX idx_sales_region ON sales(region_id);
        CREATE INDEX idx_sales_product ON sales(product_id);
        CREATE INDEX idx_sales_channel ON sales(channel);
        CREATE INDEX idx_sales_rep ON sales(sales_rep);
        """
    )

    cur.executemany("INSERT INTO regions(id, region, city, area_group) VALUES (?,?,?,?)",
                    [(i + 1, r[0], r[1], r[2]) for i, r in enumerate(REGIONS)])

    products = []
    pid = 1
    for cat, subs in CATEGORIES.items():
        for sub in subs:
            for variant in range(1, 4):
                base = {
                    "轻薄本": (4500, 3200), "游戏本": (7800, 5900), "商务本": (5600, 4100),
                    "27寸": (1300, 900), "32寸": (2100, 1500), "带鱼屏": (2900, 2100),
                    "键盘": (260, 150), "鼠标": (160, 90), "耳机": (420, 260),
                    "扩展坞": (380, 230), "充电器": (180, 110), "数据线": (60, 25),
                    "打印机": (1500, 1080), "扫描仪": (1900, 1400),
                }[sub]
                price = round(base[0] * random.uniform(0.9, 1.25), 2)
                cost = round(base[1] * random.uniform(0.95, 1.1), 2)
                launch = DAY_START + timedelta(days=random.randint(-900, 600))
                products.append((pid, f"{cat}-{sub}-{variant}号", cat, sub,
                                 price, cost, launch.isoformat()))
                pid += 1
    cur.executemany("INSERT INTO products VALUES (?,?,?,?,?,?,?)", products)

    customers = []
    surnames = ["王", "李", "张", "刘", "陈", "杨", "赵", "黄", "周", "吴", "徐", "孙", "马", "朱", "胡"]
    suffixes = ["科技有限公司", "贸易有限公司", "实业集团", "信息技术公司", "商贸中心", "政府数据中心", "教育集团", "医院"]
    customer_count = 300 * SCALE
    for cid in range(1, customer_count + 1):
        name = random.choice(surnames) + random.choice(["伟", "芳", "强", "敏", "磊", "静", "洋", "丽", "军", "杰"])
        if cid % 3 == 0:
            name += random.choice(suffixes)
        customers.append((
            cid, name, random.choice(SEGMENTS), random.randint(1, len(REGIONS)),
            random.choice(CHANNELS),
            (DAY_START - timedelta(days=random.randint(0, 1500))).isoformat(),
        ))
    cur.executemany("INSERT INTO customers VALUES (?,?,?,?,?,?)", customers)

    # ~9000 orders with seasonal weighting and week-end uplift
    sales = []
    oid = 1
    day = DAY_START
    while day <= DAY_END:
        month = day.month
        seasonal = 1.0
        if month in (6, 11):        # 618 / 双11
            seasonal = 2.6
        elif month in (1, 12):      # 年末/年初
            seasonal = 1.5
        elif month in (2, 7):
            seasonal = 0.7
        weekend = 1.25 if day.weekday() >= 5 else 1.0
        n_orders = int(random.gauss(11, 3) * seasonal * weekend * SCALE)
        n_orders = max(2, min(45 * SCALE, n_orders))
        for _ in range(n_orders):
            p = random.choice(products)
            cust = random.choice(customers)
            qty = random.choice([1, 1, 1, 2, 2, 3, 4, 5, 8])
            unit_price = round(p[4] * random.uniform(0.93, 1.05), 2)
            amount = round(unit_price * qty, 2)
            cost = round(p[5] * qty, 2)
            sales.append((
                oid, f"SO{day.strftime('%Y%m%d')}{oid:06d}", day.isoformat(),
                p[0], cust[0], cust[3], cust[4], random.choice(SALES_REPS),
                qty, unit_price, amount, cost, round(amount - cost, 2),
            ))
            oid += 1
        day += timedelta(days=1)

    cur.executemany("INSERT INTO sales VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)", sales)

    targets = []
    tid = 1
    cur_month = date(2025, 1, 1)
    while cur_month <= date(2026, 11, 1):
        for r in REGIONS:
            monthly = random.uniform(1_800_000, 3_600_000)
            if cur_month.month in (6, 11):
                monthly *= 1.6
            targets.append((tid, cur_month.strftime("%Y-%m"), r[0], round(monthly, 2)))
            tid += 1
        cur_month = date(cur_month.year + (cur_month.month // 12),
                         (cur_month.month % 12) + 1, 1)

    cur.executemany("INSERT INTO targets VALUES (?,?,?,?)", targets)

    adjustments = []
    aid = 1
    reasons = ["季度冲刺上调", "市场收缩下调", "大客户中标追加", "渠道调整", "人员变动修正"]
    for t in targets:
        _, month, region, base = t
        if random.random() < 0.35:  # ~1/3 of region-months have an adjustment
            delta = round(float(base) * random.uniform(-0.18, 0.22), 2)
            adjustments.append((aid, month, region, delta, random.choice(reasons),
                                f"{month}-15"))
            aid += 1
    cur.executemany("INSERT INTO target_adjustments VALUES (?,?,?,?,?,?)", adjustments)
    con.commit()

    counts = {t: cur.execute(f"SELECT COUNT(*) FROM {t}").fetchone()[0]
              for t in ("regions", "products", "customers", "sales", "targets", "target_adjustments")}
    total = cur.execute("SELECT ROUND(SUM(amount),2), ROUND(SUM(profit),2) FROM sales").fetchone()
    con.close()
    print(f"created {path}")
    print(f"  rows: {counts}")
    print(f"  revenue: {total[0]:,.2f}  profit: {total[1]:,.2f}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("path", nargs="?", default=os.path.join("data", "demo_sales.db"))
    parser.add_argument("--scale", type=int, default=1, help="data multiplier (default 1)")
    args = parser.parse_args()
    SCALE = max(1, args.scale)
    build(args.path)
