#!/usr/bin/env python3
"""
End-to-end flow test untuk wa-gateway-service via /simulate endpoint.

Skenario yang ditest:
  1. Search kode langsung → pilih produk → masukkan qty → cart
  2. Search kendaraan → verifikasi Flowise handle (bukan keyword split)
  3. Rubah qty di cart (ubah 1 X)
  4. Hapus item dari cart (hapus 1)
  5. Tambah beberapa produk ke cart sekaligus
  6. Lihat keranjang (/cart)

Usage:
  python3 test_bot_flow.py              # run semua skenario
  python3 test_bot_flow.py -s 1         # hanya skenario 1
  python3 test_bot_flow.py --verbose    # tampilkan semua reply

Requirements:
  - wa-gateway di :8100
  - Phone test 628000000001 sudah di-register di SQLite
    (jalankan: python3 test_bot_flow.py --setup jika belum)
"""
import sys
import json
import time
import subprocess
import urllib.request
import urllib.parse
import sqlite3

GATEWAY_URL  = "http://127.0.0.1:8100/simulate"
QR_TOKEN     = "ob-wa-qr-2026"
TEST_PHONE   = "628000000001"
STATE_DB     = "/www/wwwroot/agentic.oceanbearings.co.id/services/trade/data/wa-session/state.db"

# ── Colors ────────────────────────────────────────────────────────────────────
GREEN  = "\033[92m"; RED    = "\033[91m"; YELLOW = "\033[93m"
CYAN   = "\033[96m"; BOLD   = "\033[1m";  DIM    = "\033[2m"; RESET  = "\033[0m"

VERBOSE = "--verbose" in sys.argv or "-v" in sys.argv

def ok(msg):    print(f"  {GREEN}✓{RESET} {msg}")
def fail(msg):  print(f"  {RED}✗{RESET} {msg}")
def info(msg):  print(f"  {YELLOW}~{RESET} {msg}")
def sep(title): print(f"\n{BOLD}{'═'*55}\n  {title}\n{'═'*55}{RESET}")

# ── SQLite helpers ─────────────────────────────────────────────────────────────
def setup_phone():
    """Register test phone so it skips registration flow."""
    conn = sqlite3.connect(STATE_DB)
    conn.execute("DELETE FROM user_data WHERE phone_number = ?", (TEST_PHONE,))
    conn.execute(
        "INSERT INTO user_data (phone_number, data, updated_at) VALUES (?, ?, strftime('%s','now'))",
        (TEST_PHONE, '{"isRegistered":true,"company":"Test Company","region":"jakarta","state":"idle"}')
    )
    conn.commit(); conn.close()
    print(f"  {GREEN}setup:{RESET} phone {TEST_PHONE} registered")

def clear_cart():
    """Reset cart + activeConversation for test phone."""
    conn = sqlite3.connect(STATE_DB)
    row = conn.execute("SELECT data FROM user_data WHERE phone_number=?", (TEST_PHONE,)).fetchone()
    if row:
        data = json.loads(row[0])
        data.pop("cart", None)
        data.pop("activeConversation", None)
        data.pop("conversationHistory", None)
        data["state"] = "idle"
        conn.execute(
            "UPDATE user_data SET data=?, updated_at=strftime('%s','now') WHERE phone_number=?",
            (json.dumps(data), TEST_PHONE)
        )
    conn.commit(); conn.close()

def get_cart():
    """Return cart from SQLite for test phone."""
    conn = sqlite3.connect(STATE_DB)
    row = conn.execute("SELECT data FROM user_data WHERE phone_number=?", (TEST_PHONE,)).fetchone()
    conn.close()
    if not row:
        return []
    data = json.loads(row[0])
    return data.get("cart", [])

# ── HTTP helper ────────────────────────────────────────────────────────────────
def send(message: str, delay: float = 0.5) -> list[str]:
    """Send a simulated message and return list of replies."""
    payload = json.dumps({"phone": TEST_PHONE, "message": message, "token": QR_TOKEN}).encode()
    req = urllib.request.Request(
        GATEWAY_URL,
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=120) as r:
            resp = json.loads(r.read())
            replies = resp.get("replies", [])
            elapsed = resp.get("elapsed", "")
            if VERBOSE:
                print(f"\n  {DIM}→ \"{message}\"{RESET}")
                for r in replies:
                    print(f"  {CYAN}← {r[:120]}{'...' if len(r)>120 else ''}{RESET}")
                print(f"  {DIM}({elapsed}){RESET}")
            time.sleep(delay)
            return replies
    except Exception as e:
        print(f"  {RED}HTTP error: {e}{RESET}")
        return []

def replies_contain(replies: list[str], *keywords) -> bool:
    combined = " ".join(replies).lower()
    return all(kw.lower() in combined for kw in keywords)

def extract_product_kode(replies: list[str]) -> str | None:
    """Extract first product kode (numeric 8-digit) from search result replies."""
    import re
    combined = " ".join(replies)
    m = re.search(r'\b(5\d{7}|6\d{7})\b', combined)
    return m.group(1) if m else None

# ══════════════════════════════════════════════════════════════════════════════
# Skenario 1: Search kode → pilih → qty → cart
# ══════════════════════════════════════════════════════════════════════════════
def scenario_1():
    sep("SKENARIO 1: Search kode → pilih produk → qty → cart")
    clear_cart()
    passed = failed = 0

    # 1a. Search kode bearing standar
    replies = send("6205 2RS")
    if replies_contain(replies, "6205") and any(
        kw in " ".join(replies).lower()
        for kw in ("kode", "brand", "harga", "stok", "ada", "ditemukan", "hasil")
    ):
        ok("1a. Search '6205 2RS' → dapat hasil pencarian"); passed += 1
    else:
        fail(f"1a. Search '6205 2RS' tidak dapat hasil\n       replies: {replies[:1]}"); failed += 1

    # 1b. Pilih nomor 1 dari hasil
    replies = send("1")
    cart = get_cart()
    if cart or replies_contain(replies, "berapa", "qty", "jumlah", "pcs", "keranjang", "ditambahkan"):
        ok(f"1b. Ketik '1' → {('masuk cart' if cart else 'tanya qty / respon valid')}"); passed += 1
    else:
        fail(f"1b. Ketik '1' tidak valid\n       replies: {replies[:1]}"); failed += 1

    # 1c. Masukkan qty jika bot tanya
    if not cart:
        replies = send("3")
        cart = get_cart()
        if cart:
            ok(f"1c. Ketik '3' → produk masuk cart (qty={cart[0].get('quantity')})"); passed += 1
        else:
            # Cek reply mengandung "ditambahkan" atau sejenisnya
            if replies_contain(replies, "ditambahkan", "keranjang", "berhasil"):
                ok("1c. Ketik '3' → cart berhasil (reply confirms)"); passed += 1
                # Re-read cart setelah delay
                time.sleep(1)
                cart = get_cart()
            else:
                fail(f"1c. Ketik '3' → tidak masuk cart\n       replies: {replies[:1]}"); failed += 1
    else:
        info("1c. Produk sudah di cart dari step 1b (skip qty step)")

    # 1d. Cek keranjang
    replies = send("keranjang")
    cart = get_cart()
    if cart and replies_contain(replies, "keranjang"):
        total = sum(i.get("quantity", 0) * i.get("harga", 0) for i in cart)
        ok(f"1d. '/keranjang' → {len(cart)} item(s), total Rp {total:,.0f}"); passed += 1
    else:
        fail(f"1d. Keranjang kosong atau reply salah\n       cart={cart}\n       replies: {replies[:1]}"); failed += 1

    print(f"\n  {BOLD}Skenario 1: {passed} passed, {failed} failed{RESET}")
    return failed == 0, cart

# ══════════════════════════════════════════════════════════════════════════════
# Skenario 2: Search berdasarkan informasi kendaraan
# ══════════════════════════════════════════════════════════════════════════════
def scenario_2():
    sep("SKENARIO 2: Search kendaraan → Flowise handle (bukan keyword split)")
    passed = failed = 0

    # 2a. Query kendaraan → harus Flowise (bukan list perkata ngaco)
    replies = send("Kalau bearing belakang brio ada?", delay=1.0)
    combined = " ".join(replies).lower()
    is_keyword_split = "bearing, belakang, brio" in combined or "bearing,belakang,brio" in combined
    has_product_info = any(x in combined for x in ["sf 05", "58767416", "ntn", "brio", "honda"])
    has_reply = bool(replies)

    if has_reply and not is_keyword_split:
        if has_product_info:
            ok("2a. 'bearing belakang brio' → Flowise menemukan produk spesifik"); passed += 1
        else:
            ok("2a. 'bearing belakang brio' → Flowise handle (tidak keyword split)"); passed += 1
    elif is_keyword_split:
        fail("2a. Masih keyword split 'bearing, belakang, brio' — isApplicationQuery tidak jalan"); failed += 1
    else:
        fail(f"2a. Tidak ada reply\n       replies: {replies}"); failed += 1

    # 2b. Follow-up ke Flowise → tidak boleh "tidak mengerti"
    replies = send("3", delay=0.5)
    bad_replies = ["tidak dapat memahami", "tidak mengerti", "tidak dikenali"]
    if replies and not any(b in " ".join(replies).lower() for b in bad_replies):
        ok("2b. Follow-up '3' ke context Flowise → valid response"); passed += 1
    else:
        fail(f"2b. Follow-up '3' → error response\n       replies: {replies[:1]}"); failed += 1

    # 2c. Query avanza
    replies = send("ada bearing roda depan avanza?", delay=1.0)
    combined = " ".join(replies).lower()
    is_keyword_split2 = any(x in combined for x in ["bearing, roda", "roda, depan", "avanza, bearing"])
    if replies and not is_keyword_split2:
        ok("2c. 'bearing roda depan avanza' → tidak keyword split"); passed += 1
    else:
        fail(f"2c. Masih keyword split untuk avanza\n       replies: {replies[:1]}"); failed += 1

    print(f"\n  {BOLD}Skenario 2: {passed} passed, {failed} failed{RESET}")
    return failed == 0

# ══════════════════════════════════════════════════════════════════════════════
# Skenario 3: Rubah qty di cart
# ══════════════════════════════════════════════════════════════════════════════
def scenario_3(cart_from_s1):
    sep("SKENARIO 3: Rubah qty di cart (ubah <idx> <qty>)")
    passed = failed = 0

    if not cart_from_s1:
        # Setup cart dulu jika skenario 1 tidak jalan
        clear_cart()
        send("6205 2RS")
        send("1")
        send("3")
        time.sleep(0.5)

    cart_before = get_cart()
    if not cart_before:
        fail("3.setup. Cart kosong, tidak bisa test ubah qty"); return False
    info(f"Cart sebelum ubah: {len(cart_before)} item(s), item[0] qty={cart_before[0].get('quantity')}")

    # 3a. Ubah qty item 1 jadi 10
    replies = send("ubah 1 10")
    cart_after = get_cart()
    if cart_after and cart_after[0].get("quantity") == 10:
        ok(f"3a. 'ubah 1 10' → qty berhasil diubah ke 10"); passed += 1
    elif replies_contain(replies, "diubah", "berhasil", "qty", "jumlah"):
        ok("3a. 'ubah 1 10' → reply confirms ubah qty"); passed += 1
    else:
        fail(f"3a. 'ubah 1 10' tidak berhasil\n       cart={cart_after}\n       replies: {replies[:1]}"); failed += 1

    # 3b. Cek keranjang setelah ubah
    replies = send("keranjang")
    cart_check = get_cart()
    if cart_check and replies_contain(replies, "keranjang"):
        qty = cart_check[0].get("quantity", 0)
        ok(f"3b. Keranjang setelah ubah: item[0] qty={qty}"); passed += 1
    else:
        fail(f"3b. Keranjang setelah ubah tidak valid\n       replies: {replies[:1]}"); failed += 1

    print(f"\n  {BOLD}Skenario 3: {passed} passed, {failed} failed{RESET}")
    return failed == 0

# ══════════════════════════════════════════════════════════════════════════════
# Skenario 4: Tambah beberapa produk sekaligus
# ══════════════════════════════════════════════════════════════════════════════
def scenario_4():
    sep("SKENARIO 4: Tambah beberapa produk ke cart")
    clear_cart()
    passed = failed = 0

    # 4a. Search dan tambah produk pertama
    send("6205 2RS")
    replies = send("1x5")   # langsung pilih #1 qty 5
    cart = get_cart()
    has_5 = cart and cart[0].get("quantity") == 5
    if has_5 or replies_contain(replies, "ditambahkan", "keranjang", "berhasil"):
        ok(f"4a. '1x5' → produk + qty 5 ditambahkan"); passed += 1
    else:
        # Coba send qty separately
        replies2 = send("5")
        cart = get_cart()
        if cart or replies_contain(replies2, "ditambahkan", "keranjang"):
            ok(f"4a. '1' lalu '5' → produk ditambahkan"); passed += 1
        else:
            fail(f"4a. Tambah produk pertama gagal\n       cart={cart}\n       replies: {replies[:1]}"); failed += 1

    time.sleep(0.5)
    cart_count_before = len(get_cart())

    # 4b. Cari dan tambah produk kedua
    send("6301 2RS")
    time.sleep(0.3)
    replies = send("1x2")   # qty 2
    time.sleep(0.5)
    cart_after = get_cart()
    if len(cart_after) > cart_count_before or replies_contain(replies, "ditambahkan", "keranjang", "berhasil"):
        ok(f"4b. Produk kedua ditambahkan ({len(cart_after)} item di cart)"); passed += 1
    else:
        # Try sending qty separately
        replies2 = send("2")
        time.sleep(0.5)
        cart_after = get_cart()
        if len(cart_after) > cart_count_before:
            ok(f"4b. Produk kedua ditambahkan ({len(cart_after)} item di cart)"); passed += 1
        else:
            fail(f"4b. Produk kedua gagal ditambahkan\n       cart={cart_after}\n       replies: {replies[:1]}"); failed += 1

    # 4c. Lihat keranjang akhir
    replies = send("keranjang")
    cart_final = get_cart()
    if cart_final and replies_contain(replies, "keranjang"):
        total = sum(i.get("quantity",0)*i.get("harga",0) for i in cart_final)
        ok(f"4c. Keranjang final: {len(cart_final)} produk, total Rp {total:,.0f}"); passed += 1
        for item in cart_final:
            info(f"    {item.get('kode')} x{item.get('quantity')} @ Rp {item.get('harga',0):,.0f}")
    else:
        fail(f"4c. Keranjang tidak valid\n       cart={cart_final}"); failed += 1

    print(f"\n  {BOLD}Skenario 4: {passed} passed, {failed} failed{RESET}")
    return failed == 0

# ══════════════════════════════════════════════════════════════════════════════
# Skenario 5: Hapus item dari cart
# ══════════════════════════════════════════════════════════════════════════════
def scenario_5():
    sep("SKENARIO 5: Hapus item dari cart")
    passed = failed = 0

    cart_before = get_cart()
    if not cart_before:
        # Setup cart dulu
        clear_cart()
        send("6205 2RS"); time.sleep(0.3)
        send("1"); time.sleep(0.3)
        send("3"); time.sleep(0.5)
        cart_before = get_cart()

    if not cart_before:
        fail("5.setup. Cart kosong, tidak bisa test hapus"); return False
    info(f"Cart sebelum hapus: {len(cart_before)} item(s)")

    # 5a. Hapus item pertama
    replies = send("hapus 1")
    cart_after = get_cart()
    item_removed = len(cart_after) < len(cart_before)
    if item_removed or replies_contain(replies, "dihapus", "berhasil", "hapus"):
        ok(f"5a. 'hapus 1' → item dihapus ({len(cart_before)}→{len(cart_after)})"); passed += 1
    else:
        fail(f"5a. 'hapus 1' tidak berhasil\n       cart={cart_after}\n       replies: {replies[:1]}"); failed += 1

    # 5b. Keranjang setelah hapus
    replies = send("keranjang")
    if replies_contain(replies, "keranjang"):
        ok(f"5b. Keranjang valid setelah hapus ({len(cart_after)} item)"); passed += 1
    else:
        fail(f"5b. Reply keranjang tidak valid\n       replies: {replies[:1]}"); failed += 1

    print(f"\n  {BOLD}Skenario 5: {passed} passed, {failed} failed{RESET}")
    return failed == 0

# ══════════════════════════════════════════════════════════════════════════════
# main
# ══════════════════════════════════════════════════════════════════════════════
if __name__ == "__main__":
    args = sys.argv[1:]

    if "--setup" in args:
        setup_phone()
        sys.exit(0)

    # Tentukan skenario yang mau dirun
    run_only = None
    if "-s" in args:
        idx = args.index("-s")
        if idx + 1 < len(args):
            run_only = args[idx + 1]

    print(f"\n{BOLD}═══ BOT FLOW TEST — phone: {TEST_PHONE} ═══{RESET}")
    print(f"  endpoint: {GATEWAY_URL}")
    setup_phone()   # ensure registered

    results = {}
    cart_s1 = []

    if not run_only or run_only == "1":
        ok_s1, cart_s1 = scenario_1()
        results["1: search→pilih→qty→cart"] = ok_s1

    if not run_only or run_only == "2":
        results["2: search kendaraan→Flowise"] = scenario_2()

    if not run_only or run_only == "3":
        results["3: rubah qty"] = scenario_3(cart_s1)

    if not run_only or run_only == "4":
        results["4: tambah banyak produk"] = scenario_4()

    if not run_only or run_only == "5":
        results["5: hapus cart"] = scenario_5()

    # Summary
    print(f"\n{BOLD}{'═'*55}{RESET}")
    print(f"{BOLD}  HASIL AKHIR:{RESET}")
    all_ok = True
    for name, ok_val in results.items():
        status = f"{GREEN}PASS{RESET}" if ok_val else f"{RED}FAIL{RESET}"
        print(f"  [{status}] {name}")
        all_ok = all_ok and ok_val
    print(f"{BOLD}{'═'*55}{RESET}\n")

    if all_ok:
        print(f"{GREEN}{BOLD}✓ Semua skenario PASSED{RESET}\n")
    else:
        print(f"{RED}{BOLD}✗ Ada skenario yang FAILED{RESET}\n")
        sys.exit(1)
