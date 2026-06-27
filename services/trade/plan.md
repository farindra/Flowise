# Plan: Salesman AI Agent

## Tujuan
AgentFlow di Flowise untuk salesman — cek harga produk, buat quotation, buat sales order, email blast ke customer.

## Arsitektur

```
Salesman (Telegram bot)
    ↓
salesman-service (Go, port 8200) — Telegram webhook + Jurnal tools + customer sync
    ├── /tg-webhook          → forward ke Flowise AgentFlow
    ├── GET /products        → Meilisearch products (via product-search-service)
    ├── GET /customers       → Meilisearch customers index (sync dari Jurnal tiap 6 jam)
    ├── POST /quotation      → Jurnal API
    ├── POST /sales-order    → Jurnal API
    └── POST /sync-customers → manual trigger re-sync
    ↓
Flowise AgentFlow (SALESMAN_CHATFLOW_ID — belum dibuat)
```

## salesman-service

**Docker container:** `ob-salesman` (port 127.0.0.1:8200)
**Build:** `cd services && docker compose build salesman-service && docker compose up -d salesman-service`
**Source:** `services/trade/salesman-service/`
**Auth header:** `X-Internal-Key: ob-jurnal-internal-2026`
**Jurnal token:** hardcoded di `.env` — `HAGIoJmmkO3Tnt98THLIrAXmhtflbUnN`

**Endpoints (via nginx `/salesman/` prefix dari luar, atau langsung port 8200 dari dalam):**

| Method | Path | Keterangan |
|--------|------|------------|
| GET | /products?q=...&limit=... | Cari produk (via Meilisearch) |
| GET | /customers?q=...&limit=... | Cari customer (via Meilisearch) |
| GET | /customers?id=... | Detail customer (via Jurnal API) |
| POST | /quotation | Buat quotation ke Jurnal |
| POST | /sales-order | Buat sales order ke Jurnal |
| POST | /sync-customers | Re-sync customers Jurnal → Meilisearch |
| POST | /tg-webhook | Telegram webhook (public, tanpa auth) |
| GET | /health | Health check |

**Customer sync:** otomatis tiap 6 jam, 1181 customers sudah terindex

## Flowise AgentFlow

**Chatflow ID:** `2e7c8812-177b-46f7-95e3-b697e25e492f`
**Nama:** Salesman AI - Ocean Bearings

Custom tools yang dibuat:
| Tool ID | Nama |
|---------|------|
| ec8efa78-... | get_product_info |
| d1290c8e-... | search_customers |
| 1f169c3c-... | create_quotation |
| 1ea6d2d7-... | create_sales_order |

## Akses Salesman

**Telegram Bot:** `8927501389:AAGk0YbBZIbeHOIFVw2tH-lmkCt-34txgTg`
**Webhook:** `https://agentic.oceanbearings.co.id/tg-webhook` ✅ terdaftar

## Status

- [x] salesman-service Go — Docker container ob-salesman live port 8200
- [x] Tool: get_product_info — GET /products ✅ tested
- [x] Tool: search_customers — GET /customers (Meilisearch, 1181 customers) ✅ tested
- [x] Tool: create_quotation — endpoint ready (belum ditest end-to-end ke Jurnal)
- [x] Tool: create_sales_order — endpoint ready (belum ditest end-to-end ke Jurnal)
- [ ] Tool: send_email_blast — belum dibuat
- [x] Telegram webhook — async, terdaftar di bot
- [x] Flowise AgentFlow — ID 2e7c8812 ✅ LIVE
