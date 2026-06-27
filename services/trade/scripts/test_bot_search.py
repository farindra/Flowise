#!/usr/bin/env python3
"""
Test script untuk bot search logic.

Apa yang ditest:
  1. isApplicationQuery — apakah query dideteksi sebagai aplikasi kendaraan (→ Flowise)
  2. product-search-service API — hasil pencarian Meilisearch per query

Usage:
  python3 test_bot_search.py              # run semua
  python3 test_bot_search.py --search     # hanya search API
  python3 test_bot_search.py --routing    # hanya routing logic
  python3 test_bot_search.py -q "brio"   # cari satu query ad-hoc
"""
import sys
import re
import json
import urllib.request
import urllib.parse

SEARCH_URL = "http://127.0.0.1:8101/search"

# ── replica of isApplicationQuery dari search.go ──────────────────────────────
VEHICLE_TERMS = [
    # brands
    "honda", "toyota", "mitsubishi", "daihatsu", "suzuki", "nissan", "isuzu",
    "hyundai", "kia", "mazda", "bmw", "mercedes", "benz", "vw", "volkswagen",
    "ford", "chevrolet", "wuling", "chery", "dfsk", "hino", "fuso",
    # models
    "brio", "jazz", "freed", "civic", "accord", "crv", "hrv", "brv",
    "avanza", "xenia", "innova", "rush", "fortuner", "hilux", "kijang",
    "calya", "sigra", "agya", "ayla", "terios", "sirion", "gran max",
    "ertiga", "carry", "apv", "swift", "ignis",
    "l300", "l200", "strada", "pajero", "triton", "galant", "colt",
    "grand livina", "x-trail", "serena", "navara",
    "yaris", "vios", "camry", "alphard", "vellfire", "land cruiser",
    "trax", "captiva",
    # application parts
    "roda belakang", "roda depan", "as roda", "gardan", "transmisi",
    "setir", "kemudi", "kopling", "koppeling", "mesin", "pompa air",
]

BEARING_WORDS = ["bearing", "laher", "seal", "belt", "bantalan"]

def is_application_query(query: str) -> bool:
    lower = query.lower()
    has_bearing = any(bw in lower for bw in BEARING_WORDS) or "as " in lower
    if not has_bearing:
        return False
    return any(v in lower for v in VEHICLE_TERMS)


# ── product-search-service ─────────────────────────────────────────────────────
def search(q: str, limit: int = 5) -> list:
    params = urllib.parse.urlencode({"q": q, "limit": limit})
    url = f"{SEARCH_URL}?{params}"
    try:
        with urllib.request.urlopen(url, timeout=5) as r:
            data = json.loads(r.read())
            return data.get("products", []) or data.get("results", []) or (data if isinstance(data, list) else [])
    except Exception as e:
        return [{"error": str(e)}]


# ── display helpers ────────────────────────────────────────────────────────────
GREEN  = "\033[92m"
RED    = "\033[91m"
YELLOW = "\033[93m"
CYAN   = "\033[96m"
BOLD   = "\033[1m"
RESET  = "\033[0m"

def ok(msg):  print(f"  {GREEN}✓{RESET} {msg}")
def fail(msg): print(f"  {RED}✗{RESET} {msg}")
def info(msg): print(f"  {YELLOW}~{RESET} {msg}")


# ── Test 1: Routing logic ──────────────────────────────────────────────────────
ROUTING_CASES = [
    # (query, expected_is_application, description)
    ("Kalau bearing belakang brio ada?",       True,  "brio + bearing → Flowise"),
    ("ada bearing toyota avanza ga untuk roda belakang?", True, "avanza + bearing → Flowise"),
    ("laher honda jazz depan",                 True,  "jazz + laher → Flowise"),
    ("bearing hilux roda belakang",            True,  "hilux + bearing → Flowise"),
    ("seal honda civic transmisi",             True,  "civic + seal → Flowise"),
    # negative cases
    ("6205 2RS SKF",                           False, "kode + brand → Meilisearch"),
    ("NTN 6205",                               False, "brand + kode → Meilisearch"),
    ("bearing 6204",                           False, "generic bearing + kode → Meilisearch"),
    ("FAG 6205 2RS C3",                        False, "brand + kode → Meilisearch"),
    ("SF 05 A 84",                             False, "specific bearing code → Meilisearch"),
    ("stok 6301 2RS",                          False, "stok + kode → Meilisearch"),
]

def test_routing():
    print(f"\n{BOLD}═══ TEST 1: Routing Logic (isApplicationQuery) ═══{RESET}")
    passed = failed = 0
    for query, expected, desc in ROUTING_CASES:
        got = is_application_query(query)
        route = f"{CYAN}→ Flowise{RESET}" if got else "→ Meilisearch"
        if got == expected:
            ok(f"{desc}  [{route} ]  \"{query}\"")
            passed += 1
        else:
            fail(f"{desc}  [{route} ] EXPECTED {'Flowise' if expected else 'Meilisearch'}  \"{query}\"")
            failed += 1
    print(f"\n  {BOLD}Routing: {passed} passed, {failed} failed{RESET}")
    return failed == 0


# ── Test 2: Search API ─────────────────────────────────────────────────────────
SEARCH_CASES = [
    # (query, min_results, must_contain_kode_or_keterangan_substr, description)
    ("SF 05 A 84",       1, "58767416",  "exact nama → harus ketemu kode 58767416"),
    ("6205 2RS",         1, "6205",      "kode bearing standar"),
    ("6301 2RS",         1, "6301",      "kode bearing standar"),
    ("SKF 6205",         1, "6205",      "brand + kode"),
    ("NTN",              1, "NTN",       "brand only → harus ada produk NTN"),
    ("brio",             1, "58767416",  "nama kendaraan → harus ketemu HON BRIO"),
    ("avanza",           1, None,        "nama kendaraan avanza → ada hasil"),
    ("625 ZZ",           1, None,        "nama bearing → ada hasil"),
    # "bearing roda belakang brio" → isApplicationQuery=True → Flowise, bukan Meilisearch.
    # Tapi "brio" saja (tanpa kata bearing) → Meilisearch → harus ketemu.
    ("brio",                     1, "58767416",  "nama kendaraan brio saja → kode 58767416"),
]

def format_product(p: dict) -> str:
    kode = p.get("kode", p.get("Kode", "?"))
    nama = p.get("nama", p.get("Nama", "?"))
    stok = p.get("stok", p.get("Stok", "?"))
    ket  = p.get("keterangan", p.get("Keterangan", ""))
    s = f"{kode} | {nama} | stok:{stok}"
    if ket:
        s += f" | {ket[:60]}"
    return s

def test_search():
    print(f"\n{BOLD}═══ TEST 2: Search API (product-search-service :8101) ═══{RESET}")
    passed = failed = 0

    for query, min_results, must_have, desc in SEARCH_CASES:
        results = search(query, limit=5)
        if results and "error" in results[0]:
            fail(f"{desc} → ERROR: {results[0]['error']}")
            failed += 1
            continue

        count = len(results)
        has_min = count >= min_results
        has_kode = True
        if must_have:
            has_kode = any(
                must_have.lower() in json.dumps(p).lower()
                for p in results
            )

        if has_min and has_kode:
            ok(f"{desc}  ({count} hasil)")
            for p in results[:2]:
                print(f"       {format_product(p)}")
            passed += 1
        else:
            problems = []
            if not has_min:
                problems.append(f"hanya {count} hasil (min {min_results})")
            if not has_kode:
                problems.append(f"tidak ada '{must_have}'")
            fail(f"{desc}: {', '.join(problems)}")
            for p in results[:3]:
                print(f"       {format_product(p)}")
            failed += 1

    print(f"\n  {BOLD}Search: {passed} passed, {failed} failed{RESET}")
    return failed == 0


# ── Ad-hoc search ──────────────────────────────────────────────────────────────
def ad_hoc_search(query: str):
    is_app = is_application_query(query)
    route = f"{CYAN}Flowise (application query){RESET}" if is_app else "Meilisearch (keyword search)"
    print(f"\n{BOLD}Query:{RESET} \"{query}\"")
    print(f"{BOLD}Route:{RESET} {route}")

    results = search(query, limit=10)
    if not results:
        print(f"{RED}Tidak ada hasil.{RESET}")
        return
    if "error" in results[0]:
        print(f"{RED}Error: {results[0]['error']}{RESET}")
        return

    print(f"{BOLD}{len(results)} produk ditemukan:{RESET}")
    for i, p in enumerate(results, 1):
        print(f"  {i}. {format_product(p)}")


# ── main ───────────────────────────────────────────────────────────────────────
if __name__ == "__main__":
    args = sys.argv[1:]

    if "-q" in args:
        idx = args.index("-q")
        query = args[idx + 1] if idx + 1 < len(args) else ""
        if query:
            ad_hoc_search(query)
            sys.exit(0)
        else:
            print("Usage: -q \"query string\"")
            sys.exit(1)

    run_routing = "--search" not in args
    run_search  = "--routing" not in args

    all_ok = True
    if run_routing:
        all_ok &= test_routing()
    if run_search:
        all_ok &= test_search()

    print()
    if all_ok:
        print(f"{GREEN}{BOLD}✓ Semua test PASSED{RESET}")
    else:
        print(f"{RED}{BOLD}✗ Ada test yang FAILED{RESET}")
        sys.exit(1)
