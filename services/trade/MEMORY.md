# MEMORY — Context untuk sesi Claude baru

> Baca ini dulu sebelum lanjut kerja di project `ob-bot`. Untuk detail
> arsitektur/plan lengkap, lihat [PLAN.md](./PLAN.md).

## Apa project ini

Migrasi total bot WhatsApp Ocean Bearings dari Node.js
(`/opt/oceanbearings/node/wa-bot/wa-chat-bot-ai-ocean-bearing/`) ke **Go**,
dipecah jadi microservices independen, jalan di **Docker**, search pakai
**Meilisearch**, koneksi WA pakai **whatsmeow** (pengganti whatsapp-web.js).

User eksplisit bilang: **"ga mau refactor"** logic bisnis — ini PORTING 1:1
behavior, bukan redesign. Source kebenaran behavior ada di
`PANDUAN-PENGGUNAAN-BOT.md` (30 skenario test) di project Node lama.

## Fakta Server (jangan ulang cek kalau belum > beberapa hari)

- OS: Ubuntu 24.04.3 LTS, user `root`, sudo ok (passwordless)
- RAM: 15GB total, ~9.5GB available saat dicek
- **Disk: HANYA 22GB free dari 193GB (89% used)** — ini ketat. Pakai base
  image kecil (alpine/distroless), pure-Go sqlite (`modernc.org/sqlite`,
  bukan cgo `go-sqlite3`), bersihkan image/build cache Docker secara rutin.
- Port yang SUDAH dipakai (jangan konflik):
  - `127.0.0.1:8000` → trade app Python/uvicorn (`/opt/oceanbearings/trade`)
  - `:3000` & `:3001` → bot Node lama (`wa-chat-bot-ai-ocean-bearing`, app + health check)
- Port rencana untuk ob-bot: `7700` (meilisearch), `8100` (wa-gateway),
  `8101` (product-search), `8102` (customer-pricing), `8103` (ai-vision) —
  semua di internal docker network `ob-bot-net`, tidak wajib expose ke host.
- Docker (29.1.3) + Compose plugin (2.40.3) + Go (1.22.2) **sudah terinstall**
  sejak 2026-06-13 via apt.
- ⚠️ **JANGAN curl/akses `http://127.0.0.1:3001/health`** (health check bot
  Node lama) — endpoint itu punya bug pre-existing (circular JSON di
  `app.js:183`, `getHealthStatus()` -> `Client.options.authStrategy.client`)
  yang men-trigger `uncaughtException` -> `process.exit()` -> PM2 restart
  (terjadi 2x saat sesi 2026-06-13 karena testing port). Bot auto-recover
  tanpa scan ulang QR, tapi tetap downtime ~6-14s tiap kali. Tidak terkait
  ob-bot, jangan dipancing lagi.

## Struktur Project

```
/opt/oceanbearings/ob-bot/
├── PLAN.md              <- rencana lengkap + checklist phase, UPDATE INI setiap selesai langkah
├── MEMORY.md            <- file ini
├── docker-compose.yml
├── data/wa-session/     <- BIND MOUNT, sqlite whatsmeow + cart state. JANGAN HAPUS, ini = sesi WA login
├── data/meilisearch/    <- BIND MOUNT, index produk
├── wa-gateway-service/  <- INTI: whatsmeow + porting messageHandler.js (5266 baris)
├── product-search-service/
├── customer-pricing-service/
├── ai-vision-service/
├── sync-indexer/
├── meilisearch/
└── docs/
```

## Status Saat Ini

Lihat checklist "Phase" di [PLAN.md](./PLAN.md) bagian 6 — itu **source of
truth** progress, selalu update di sana setiap menyelesaikan satu item.

Terakhir diketahui (2026-06-14): **Phase 1 selesai — GO**. `wa-gateway-service`
jalan di Docker (`ob-wa-gateway`), terhubung ke WhatsApp nomor testing
**+62 851-5737-9919**, sesi persist di `data/wa-session/whatsmeow.db`. Pesan
masuk dibalas `echo: <text>` (sudah dites end-to-end di container). Siap
lanjut **Phase 2** (foundation services: product-search, customer-pricing,
ai-vision + sync-indexer).

**Phase 2 progress (2026-06-14): `ai-vision-service` selesai — GO**. Endpoint
`/analyze-image` (port `aiService.analyzeImage`: rate limit 5/min per nomor,
cache SHA-256 1 jam, concurrency limiter 5, retry 503/overloaded dengan
backoff 1s/2s/4s, `validateAndNormalizeBearingCodes` + `extractProductCodesFromText`
verbatim) dan `/parse-multi-product` (port `aiService.parseMultiProductWithAI`:
cache 1 jam by base64(text), rate limit 20/min, fallback manual ke
`parseMultiProductInput`/`isMultiProductSearch` dari `productService.js`) —
semua sudah dites end-to-end di Docker (`ob-ai-vision`, port 8103), termasuk
jalur AI (`method: "ai_parsing"`) dan jalur rate-limit (`429`). Unit test
manual-fallback di `internal/vision/multiproduct_test.go`. `.env` sudah diisi
`GEMINI_API_KEY`/`GEMINI_MODEL=gemini-3-flash-preview`/`GEMINI_BASE_URL=.../v1beta`
dari `.env` bot Node lama. Lanjut: `customer-pricing-service`.

**Phase 2 progress (2026-06-14): `customer-pricing-service` selesai — GO**.
Endpoint `/customer` (verifyCustomer + getCustomerDetailWithIsland, dgn
self-correcting island/province fallback), `/customer/by-company`,
`/customer/vip`, `/price` (VIP check -> registered island pricing ->
non-customer pricing, port `getCustomerPrice`). Pricing rules + 120-entry
province->island mapping (`internal/pricing/island.go`) di-port sebagai
ordered slice (bukan map) supaya urutan substring-match fallback sama
dengan JS `Object.entries`. VIP data (`customer_vip.json`, 82 entries)
di-seed dari `local_data/`, hot-reload tiap 30s (mirip chokidar). Customer
sync background dari Jurnal.id (paginate `/contacts` + province enrichment
per-customer) jalan tiap `CACHE_SYNC_INTERVAL` (30 menit), snapshot
`data/customer-pricing/customers.json` (1876 customers, atomic write).
Sudah dites end-to-end di Docker (`ob-customer-pricing`, port 8102): sync
pertama sukses 1876 customers dalam ~84s, semua endpoint (`/customer`,
`/customer/by-company`, `/customer/vip`, `/price` utk registered/VIP/
non-customer) return hasil yang benar. Unit test di
`internal/pricing/pricing_test.go`. Lanjut: `sync-indexer`.

**Phase 2 progress (2026-06-15): `sync-indexer` selesai — GO**. Background
service (tanpa endpoint HTTP, cuma `/health` placeholder) yang sync produk
dari `PRODUK_API_URL`(+`PRODUK_API_URL2`) ke index Meilisearch `products` (PK
`id`), jalan saat startup + tiap `CACHE_SYNC_INTERVAL` (30 menit). Port
`syncProducts` (merge by `kode||name`, sum `quantity` utk duplikat dari
endpoint kedua, safety check `existingCount>100 && len(merged) <
existingCount*0.5` pakai float64) + field mapping/brand-extraction dari
`saveProducts` + `createCodeVariations`/`normalizeProductCode` verbatim
(`internal/codevariations/`, ada unit test). Index settings: searchable
`[kode, code_variations, nama, brand]`, filterable `[stok, active, available,
code_variations]`. Dites end-to-end di Docker (`ob-sync-indexer`): sync
pertama sukses, 36589 produk primary + 745 secondary -> 36338 dokumen
ter-index (~13s), full-text search & exact filter di Meilisearch keduanya
berfungsi. `mem_limit` dinaikkan 256m->512m (lihat Catatan Teknis di bawah).
Field `keterangan`/`keterangan_words` **sengaja tidak diporting** (lihat
Catatan Teknis). Lanjut: `product-search-service`.

**Phase 2 progress (2026-06-15): `product-search-service` selesai — GO,
Phase 2 SEMUA SELESAI**. Endpoint `GET /search?q=...&limit=10` (POST JSON
body juga didukung): lowercase+trim query -> generate `code_variations` via
`createCodeVariations` (duplikat verbatim dari sync-indexer, ada unit test) ->
Phase A exact match (single Meilisearch query, filter `(code_variations =
"v1" OR code_variations = "v2" OR ...) AND stok > 0`) -> kalau hasil < limit,
Phase B fallback full-text search `stok > 0` -> dedupe by `kode`, truncate ke
`limit`. Response `{"products": [...]}`, tiap produk reconstruct shape asli
`this.products` (`kode, nama, stok, brand, hargaNormal/Customer/NonCustomer/
Cash, _hargaNum{normal,customer,nonCustomer,cash}, active, available,
lastUpdated` — camelCase, BUKAN snake_case seperti dokumen Meilisearch; tanpa
`keterangan`). Dites end-to-end di Docker (`ob-product-search`, port 8101):
`q=6205` (fuzzy fallback, 401 stok FAG di urutan pertama), `q=6205 zz
(korea).fag` (exact match urutan pertama), `q=6205-2rs` & `q=6205 2RS SKF`
(semua varian 2RS naik ke atas), `q=nonexistentcode123` -> `{"products":[]}`,
missing `q` -> 400. Memory ~3MB/256MB limit.

**Phase 2 SELESAI (4/4 foundation services)**: `ai-vision-service`,
`customer-pricing-service`, `sync-indexer`, `product-search-service` semua
jalan di Docker. Lanjut **Phase 3**: porting `messageHandler.js` (5266 baris)
ke `wa-gateway-service` — kerjaan terbesar di project ini.

### Catatan Teknis Phase 1 (whatsmeow) — penting utk lanjutan

1. **Pairing butuh ~beberapa kali coba** — QR di-render via `rsc.io/qr` jadi
   PNG (`/qr?token=...`), atau pakai pairing-code via nomor HP
   (`/pair-phone?phone=62...&token=...`, whatsmeow `PairPhone`). Endpoint ini
   ada di `cmd/server/main.go`, dilindungi `QR_TOKEN` (random per startup,
   dicetak di log) — **WAJIB pakai token ini** kalau bind address di-buka ke
   publik (jangan biarkan QR pairing tanpa token bisa diakses orang lain,
   resiko pairing-hijack).
2. **Sesi sempat ke-revoke sekali** (`401 logged out from another device` /
   `device_removed`) tepat setelah "Successfully paired" + "Got 515 code,
   reconnecting" — `whatsmeow.db` ikut terhapus (whatsmeow auto-delete on
   logout), device harus pairing ulang dari nol. Belum jelas root cause
   (kemungkinan WhatsApp anti-abuse karena banyak percobaan pairing beruntun).
   Kalau ini terjadi lagi: tinggal generate QR baru & scan ulang, bukan bug
   kode kita.
3. **SQLITE_BUSY saat history sync** — `sqlstore.New` connection string WAJIB
   pakai `?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)` (sudah di
   `internal/waclient/client.go`). Tanpa `busy_timeout`, banyak write
   concurrent (history sync, prekey, app state) gagal dgn `SQLITE_BUSY` dan
   bikin pesan masuk gagal di-decrypt + balasan echo gagal terkirim (no
   signal session established).
4. **File permission utk bind-mount sqlite + distroless `nonroot`** — kalau
   `whatsmeow.db` dibuat pertama kali oleh proses di host (root, mode 644),
   container `nonroot` (uid 65532) gak bisa nulis -> error
   `attempt to write a readonly database (8)`. Fix: `chmod 666` file db-nya
   (direktori `data/wa-session` sudah 777). Kalau mulai dari nol di dalam
   container, biasanya gak masalah karena file dibuat oleh `nonroot` sendiri.
5. **`SendMessage` sukses TIDAK menghasilkan log** — whatsmeow cuma log kalau
   error. Sudah ditambah log custom di `handleEvent`:
   `"pesan dari %s diterima, dibalas: %q"` supaya operator bisa lihat di
   `docker compose logs` kalau echo benar-benar terkirim.
6. **Multi-session**: belum ada tes 2 device live (butuh nomor HP ke-2), tapi
   `waclient.New(ctx, dataDir)` sudah diparameter-kan per `dataDir` (file
   sqlite terpisah) — N sesi = N `Client` instance / N container dengan
   `DATA_DIR` berbeda, tanpa shared state. Cukup aman dianggap proven by
   design untuk keputusan go/no-go Phase 1.
7. **Dockerfile** `wa-gateway-service` sudah diupdate: base image
   `golang:1.25-alpine` (go.mod butuh go 1.25.0 setelah `go get whatsmeow`),
   `COPY go.mod go.sum` + `go mod download` + `COPY internal`, tetap
   `CGO_ENABLED=0` + distroless (pure-Go sqlite `modernc.org/sqlite`).

### Catatan Teknis Phase 2 (customer-pricing-service) — penting utk lanjutan

1. **`api.jurnal.id` query string HARUS di-percent-encode di Go, beda dari
   Node!** Edge Jurnal.id (Alibaba ESA/Cloudflare-like WAF) balas `400 Bad
   Request` (body kosong) kalau query string `contact_index={"curr_page":1,...}`
   dikirim mentah (unencoded `{`, `"`, `:`) lewat Go `net/http` — TLS
   fingerprint/h1-vs-h2 BUKAN sebabnya (sudah dites dgn uTLS Chrome/Firefox/
   Safari fingerprint + paksa HTTP/1.1, tetap 400). Node `https` module kirim
   string sama persis tapi dapat `200` (jadi kode lama "raw RawQuery utk
   match JS" itu **salah/tidak perlu** — Node ternyata juga bisa terima
   encoded). Fix: pakai `url.Values{}.Encode()` (lihat
   `customer-pricing-service/internal/jurnal/client.go` `fetchSinglePage`) —
   hasil JSON identik, sudah dikonfirmasi return 1876 customers yg sama.
   **Berlaku jg utk `sync-indexer`** kalau `PRODUK_API_URL`/`PRODUK_API_URL2`
   atau endpoint Jurnal lain butuh query param JSON serupa — selalu pakai
   `url.Values.Encode()`, jangan raw `RawQuery` dgn karakter `{`/`"`/`:`.

### Catatan Teknis Phase 2 (sync-indexer / product-search-service) — penting utk lanjutan

1. **JANGAN decode response API besar (~37k items) ke
   `map[string]interface{}`** — `sync-indexer` awalnya OOM-killed (cgroup
   limit 256m, anon-rss ~260MB) saat decode array produk dari
   `PRODUK_API_URL` ke `[]map[string]interface{}` (~70 key per produk, Go map
   overhead besar per entry -> ~287MB utk 37334 produk). Fix: typed struct
   `RawProduct` (`sync-indexer/internal/transform/transform.go`) dengan hanya
   ~13 field yang dipakai, pakai custom type `flexString` (unmarshal JSON
   string/number/bool jadi string) buat jaga-jaga field API kadang bukan
   string. Setelah fix, peak Alloc full pipeline (fetch primary+secondary +
   merge + transform ke docs) ~348MB sekali jalan -> `mem_limit` dinaikkan
   256m->512m (steady-state cuma ~12-170MB via `docker stats`, ini one-time
   batch job tiap 30 menit, bukan steady cost). **Relevan utk service lain**
   kalau pernah perlu bulk-decode response API besar.
2. **Field `keterangan`/`keterangan_words` sengaja TIDAK diporting.** Trace:
   `saveProducts` (`local-data-manager.js` ~baris 290-433, lihat snippet
   `_hargaNum` ~baris 347-365) tidak pernah menulis field `keterangan` ke
   `this.products`. Field `keterangan` yang terlihat di
   `local_data/products.json` (38910 entries) berasal dari import CSV
   satu-kali (`csv_to_json_converter.js`), tidak pernah di-refresh oleh
   `syncProducts`/`saveProducts`. Jadi `keterangan_words` index
   (`buildSearchIndexes` ~baris 263-266, split on `/[^a-z0-9]+/`, len>2, minus
   `commonWords`) selalu kosong utk produk hasil sync API. Exclude dari
   Meilisearch document schema (`transform.go`) dan dari response
   `product-search-service` (`internal/model/product.go`) — keduanya punya
   komentar yang jelaskan ini.
3. **Field harga di response `product-search-service` pakai camelCase**
   (`hargaNormal`, `hargaCustomer`, `hargaNonCustomer`, `hargaCash`,
   `_hargaNum{normal,customer,nonCustomer,cash}`) — ini shape asli
   `this.products` entry di `local-data-manager.js` (dipakai langsung oleh
   `productService.js` via `product.hargaCustomer` dst). **Beda** dengan
   dokumen Meilisearch (`sync-indexer`'s `ProductDoc`) yang pakai snake_case +
   flattened (`harga_normal`, `harga_normal_num`, dst) — itu cuma skema index,
   bukan skema response API. `product-search-service` decode dokumen
   Meilisearch (`model.Doc`) lalu `ToProduct()` reconstruct ke shape camelCase
   ber-nested `_hargaNum`. Phase 3 (`wa-gateway-service`) harus konsumsi shape
   camelCase ini dari `/search`, bukan shape Meilisearch.
4. **Exact-match Phase A pakai 1 query Meilisearch dgn filter OR'd**, bukan N
   query terpisah per variation (`product-search-service/internal/meili/client.go`
   `SearchExact`) — sama hasilnya (semua variation di-cek), tapi 1 round-trip.
   Filter value di-quote/escape via `quoteFilterValue` (`\\` dan `"`).

## Hal Penting yang Jangan Dilupakan

1. **`wa-gateway-service` adalah pekerjaan terbesar** — bukan 4 service
   pendukung. Itu hasil porting `messageHandler.js` (5266 baris): cart,
   checkout, registrasi, FAQ, follow-up, dll.
2. **Phase 1 (spike whatsmeow) butuh interaksi user** — scan QR via HP untuk
   pairing. Tidak bisa diselesaikan otomatis tanpa user standby.
3. **Prompt Gemini** sudah ditune dengan success rate tinggi — sudah dipindah
   **verbatim** ke `ai-vision-service/internal/vision/prompt.go` (selesai).
   Catatan: prompt asli ada di **inline template literal di `aiService.js`**
   (~baris 212-286 utk `analyzeImage`, ~baris 1298-1327 utk
   `parseMultiProductWithAI`) — bukan di `enhanced-bearing-prompt.js`, yang
   ternyata dead code (tidak pernah diimport/dipanggil).
4. **Jangan edit project Node lama** (`/opt/oceanbearings/node/wa-bot/...`)
   — itu masih production & jadi referensi. `ob-bot/` adalah project baru
   yang sepenuhnya terpisah.
5. **`.env` per service** — JANGAN pernah commit/bake ke image Docker. Mount
   sebagai file/`env_file` saja.
6. Resource limit & port allocation sudah fix di PLAN.md bagian 4 — pakai
   itu sebagai default, jangan re-decide dari nol.

## Cara Lanjut

1. Baca [PLAN.md](./PLAN.md) bagian 6 (Phased Plan & Status) untuk lihat
   item checklist berikutnya yang belum `[x]`.
2. Kerjakan item itu.
3. Update checklist di PLAN.md (`[ ]` -> `[x]`) + update bagian "Status Saat
   Ini" di file ini dengan tanggal & ringkasan progres terbaru.
