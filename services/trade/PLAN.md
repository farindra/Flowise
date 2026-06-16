# OB-Bot — Migrasi WhatsApp Bot Ocean Bearings ke Go (Microservices + Docker)

> Status terakhir: **Phase 7 - Flowise integration (partial)** — Phase 1–6 selesai, Flowise client deployed, menunggu Flowise pindah ke VPS yang sama (2026-06-16)
> Update terakhir: 2026-06-16

## 1. Latar Belakang & Tujuan

Bot WhatsApp AI Ocean Bearings saat ini ada di
`/opt/oceanbearings/node/wa-bot/wa-chat-bot-ai-ocean-bearing/` (Node.js,
`whatsapp-web.js` + Puppeteer, logic bisnis utama di `src/handlers/messageHandler.js`
~5266 baris).

Tujuan migrasi:
- Pindahkan **semua** komponen ke **Go**, dijalankan sebagai **microservices**
  yang independen per fungsi (masing-masing punya `.env`, `go.mod`, dan
  container Docker sendiri).
- Ganti `whatsapp-web.js` (Puppeteer, berat) dengan **whatsmeow**
  (`go.mau.fi/whatsmeow`) — native WhatsApp multi-device protocol, lebih
  ringan, dan support multi-session (mengganti `multi-bot.js`/`multi-instance.js`).
- Ganti pencarian produk berbasis `string-similarity` dengan **Meilisearch**
  (typo-tolerant search, cocok untuk pola kode bearing seperti `6205-2RS`,
  `SKF 6205`, `6224.FAG`, dll).
- Semua service jalan di **Docker** dengan resource limit per container, data
  persisten via bind mount ke host.
- **Tidak mengubah behavior/business rules** yang sudah ada — ini PORTING,
  bukan redesign. Referensi behavior: `PANDUAN-PENGGUNAAN-BOT.md` (30 skenario
  test) di project Node lama.

## 2. Arsitektur

```
                         ┌─────────────────────┐
                         │   wa-gateway-service │  (whatsmeow, Go)
                         │   - koneksi WA       │
                         │   - conversation FSM │
                         │   - cart/checkout    │
                         │   - state (sqlite)   │
                         └──────────┬───────────┘
                                     │ HTTP (internal docker network)
        ┌────────────────────┬──────┴───────────┬──────────────────────┐
        │                     │                  │                      │
┌───────▼─────────┐  ┌────────▼─────────┐  ┌─────▼──────────┐   ┌──────▼───────┐
│ product-search-  │  │ customer-pricing-│  │ ai-vision-      │   │ sync-indexer │
│ service          │  │ service          │  │ service         │   │ (cron, Go)   │
│ (Meilisearch)    │  │ (Jurnal.id +     │  │ (Gemini OCR +   │   │ -> push data │
│                  │  │  island pricing) │  │  parsing)       │   │    ke Meili  │
└───────┬──────────┘  └──────────────────┘  └─────────────────┘   └──────┬───────┘
        │                                                                  │
        └───────────────────────► Meilisearch ◄───────────────────────────┘
```

Semua service di custom bridge network `ob-bot-net`. Hanya `wa-gateway-service`
yang punya "pintu keluar" (koneksi WhatsApp). Tidak ada port yang wajib
di-expose ke host kecuali untuk debugging/health-check sementara.

## 3. Folder Structure

```
/opt/oceanbearings/ob-bot/
├── PLAN.md                          # dokumen ini
├── MEMORY.md                        # context utk lanjut sesi Claude baru
├── docker-compose.yml
│
├── data/                            # BIND MOUNT - persisten, jangan dihapus
│   ├── wa-session/                  # whatsmeow sqlite store + cart/user state
│   └── meilisearch/                 # index Meilisearch
│
├── wa-gateway-service/              # INTI - pengganti whatsapp-web.js + messageHandler.js
│   ├── Dockerfile
│   ├── .env
│   ├── go.mod
│   ├── cmd/server/main.go
│   └── internal/
│       ├── waclient/                # whatsmeow: koneksi, pairing/QR, multi-session
│       ├── router/                  # routing pesan masuk
│       ├── conversation/            # greeting, FAQ (lokasi/harga/stok/PPN), natural chat
│       ├── search/                  # deteksi format kode, ^precision, multi-product
│       ├── cart/                    # add/update/remove cart
│       ├── checkout/                # region -> marketing -> konfirmasi -> notif
│       ├── registration/            # daftar customer baru
│       ├── notification/            # notif WA ke marketing
│       ├── state/                   # persistence (sqlite)
│       └── client/                  # HTTP client ke service lain
│
├── product-search-service/
│   ├── Dockerfile
│   ├── .env
│   ├── go.mod
│   └── internal/{codevariations,handler,meili,model,search}/
│
├── customer-pricing-service/
│   ├── Dockerfile
│   ├── .env
│   ├── go.mod
│   └── internal/{handler,jurnal,pricing}/
│
├── ai-vision-service/
│   ├── Dockerfile
│   ├── .env
│   ├── go.mod
│   └── internal/{handler,gemini}/
│
├── sync-indexer/
│   ├── Dockerfile
│   ├── .env
│   ├── go.mod
│   ├── cmd/indexer/main.go
│   └── internal/{codevariations,meili,sourceapi,transform}/
│
├── meilisearch/
│   └── .env                         # image resmi getmeili/meilisearch
│
└── docs/
    ├── API.md                       # kontrak antar service
    └── PARITY-CHECKLIST.md          # 30 skenario dari PANDUAN-PENGGUNAAN-BOT.md
```

## 4. Resource Limit Default (docker-compose, non-Swarm: `mem_limit`/`cpus`)

| Service | Mem limit | CPU limit | Catatan |
|---|---|---|---|
| `meilisearch` | 1g | 1.0 | titik awal, sesuaikan kalau index besar |
| `product-search-service` | 256m | 0.5 | API stateless |
| `customer-pricing-service` | 256m | 0.5 | API stateless + cache kecil |
| `ai-vision-service` | 256m | 0.5 | proxy ke Gemini |
| `sync-indexer` | 512m | 0.5 | jalan periodik via internal scheduler; dinaikkan dari 256m karena OOM saat bulk-decode ~37k produk (lihat MEMORY.md Catatan Teknis Phase 2) |
| `wa-gateway-service` | 512m | 1.0 | koneksi WA persisten + state cart/session |

**Total budget**: ~2.5GB RAM, ~4 CPU core. Server: 15GB RAM (9.5GB available),
disk **hanya 22GB free dari 193GB (89% used)** — perlu hati-hati ukuran image
Docker (pakai base alpine/distroless, pure-Go sqlite driver biar bisa static
binary).

## 5. Persistence Strategy

| Data | Lokasi (bind mount) | Kritis? | Backup |
|---|---|---|---|
| Sesi WA (whatsmeow sqlite) + cart/user state | `data/wa-session/*.db` | 🔴 sangat | `sqlite3 .backup` (atomic) -> tar harian ke `/opt/oceanbearings/backups/` |
| Index Meilisearch | `data/meilisearch/` | 🟡 sedang | Meilisearch dump API (`POST /dumps`) -> tar harian |
| Chat history | MongoDB Atlas (cloud, existing) | ✅ sudah aman | tidak perlu volume lokal, pakai connection string yang sama |
| `.env` per service | host filesystem, **JANGAN** di-bake ke image | 🔴 sangat | sudah persisten by design |

## 6. Phased Plan & Status

- [x] **Phase 0a** - Cek resource server (disk/mem/OS/port) — done, lihat MEMORY.md
- [x] **Phase 0b** - Buat folder structure + PLAN.md + MEMORY.md
- [x] **Phase 0c** - Install Docker Engine + Compose plugin — `docker.io` + `docker-compose-v2` dari apt Ubuntu 24.04. Versi: Docker 29.1.3, Compose 2.40.3
- [x] **Phase 0d** - Install Go toolchain — `golang-go` 1.22.2 dari apt
- [x] **Phase 0e** - Scaffold `docker-compose.yml` (network `ob-bot-net`, volume, resource limit, placeholder service) —
      6 service (meilisearch + 5 Go placeholder) berhasil **build & up**, semua
      `/health` merespon OK, resource limit aktif (cek via `docker stats`),
      bind mount `data/wa-session` & `data/meilisearch` jalan. Build cache
      di-prune (`docker builder prune -f`) untuk hemat disk (sisa ~21GB free).
      Verifikasi: `cd /opt/oceanbearings/ob-bot && docker compose ps`
- [x] **Phase 1** - 🟢 SPIKE whatsmeow — **GO**. `wa-gateway-service` minimal
      jalan: pairing via QR (PNG, endpoint `/qr?token=...`) atau via kode
      nomor HP (`/pair-phone?phone=...&token=...`), device tersambung
      (`+62 851-5737-9919`), terima pesan & balas "echo: <text>" — divalidasi
      end-to-end di Docker (`ob-wa-gateway`), sesi persist di
      `data/wa-session/whatsmeow.db` (reconnect tanpa scan ulang setelah
      restart). Multi-session: belum dites 2 device live, tapi terbukti aman
      secara desain — `waclient.New(ctx, dataDir)` di-parameter-kan per
      `dataDir`/sqlite file, jadi N sesi = N instance `Client` (atau N
      container) dengan `DATA_DIR` beda, tanpa shared state. Detail teknis
      & gotcha penting di MEMORY.md bagian "Catatan Teknis Phase 1".
- [x] **Phase 2** - Foundation services — SEMUA SELESAI (2026-06-15):
  - [x] `sync-indexer` + Meilisearch — DONE (2026-06-15)
        (port `syncProducts` (merge primary+secondary by `kode||name`, sum
        `quantity` utk duplikat, safety check `< existingCount*0.5`) dari
        `local-sync-system.js`, dan field mapping/brand-extraction dari
        `saveProducts` + `createCodeVariations`/`normalizeProductCode` dari
        `local-data-manager.js`. Index `products` di Meilisearch (PK `id`),
        36338 dokumen ter-index dari 36589+745 produk sumber. Sync jalan saat
        startup + tiap `CACHE_SYNC_INTERVAL` (30 menit). Field `keterangan`/
        `keterangan_words` **tidak diporting** — lihat poin 7 di bawah)
  - [x] `product-search-service` — DONE (2026-06-15)
        (endpoint `GET /search?q=...&limit=10`: generate `code_variations`
        dari query via `createCodeVariations` (duplikat dari sync-indexer),
        Phase A exact-match filter `code_variations = "..."` AND `stok > 0`,
        Phase B fallback full-text Meilisearch `stok > 0`, dedupe by `kode`,
        response reconstruct shape asli `this.products` — `kode, nama, stok,
        brand, harga*, _hargaNum{normal,customer,nonCustomer,cash}, active,
        available, lastUpdated`, tanpa `keterangan`)
  - [x] `customer-pricing-service` — DONE (2026-06-14)
        (port `src/services/customerService.js` + `src/utils/islandPricing.json`
        + `src/utils/provinceMapping.js` + VIP logic dari `messageHandler.js`
        + customer sync dari `local-sync-system.js`)
  - [x] `ai-vision-service` — DONE (2026-06-14)
        (port `src/services/aiService.js`: `analyzeImage` + `parseMultiProductWithAI`
        + manual fallback dari `productService.js` `parseMultiProductInput`/
        `isMultiProductSearch`. Prompt dipindah **verbatim** dari inline template
        literal di `aiService.js` ~baris 212-286 & 1298-1327 — bukan dari
        `enhanced-bearing-prompt.js`, yang ternyata dead code/tidak pernah dipanggil)
- [x] **Phase 3** - 🟡 Porting `messageHandler.js` (5266 baris) ke
      `wa-gateway-service/internal/`, modul per modul:
      - [x] **3a** - `internal/state` (Store + CustomerCache), `internal/client`
            (ProductSearch + CustomerPricing + AIVision), `internal/shared`
            (format + detect + context + brands), extend `ai-vision-service`
            `/analyze-message`. Build clean, all tests pass, docker build OK.
      - [x] **3b** - `internal/router` (throttle/queue, handleSingleMessage,
            handleCommand, handleTextMessage, handleGeneralMessage skeleton,
            /status + /reset + /help commands). `waclient.SetEventHandler` wired
            in `main.go`. Build + tests clean, container up & authenticated.
      - [x] **3c** - `internal/notification` (marketingMap, notifyMarketing,
            notifyMarketingInternal, handleMarketingCommand full, getAllMarketingNumbers,
            isMarketingNumber wired into router.marketingNumbers). Build + tests
            clean, container up & authenticated.
      - [x] **3d** - `internal/router/registration.go` (startRegistration, completeRegistration).
            `CustomerCache.GetCustomerByCompany` added. general.go registration
            stub replaced with real call. Build + tests clean, container up.
      - [x] **3e** - `internal/router/conversation.go` (handleGreeting, handleNegation/
            ContinueResponse, handleBotIdentityQuestion, handleBantuanRequest,
            handleMarketingSelectionForBantuan, handleWarehouseSearchRequest,
            handleMarketingSelectionForWarehouse, handlePriceQuestion,
            handleStockQuestion, handleLocationQuestion, handleMarketingQuestion,
            handlePPNQuestion). All stubs in general.go + handle.go wired to
            real handlers. WarehouseSearchRequested field added to ActiveConv.
            Build + tests clean, container up.
      - [x] **3f** - `internal/router/cart.go` (makeCartSummary, handleAddToCart,
            handleUpdateCartQuantity, handleBackToSearch, handleRemoveFromCart,
            handleCartCommand full). Stubs in general.go wired with index
            parsing. Build + tests clean, container up.
      - [x] **3g** - `internal/router/search.go` (handleProductSearch, handleMedia,
            formatSearchResults, number selection, hashtag order). Build + deploy 2026-06-16.
      - [x] **3h** - `internal/router/checkout.go` (full checkout state machine,
            NotifyInternal to marketing). Build + deploy 2026-06-16.
      - [x] **3i** - Integration: all stubs wired, isValidProductQuery & isProductSearch
            for registered users fixed, generateNatural AI endpoint added.
            `docs/PARITY-CHECKLIST.md` written (40 scenarios). 2026-06-16.
- [x] **Phase 4** - Data layer: `internal/mongo/client.go` — batch sync to Atlas
      (chat_history + users). `MongoSyncer` interface wired into state.Store.
      MONGODB_URI filled in .env. Verified: "mongo: connected to MongoDB Atlas". 2026-06-16.
- [x] **Phase 5** - Paritas audit: 40 skenario di `docs/PARITY-CHECKLIST.md`.
      1 gap ditemukan dan diperbaiki (isProductSearch untuk registered users).
      4 minor differences terdokumentasi (semuanya low-impact). 2026-06-16.
- [x] **Phase 6** - Cutover prep:
      - [x] Migration tool: `cmd/migrate/main.go` (copy MongoDB users → sqlite)
      - [x] Runbook: `docs/CUTOVER-RUNBOOK.md`
      - [x] Run migration tool — 316 migrated, 67 skipped, 0 failed (2026-06-16)
      - [ ] Stop Node bot + pair Go bot ke nomor produksi — **DITUNDA**
            (saat ini Go bot tetap di nomor test, Node bot tetap jalan di produksi)
- [ ] **Phase 7** - Flowise integration:
      - Flowise runs on separate VPS (akan dipindah ke VPS yang sama nanti)
      - Gantiin Gemini `generateNatural` (greeting + free-form chat) dengan Flowise
      - Semua business logic (product search, checkout, cart) tetap di Go bot
      - Flowise hanya handle: free-form conversation response
      - Arsitektur: Go bot → `POST {FLOWISE_URL}/api/v1/prediction/{chatflowId}`
        body: `{"question": "...", "history": [...]}` → response `{"text": "..."}`
      - [x] Buat `internal/client/flowise.go` — `FlowiseClient` dengan fallback ke Gemini
      - [x] Tambah `flowise *client.FlowiseClient` ke Router + helper `generateNatural()`
      - [x] Wire ke `handleGreeting`, `handleGeneralMessage` fallback, `startRegistration`
      - [x] Add `FLOWISE_URL`, `FLOWISE_CHATFLOW_ID`, `FLOWISE_API_KEY` ke `.env`
      - [x] Build + deploy (2026-06-16) — log: "Flowise not configured — natural chat via Gemini"
      - [ ] Isi `FLOWISE_URL` + `FLOWISE_CHATFLOW_ID` setelah Flowise pindah ke VPS yang sama
      - [ ] Test end-to-end: kirim "halo" → balasan dari Flowise

## 7. Risiko & Keputusan Penting

1. whatsmeow pairing ≠ whatsapp-web.js QR flow — script `vps-login.js`,
   `generate-qr.sh`, `login-manual.md` perlu ditulis ulang utk whatsmeow.
2. Prompt Gemini di `enhanced-bearing-prompt.js` & `aiService.js` sudah
   ditune (success rate 91%/100% di dokumen) — **pindah verbatim**.
3. Tidak ada test suite otomatis di project Node lama (`npm test` cuma stub)
   — paritas divalidasi manual via 30 skenario PANDUAN. Disiplin testing
   manual per modul saat Phase 3.
4. State cart/session: in-memory per `phoneNumber` di Node lama -> di Go
   disimpan di sqlite lokal (`wa-gateway-service/internal/state/`), tanpa
   nambah service baru.
5. SQLite driver: pakai **pure-Go** (`modernc.org/sqlite`) bukan `go-sqlite3`
   (cgo) agar bisa static binary + base image kecil (distroless/alpine).
6. **Jurnal.id API query string harus di-percent-encode di Go** (beda dari
   Node) — lihat MEMORY.md bagian Catatan Teknis Phase 2 utk detail. Berlaku
   utk semua call ke `api.jurnal.id` (termasuk `FetchCustomerProfile` kalau
   nanti ditambah query param JSON serupa).
7. **Field `keterangan`/`keterangan_words` tidak diporting ke Meilisearch
   index maupun response `product-search-service`.** `saveProducts`
   (`local-data-manager.js` ~baris 290-433) TIDAK pernah menghasilkan field
   `keterangan` — field itu hanya ada di `local_data/products.json` hasil
   import CSV satu-kali (`csv_to_json_converter.js`), tidak pernah diisi ulang
   oleh sync API biasa. Jadi `keterangan_words` (dari `buildSearchIndexes`)
   selalu kosong utk produk hasil sync. Keputusan: exclude kedua field ini
   dari schema, didokumentasikan di `sync-indexer/internal/transform/transform.go`
   dan `product-search-service/internal/model/product.go`.
8. **Bulk JSON decode ~37k produk: jangan pakai `map[string]interface{}`** —
   `sync-indexer` awalnya OOM (killed, cgroup limit 256m) saat decode array
   produk ke `map[string]interface{}` per item (~70 keys, overhead besar per
   map). Fix: typed struct (`RawProduct`) dengan hanya ~13 field yang dipakai
   (custom `flexString` unmarshal utk field yang kadang string/number/bool).
   Tetap perlu `mem_limit: 512m` (peak ~348MB Alloc saat fetch+merge+transform
   sekaligus utk ~37k produk) — relevan kalau ada service lain yang bulk-decode
   response API besar.

## 8. Referensi Source (Node lama, JANGAN diedit — hanya dibaca utk porting)

- `/opt/oceanbearings/node/wa-bot/wa-chat-bot-ai-ocean-bearing/src/handlers/messageHandler.js`
- `/opt/oceanbearings/node/wa-bot/wa-chat-bot-ai-ocean-bearing/src/services/{productService,customerService,aiService,mongoService,whatsappService}.js`
- `/opt/oceanbearings/node/wa-bot/wa-chat-bot-ai-ocean-bearing/src/utils/islandPricing.json`
- `/opt/oceanbearings/node/wa-bot/wa-chat-bot-ai-ocean-bearing/enhanced-bearing-prompt.js`
- `/opt/oceanbearings/node/wa-bot/wa-chat-bot-ai-ocean-bearing/PANDUAN-PENGGUNAAN-BOT.md` (skenario uji paritas)
- `/opt/oceanbearings/node/wa-bot/wa-chat-bot-ai-ocean-bearing/.env` (daftar semua env var yang perlu dipecah per service)
