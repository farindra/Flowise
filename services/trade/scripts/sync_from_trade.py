#!/usr/bin/env python3
"""
Sync produk dari MySQL oceanbearings_prime (trade server 212.85.25.235)
ke Meilisearch 'products' index yang dipakai product-search-service.

Dijalankan via cron di Flowise server — SSH ke trade server, query MySQL,
transform, push ke Meilisearch lokal.
"""
import json
import os
import re
import subprocess
import sys
import requests

# --- Config ---
TRADE_SSH_HOST = os.getenv("TRADE_SSH_HOST", "212.85.25.235")
TRADE_SSH_USER = os.getenv("TRADE_SSH_USER", "root")
TRADE_DB_USER = os.getenv("TRADE_DB_USER", "ob_prime_user")
TRADE_DB_PASS = os.getenv("TRADE_DB_PASS", "Obp2025_DbStrong")
TRADE_DB_NAME = os.getenv("TRADE_DB_NAME", "oceanbearings_prime")
TRADE_SHOP_ID = os.getenv("TRADE_SHOP_ID", "1")

MEILI_URL = os.getenv("MEILI_URL", "http://127.0.0.1:7700")
MEILI_KEY = os.getenv("MEILI_KEY", "61434e3b7b15c9c890dd0785d3bb9c555d3d831b217249f31aeb547532ac721b")
DST_INDEX = os.getenv("DST_INDEX", "products")
BATCH_SIZE = int(os.getenv("BATCH_SIZE", "500"))

headers = {"Authorization": f"Bearer {MEILI_KEY}", "Content-Type": "application/json"}

MYSQL_QUERY = f"""
SELECT
  a.id_product,
  COALESCE(a.reference, '') AS reference,
  COALESCE(b.name, '') AS name,
  COALESCE(c.quantity, 0) AS quantity,
  COALESCE(a.price, 0) AS price,
  COALESCE(a.wholesale_price, 0) AS wholesale_price,
  COALESCE(b.description_short, '') AS keterangan
FROM ob_product a
JOIN ob_product_lang b ON a.id_product = b.id_product AND b.id_lang = 1
JOIN ob_stock_available c ON a.id_product = c.id_product AND b.id_shop = c.id_shop
WHERE b.id_shop = {TRADE_SHOP_ID} AND a.active = 1
ORDER BY a.id_product;
"""


def fetch_from_trade():
    """SSH ke trade server, query MySQL, return list of dicts."""
    cmd = [
        "ssh", "-o", "ConnectTimeout=15", "-o", "BatchMode=yes",
        f"{TRADE_SSH_USER}@{TRADE_SSH_HOST}",
        f"mysql -u {TRADE_DB_USER} -p{TRADE_DB_PASS} {TRADE_DB_NAME} "
        f"--batch --skip-column-names -e \"{MYSQL_QUERY.strip()}\""
    ]
    result = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
    if result.returncode != 0:
        print(f"[sync] SSH/MySQL error: {result.stderr[:200]}", file=sys.stderr)
        sys.exit(1)

    rows = []
    for line in result.stdout.strip().splitlines():
        parts = line.split("\t")
        if len(parts) < 6:
            continue
        rows.append({
            "id_product": int(parts[0]),
            "reference": parts[1].strip(),
            "name": parts[2].strip(),
            "quantity": int(parts[3]) if parts[3].isdigit() else 0,
            "price": float(parts[4]) if parts[4] else 0.0,
            "wholesale_price": float(parts[5]) if parts[5] else 0.0,
            "keterangan": parts[6].strip() if len(parts) > 6 else "",
        })
    return rows


def extract_brand(name: str) -> str:
    """Ambil brand dari nama format '6205 ZZ.FAG' atau '6205 ZZ (KOREA).FAG'."""
    # Format: nama.BRAND (dot di akhir)
    m = re.search(r'\.([A-Z][A-Z0-9\-]{1,10})$', name.strip())
    if m:
        return m.group(1)
    return ""


def clean_name(name: str) -> str:
    """Hapus .BRAND di akhir nama."""
    return re.sub(r'\.[A-Z][A-Z0-9\-]{1,10}$', '', name.strip()).strip()


def build_code_variations(reference: str, name: str) -> list:
    variations = set()
    ref = reference.strip().upper()
    if ref:
        variations.add(ref)
        clean = re.sub(r"[.\-/ ]", "", ref)
        if clean != ref:
            variations.add(clean)

    # Kode dari nama (sebelum dot brand)
    base_name = clean_name(name).upper()
    # Ambil token pertama yang mengandung digit (kode bearing)
    tokens = base_name.split()
    code_parts = []
    for t in tokens:
        if re.search(r'\d', t):
            code_parts.append(re.sub(r"[.\-/ ]", "", t))
    if code_parts:
        variations.add("".join(code_parts))

    return list(v for v in variations if v)


def format_price(price: float) -> str:
    if not price or price <= 0:
        return "-"
    return f"Rp {int(price):,}".replace(",", ".")


def transform(row: dict) -> dict:
    name = row["name"]
    brand = extract_brand(name)
    clean = clean_name(name)
    ref = row["reference"]
    price = row["price"]
    ws_price = row["wholesale_price"]
    stok = row["quantity"]

    price_str = format_price(price)
    ws_str = format_price(ws_price) if ws_price > 0 else price_str

    return {
        "id": row["id_product"],
        "kode": ref,
        "nama": clean,
        "brand": brand,
        "stok": stok,
        "active": True,
        "available": stok > 0,
        "harga_normal": price_str,
        "harga_customer": price_str,
        "harga_noncustomer": price_str,
        "harga_cash": ws_str,
        "harga_normal_num": price,
        "harga_customer_num": price,
        "harga_noncustomer_num": price,
        "harga_cash_num": ws_price if ws_price > 0 else price,
        "code_variations": build_code_variations(ref, name),
        "keterangan": row.get("keterangan", ""),
        "lastUpdated": "",
    }


def meili_post(path, body):
    r = requests.post(f"{MEILI_URL}{path}", headers=headers, json=body, timeout=60)
    r.raise_for_status()
    return r.json()


def meili_patch(path, body):
    r = requests.patch(f"{MEILI_URL}{path}", headers=headers, json=body, timeout=60)
    r.raise_for_status()
    return r.json()


def wait_task(task_uid: int, timeout: int = 120):
    """Poll Meilisearch task until succeeded or failed."""
    import time
    for _ in range(timeout // 2):
        r = requests.get(f"{MEILI_URL}/tasks/{task_uid}", headers=headers, timeout=10)
        status = r.json().get("status")
        if status in ("succeeded", "failed", "canceled"):
            return status
        time.sleep(2)
    return "timeout"


def clear_index():
    """Delete all documents and wait for completion."""
    r = requests.delete(f"{MEILI_URL}/indexes/{DST_INDEX}/documents", headers=headers, timeout=30)
    if r.status_code == 202:
        task_uid = r.json().get("taskUid")
        status = wait_task(task_uid)
        print(f"[sync] index cleared (task {task_uid}: {status})")


def ensure_index():
    try:
        meili_post("/indexes", {"uid": DST_INDEX, "primaryKey": "id"})
    except Exception:
        pass
    meili_patch(f"/indexes/{DST_INDEX}/settings", {
        "searchableAttributes": ["kode", "code_variations", "nama", "brand", "keterangan"],
        "filterableAttributes": ["stok", "active", "available", "code_variations"],
    })


def push_docs(docs):
    for i in range(0, len(docs), BATCH_SIZE):
        batch = docs[i:i+BATCH_SIZE]
        meili_post(f"/indexes/{DST_INDEX}/documents", batch)
        print(f"  pushed {min(i+BATCH_SIZE, len(docs))} / {len(docs)}", end="\r")
    print()


def main():
    print(f"[sync] fetching from {TRADE_SSH_HOST} MySQL ({TRADE_DB_NAME})...")
    rows = fetch_from_trade()
    if not rows:
        print("[sync] tidak ada data, abort")
        sys.exit(1)
    print(f"[sync] {len(rows)} produk ditemukan")

    print("[sync] transforming...")
    docs = [transform(r) for r in rows]

    print(f"[sync] clearing old data from '{DST_INDEX}'...")
    ensure_index()
    clear_index()

    print(f"[sync] pushing to Meilisearch...")
    push_docs(docs)
    print(f"[sync] selesai — {len(docs)} produk di-index")


if __name__ == "__main__":
    main()
