#!/usr/bin/env python3
"""
Transform prime_sbb_products (from go-index/PrestaShop) into the 'products'
index format expected by product-search-service.

Run manually or via cron after go-index sync completes.
"""
import json
import os
import sys
import re
import requests

MEILI_URL = os.getenv("MEILI_URL", "http://127.0.0.1:7700")
MEILI_KEY = os.getenv("MEILI_KEY", "61434e3b7b15c9c890dd0785d3bb9c555d3d831b217249f31aeb547532ac721b")
SRC_INDEX = os.getenv("SRC_INDEX", "prime_sbb_products")
DST_INDEX = os.getenv("DST_INDEX", "products")
BATCH_SIZE = int(os.getenv("BATCH_SIZE", "1000"))

headers = {"Authorization": f"Bearer {MEILI_KEY}", "Content-Type": "application/json"}


def meili_get(path, params=None):
    r = requests.get(f"{MEILI_URL}{path}", headers=headers, params=params, timeout=30)
    r.raise_for_status()
    return r.json()


def meili_post(path, body):
    r = requests.post(f"{MEILI_URL}{path}", headers=headers, json=body, timeout=60)
    r.raise_for_status()
    return r.json()


def meili_patch(path, body):
    r = requests.patch(f"{MEILI_URL}{path}", headers=headers, json=body, timeout=60)
    r.raise_for_status()
    return r.json()


def build_code_variations(reference: str, name: str) -> list:
    """Generate searchable code variations from reference and name."""
    variations = set()
    ref = reference.strip().upper()
    if ref:
        variations.add(ref)
        # without dots/dashes
        clean = re.sub(r"[.\-/ ]", "", ref)
        if clean != ref:
            variations.add(clean)
        # parts split by dot
        parts = ref.split(".")
        if len(parts) > 1:
            variations.add(parts[0])

    # extract code from name if format is "BRAND - CODE" or just "CODE"
    name_upper = name.strip().upper()
    code_match = re.search(r"-\s*([A-Z0-9]{4,}[A-Z0-9\-/]*)", name_upper)
    if code_match:
        code = code_match.group(1).strip()
        variations.add(code)
        clean = re.sub(r"[.\-/ ]", "", code)
        if clean != code:
            variations.add(clean)

    return list(variations)


def extract_brand(name: str) -> str:
    """Try to extract brand from name like '.DYZV - 22209CA/W33'."""
    # Pattern: optional dot + BRAND - rest
    m = re.match(r"^\.?([A-Z]{2,8})\s*[-–]", name.strip().upper())
    if m:
        return m.group(1)
    return ""


def format_price(price: float) -> str:
    if not price or price <= 0:
        return "-"
    return f"Rp {int(price):,}".replace(",", ".")


def transform(doc: dict) -> dict:
    ref = (doc.get("reference") or "").strip()
    name = (doc.get("name") or "").strip()
    price = float(doc.get("price") or 0)
    stock = int(doc.get("stock") or 0)
    active = bool(doc.get("active", True))
    brand = extract_brand(name)

    price_str = format_price(price)

    return {
        "id": doc.get("id_product") or doc.get("id"),
        "kode": ref,
        "nama": name,
        "brand": brand,
        "stok": stock,
        "active": active,
        "available": active and stock > 0,
        "harga_normal": price_str,
        "harga_customer": price_str,
        "harga_noncustomer": price_str,
        "harga_cash": price_str,
        "harga_normal_num": price,
        "harga_customer_num": price,
        "harga_noncustomer_num": price,
        "harga_cash_num": price,
        "code_variations": build_code_variations(ref, name),
        "lastUpdated": doc.get("lastUpdated", ""),
    }


def ensure_dst_index():
    try:
        meili_post(f"/indexes", {"uid": DST_INDEX, "primaryKey": "id"})
    except Exception:
        pass  # already exists
    # Set searchable/filterable attributes
    meili_patch(f"/indexes/{DST_INDEX}/settings", {
        "searchableAttributes": ["kode", "code_variations", "nama", "brand"],
        "filterableAttributes": ["stok", "active", "available", "code_variations"],
    })


def fetch_all_src():
    """Fetch all documents from source index with pagination."""
    offset = 0
    all_docs = []
    while True:
        data = meili_get(f"/indexes/{SRC_INDEX}/documents", params={"offset": offset, "limit": BATCH_SIZE})
        results = data.get("results", [])
        if not results:
            break
        all_docs.extend(results)
        print(f"  fetched {len(all_docs)} / {data.get('total', '?')}", end="\r")
        if len(all_docs) >= data.get("total", 0):
            break
        offset += BATCH_SIZE
    print()
    return all_docs


def push_to_dst(docs):
    """Bulk push transformed docs to destination index in batches."""
    for i in range(0, len(docs), BATCH_SIZE):
        batch = docs[i:i+BATCH_SIZE]
        meili_post(f"/indexes/{DST_INDEX}/documents", batch)
        print(f"  pushed {min(i+BATCH_SIZE, len(docs))} / {len(docs)}", end="\r")
    print()


def main():
    print(f"[transform] fetching from '{SRC_INDEX}'...")
    src_docs = fetch_all_src()
    if not src_docs:
        print("[transform] no documents found in source index, aborting")
        sys.exit(1)
    print(f"[transform] fetched {len(src_docs)} docs")

    print(f"[transform] transforming...")
    transformed = [transform(d) for d in src_docs]

    print(f"[transform] ensuring dst index '{DST_INDEX}'...")
    ensure_dst_index()

    print(f"[transform] pushing to '{DST_INDEX}'...")
    push_to_dst(transformed)
    print(f"[transform] done — {len(transformed)} products indexed")


if __name__ == "__main__":
    main()
