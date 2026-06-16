# Cutover Runbook — Node Bot → Go Bot

Cutover memindahkan nomor WhatsApp produksi dari Node bot (whatsapp-web.js, PM2)
ke Go bot (whatsmeow, Docker). Lakukan cutover saat traffic rendah (malam/subuh).

---

## Pre-Cutover Checklist

- [ ] Go bot sudah jalan di nomor testing dan semua 40 skenario di `PARITY-CHECKLIST.md` lulus
- [ ] `MARKETING_NOTIFY_OVERRIDE` di `wa-gateway-service/.env` sudah dikosongkan (agar notif ke Celvin/Puput asli)
- [ ] Backup MongoDB Atlas sudah dikonfirmasi ada (Atlas automatic backup)
- [ ] Semua service Go sudah `docker compose up -d` dan healthy:
  - `ob-wa-gateway` ✅
  - `ob-ai-vision` ✅
  - `ob-product-search` ✅
  - `ob-customer-pricing` ✅
  - `ob-meilisearch` ✅

---

## Step 1 — Migrate User State

Jalankan sekali sebelum cutover untuk copy company/region dari MongoDB ke sqlite Go:

```bash
cd /opt/oceanbearings/ob-bot/wa-gateway-service

MONGODB_URI="mongodb+srv://suksesbearindo_db_user:xkXFl5wxALhO8XRG@cluster0.dlgrnty.mongodb.net/?retryWrites=true&w=majority&appName=Cluster0" \
MONGODB_DB_NAME=ocean_bearings \
MONGODB_COLLECTION_USERS=users \
DATA_DIR=/opt/oceanbearings/ob-bot/data/wa-gateway \
go run ./cmd/migrate
```

Output yang diharapkan:
```
Connected to MongoDB ocean_bearings/users
migrated 6281xxxxxxxxx (2 keys)
migrated 6285xxxxxxxxx (3 keys)
...
Done — migrated=N skipped=M failed=0
```

> **Catatan:** Tool ini idempoten — tidak menimpa data yang sudah ada di sqlite.
> Aman dijalankan ulang.

---

## Step 2 — Stop Node Bot

```bash
pm2 stop bobi-whatsapp-bot
pm2 save
```

Verifikasi Node bot berhenti:
```bash
pm2 list
```
Status `bobi-whatsapp-bot` harus `stopped`.

> **Penting:** Setelah ini nomor produksi bebas untuk di-pair ulang.
> WA session Node bot (whatsapp-web.js) otomatis terminated saat proses mati.

---

## Step 3 — Pair Go Bot dengan Nomor Produksi

Ada dua cara pairing di whatsmeow:

### Opsi A: Phone Number Pairing (Direkomendasikan)

Akses endpoint `/pair-phone` dari Go bot. Format nomor: `628xxx` (tanpa + atau 0 depan).

```bash
# Ganti TOKEN dengan QR_TOKEN dari docker logs ob-wa-gateway
# Ganti NOMOR dengan nomor produksi format 628xxx

curl "http://localhost:8100/pair-phone?token=TOKEN&phone=NOMOR"
```

Response:
```json
{"pairing_code": "XXXX-XXXX"}
```

Masukkan `pairing_code` di WA: Settings → Linked Devices → Link a Device → Link with phone number.

### Opsi B: QR Code

1. Akses `http://localhost:8100/qr?token=TOKEN` di browser
2. Scan QR dari WA: Settings → Linked Devices → Link a Device

> **Catatan:** QR_TOKEN ada di `docker logs ob-wa-gateway | head -5`.

---

## Step 4 — Verifikasi Go Bot Aktif

```bash
# Cek logs
docker logs ob-wa-gateway 2>&1 | tail -20

# Harusnya ada:
# [Client INFO] Successfully authenticated
# mongo: connected to MongoDB Atlas
```

Kirim pesan test dari HP ke nomor produksi:
```
6205
```
Harus dapat balasan hasil pencarian bearing.

---

## Step 5 — Run Parity Scenarios

Isi kolom "Manual WA Test Log" di `docs/PARITY-CHECKLIST.md` untuk tiap skenario S01–S40.
Minimum pass untuk go-live: semua S01–S16 (product search + caret) harus lulus.

---

## Step 6 — Disable Node Bot Autostart

Setelah Go bot stabil minimal 24 jam:

```bash
pm2 delete bobi-whatsapp-bot
pm2 save
```

Backup Node bot codebase dulu sebelum hapus:
```bash
tar -czf /opt/oceanbearings/node-bot-backup-$(date +%Y%m%d).tar.gz \
  /opt/oceanbearings/node/wa-bot/wa-chat-bot-ai-ocean-bearing/
```

---

## Rollback

Jika ada masalah setelah cutover:

1. Stop Go bot sementara (jangan disconnect WA):
   ```bash
   docker compose stop wa-gateway-service
   ```
2. Restart Node bot:
   ```bash
   pm2 restart bobi-whatsapp-bot
   ```
3. Node bot akan reconnect ke WA session yang sama (whatsapp-web.js reconnect otomatis).

> **Catatan:** Rollback hanya feasible dalam ~10 menit pertama. Setelah itu WA
> session Node mungkin sudah expired dan perlu re-scan QR.

---

## Pasca Cutover — Monitoring

```bash
# Watch logs real-time
docker logs -f ob-wa-gateway

# Check MongoDB sync
# Buka Atlas UI → ocean_bearings → chat_history
# Pastikan entry baru masuk setelah cutover

# Disk usage (sqlite + whatsmeow db)
du -sh /opt/oceanbearings/ob-bot/data/wa-gateway/

# Container health
docker compose ps
```

---

## Kontak Darurat

Kalau Go bot error dan rollback tidak berhasil:
- Celvin: 0812-8830-9688
- Puput: 0812-8298-3305

(Beritahu marketing bahwa bot sedang maintenance sementara.)
