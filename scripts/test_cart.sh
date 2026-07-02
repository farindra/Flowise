#!/bin/bash
# Test script untuk cart tools Trade CS agentflow
# Usage: bash scripts/test_cart.sh [session_id]

FLOWISE_URL="http://127.0.0.1:3000"
CHATFLOW_ID="8f54eb54-8fcc-435d-ab64-6ae53ebdcb89"
FLOWISE_KEY="gpMebq4hbHBJPIKE_nk13m3CAn7h4nAyrntyTmLuzZE"
SESSION_ID="${1:-test-session-$(date +%s)}"
export PGPASSWORD=xfydBwYFRTSZT6ia
PSQL="/www/server/pgsql/bin/psql -h 127.0.0.1 -U flowise -d flowise"

echo "======================================"
echo "CART TOOLS TEST"
echo "Session ID: $SESSION_ID"
echo "======================================"

# 1. Cek DB connection & cart_items table
echo ""
echo "[1] Cek tabel cart_items..."
$PSQL -t -c "SELECT COUNT(*) || ' total items in cart_items' FROM cart_items;" 2>&1

# 2. Insert test item langsung ke DB
echo ""
echo "[2] Insert test item ke cart (session: $SESSION_ID)..."
$PSQL -t -c "
INSERT INTO cart_items (session_id, kode, nama, brand, harga, qty, updated_at)
VALUES ('$SESSION_ID', 'TEST001', 'SKF 6205 2RS', 'SKF', 60340, 2, NOW())
ON CONFLICT (session_id, kode) DO UPDATE SET qty=2, updated_at=NOW();
SELECT kode, nama, qty, harga FROM cart_items WHERE session_id='$SESSION_ID';
" 2>&1

# 3. Test Flowise API - lihat keranjang
echo ""
echo "[3] Test chat: 'lihat keranjang' (session: $SESSION_ID)..."
RESPONSE=$(curl -s -X POST "$FLOWISE_URL/api/v1/internal-prediction/$CHATFLOW_ID" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $FLOWISE_KEY" \
  -d "{\"question\":\"lihat keranjang\",\"sessionId\":\"$SESSION_ID\"}" 2>&1)

echo "Response:"
echo "$RESPONSE" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    print('text:', d.get('text','(no text)'))
    tools = d.get('usedTools', [])
    if tools:
        print('tools called:', [t.get('tool') for t in tools])
    else:
        print('tools called: NONE - bot tidak memanggil tool!')
except:
    print(sys.stdin.read()[:500])
" 2>&1

# 4. Test Flowise API - tambah produk
echo ""
echo "[4] Test chat: 'tambah 6205 SKF 3 pcs'..."
RESPONSE2=$(curl -s -X POST "$FLOWISE_URL/api/v1/internal-prediction/$CHATFLOW_ID" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $FLOWISE_KEY" \
  -d "{\"question\":\"tambah 6205 SKF 3 pcs ke keranjang\",\"sessionId\":\"$SESSION_ID\"}" 2>&1)

echo "Response:"
echo "$RESPONSE2" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    print('text:', d.get('text','(no text)')[:300])
    tools = d.get('usedTools', [])
    if tools:
        print('tools called:', [t.get('tool') for t in tools])
    else:
        print('tools called: NONE')
except:
    print(sys.stdin.read()[:500])
" 2>&1

# 5. Cek isi cart di DB setelah test
echo ""
echo "[5] Isi cart_items setelah test..."
$PSQL -t -c "SELECT kode, nama, qty, harga FROM cart_items WHERE session_id='$SESSION_ID';" 2>&1

# 6. Cleanup test session
echo ""
echo "[6] Cleanup test session..."
$PSQL -t -c "DELETE FROM cart_items WHERE session_id='$SESSION_ID'; SELECT 'Cleaned up' AS status;" 2>&1

echo ""
echo "======================================"
echo "TEST SELESAI"
echo "======================================"
