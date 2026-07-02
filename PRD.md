# PRD — Al Azhar Memorial Garden: AI Agentic Platform

**Versi**: 1.0  
**Tanggal**: 27 Juni 2026  
**Platform**: Al Azhar Agentic (Flowise-based, `alazhar-agentic.farindra.com`)

---

## 1. Latar Belakang Bisnis

**Al Azhar Memorial Garden** adalah pemakaman muslim premium di Karawang yang dikelola oleh YPIA & PT Nuansa Usaha Mandiri. Beroperasi sejak 2011, telah melayani 7.000+ keluarga, dengan lahan 25 hektar dan 10 tipe kavling (Single hingga Royal Family).

**Tantangan operasional saat ini:**

-   Volume pertanyaan CS tinggi, repetitif (harga, lokasi, cara beli)
-   Proses penjualan kavling masih manual, bergantung pada tenaga salesman
-   Tidak ada sistem follow-up otomatis untuk prospek
-   Koordinasi admin, salesman, dan operasional masih via WhatsApp manual
-   Tidak ada sistem notifikasi terjadwal untuk perawatan/ziarah
-   Potensi IoT untuk monitoring kondisi lahan belum dimanfaatkan

---

## 2. Tujuan

Membangun ekosistem AI agentic yang mengotomasi komunikasi, penjualan, operasional, dan monitoring Al Azhar Memorial Garden — terintegrasi via WhatsApp, Telegram, dan web — menggunakan platform Flowise yang sudah berjalan.

---

## 3. Ruang Lingkup

### 3.1 Agent CS Bot (Customer Service)

**Channel**: WhatsApp, Website Chat  
**Target user**: Calon pembeli, keluarga yang sedang berduka, pengunjung

**Fungsi:**

-   Menjawab FAQ otomatis: tipe kavling, harga, lokasi, fasilitas, cara pembelian
-   Menerima dan mencatat pertanyaan masuk ke sistem CRM
-   Triage urgensi: membedakan keluarga yang sedang berduka (perlu respons cepat) vs. calon pembeli (nurturing)
-   Booking konsultasi dengan salesman (kirim jadwal otomatis)
-   Mengirim brosur digital, video profil, denah lokasi
-   Bahasa: Indonesia dengan nuansa santun/islami ("Assalamu'alaikum", dll.)

**Flowise Implementation:**

-   Agentflow dengan tools: RAG (knowledge base produk), Calendar Booking, CRM Write
-   Agentflow terpisah per channel (WA bot customer vs. web chat)
-   Handoff ke manusia jika: komplain serius, negosiasi harga, atau permintaan khusus

**KPI**: Response time < 30 detik, containment rate > 70%, CSAT > 4/5

---

### 3.2 Asisten Salesman

**Channel**: WhatsApp (bot owner/salesman), Web dashboard  
**Target user**: Tim sales internal AAMG

**Fungsi:**

-   Profil prospek otomatis: ringkasan percakapan + rekomendasi pendekatan
-   Notifikasi hot lead: alert jika prospek sudah tanya 3x tanpa closing
-   Draft pesan follow-up yang dipersonalisasi berdasarkan riwayat percakapan
-   Laporan pipeline harian: berapa prospek, di stage mana, estimasi closing
-   Simulasi kalkulasi harga + cicilan untuk prospek
-   Reminder follow-up terjadwal: D+1, D+3, D+7 setelah kontak pertama
-   Script handling keberatan (objection handling) berbasis konteks prospek

**Flowise Implementation:**

-   Agent dengan akses ke CRM database (read/write via HTTP Tool)
-   Scheduled Flow untuk reminder harian (jam 08.00 WIB)
-   Pipeline stage tracker dengan kondisi otomatis
-   Integrasi dengan Telegram Bot salesman

**KPI**: Conversion rate +20%, response time salesman ke prospek < 2 jam

---

### 3.3 Asisten Admin

**Channel**: Telegram Bot internal, Web dashboard  
**Target user**: Tim administrasi AAMG

**Fungsi:**

-   Pencatatan data pembeli baru (form intake otomatis dari percakapan WA)
-   Generate dokumen: surat perjanjian kavling, kuitansi, sertifikat lahan (template PDF)
-   Rekap transaksi harian/mingguan otomatis ke email/Telegram admin
-   Cek status pembayaran dan kirim pengingat cicilan otomatis ke pembeli
-   Pencatatan jadwal pemakaman darurat (masuk antrian otomatis)
-   Lookup data makam: nomor kavling, status, pemilik, lokasi GPS

**Flowise Implementation:**

-   Agent dengan tool: PostgreSQL query, PDF Generator, Email/Telegram sender
-   Scheduled Flow untuk rekap (setiap hari jam 17.00 dan Senin pagi)
-   Chatflow khusus "admin bot" di Telegram dengan autentikasi internal key

**KPI**: Waktu input data manual berkurang 60%, zero missed payment reminder

---

### 3.4 Asisten Marketing

**Channel**: Telegram internal marketing, Dashboard web  
**Target user**: Tim marketing AAMG

**Fungsi:**

-   Riset tren: monitoring keyword pencarian pemakaman muslim di sosial media
-   Draft konten sosmed otomatis: caption Instagram, naskah YouTube Shorts, artikel blog
-   Jadwal posting konten (content calendar) berdasarkan momen penting Islam (Ramadan, Muharram, dll.)
-   Laporan performa iklan (jika Google Ads/Meta Ads API tersedia)
-   Segmentasi audiensberdasarkan interaksi WA/web (untuk retargeting)
-   A/B test copy iklan: generate 3 variasi teks iklan untuk satu produk

**Flowise Implementation:**

-   Agent dengan tool: WebSearch (monitoring kompetitor), Text Generator, Calendar
-   Scheduled Flow untuk content calendar reminder
-   Output dikirim ke Telegram channel internal marketing

**KPI**: Produksi konten +3x per bulan, waktu pembuatan konten -50%

---

### 3.5 Otomasi IoT (Monitoring Lahan)

**Channel**: Dashboard web, alert Telegram  
**Target user**: Tim operasional/perawatan lahan

**Fungsi:**

-   Integrasi sensor IoT di lahan: kelembaban tanah, cuaca, kondisi taman
-   Alert otomatis jika anomali: tanah terlalu kering → notifikasi tim penyiram
-   Jadwal perawatan prediktif: berdasarkan data sensor + pola musim
-   Log kondisi lahan per zona (peta kavling + status terkini)
-   Kamera CCTV: alert gerakan mencurigakan di luar jam operasional
-   Laporan kondisi lahan mingguan ke manajemen

**Flowise Implementation:**

-   HTTP Tool menerima webhook dari sensor IoT (MQTT → REST bridge)
-   Condition nodes untuk threshold alert
-   Scheduled Flow untuk laporan mingguan
-   Output: Telegram alert + log ke database

**KPI**: Respons insiden lahan < 15 menit, perawatan preventif terjadwal 100%

---

### 3.6 Notification Scheduler

**Channel**: WhatsApp, Telegram, Email  
**Target user**: Pembeli kavling aktif, keluarga jenazah

**Fungsi:**

-   Pengingat ziarah: kirim pesan undangan ziarah bersama (event bulanan)
-   Reminder peringatan: notifikasi setiap tahun pada tanggal wafat keluarga
-   Ucapan Idul Fitri, Idul Adha, Muharram kepada seluruh database pembeli
-   Notifikasi pembaruan fasilitas atau event di AAMG
-   Pengingat cicilan kavling (D-7, D-3, H-0, H+3 jika terlambat)
-   Survei kepuasan setelah pemakaman berlangsung (H+14)

**Flowise Implementation:**

-   Scheduled Flow dengan cron expression (berbasis tanggal di database pembeli)
-   Integration dengan go-wa dan go-telegram service yang sudah ada
-   Database: tabel `notifications_queue` di PostgreSQL
-   Deduplikasi: jangan kirim 2x jika sudah terkirim

**KPI**: Open rate notifikasi > 85% (WA), kepuasan pelanggan jangka panjang

---

### 3.7 Lead Generation & Nurturing Pipeline

**Channel**: Website form, WhatsApp, Landing page  
**Target user**: Pengunjung website baru, prospek cold

**Fungsi:**

-   Chatbot lead capture di website: kumpulkan nama, kontak, budget range, urgensi
-   Lead scoring otomatis: urgensi tinggi (ada keluarga sakit) vs. investasi (beli dari jauh)
-   Drip campaign via WhatsApp: seri pesan edukatif selama 7 hari setelah pertama kontak
    -   Hari 1: Kenalan + profil AAMG
    -   Hari 3: Video testimoni keluarga
    -   Hari 5: Perbandingan tipe kavling
    -   Hari 7: Penawaran konsultasi gratis
-   Retargeting: prospek yang tidak respons setelah 30 hari → kirim pesan re-engagement
-   Routing otomatis ke salesman yang tepat berdasarkan lokasi asal prospek

**Flowise Implementation:**

-   Multi-turn conversation flow untuk lead capture
-   Scoring engine (kondisi berbasis field di CRM)
-   Scheduled Flow untuk drip sequence (trigger by event)
-   Round-robin assignment ke salesman

**KPI**: Lead-to-consultation rate > 25%, cost per qualified lead turun 30%

---

## 4. Arsitektur Teknis

```
┌─────────────────────────────────────────────────────┐
│                   CHANNEL LAYER                      │
│  WhatsApp (go-wa:8082)  │  Telegram (go-telegram:8081)  │
│  Web Chat (Flowise UI)  │  Email (SMTP)                  │
└────────────────────┬────────────────────────────────┘
                     │ webhook
┌────────────────────▼────────────────────────────────┐
│              FLOWISE CORE (port 3000)                │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────┐  │
│  │ CS Agent │ │ Sales    │ │ Admin    │ │Sched.  │  │
│  │ Chatflow │ │ Agent    │ │ Agent    │ │Flows   │  │
│  └──────────┘ └──────────┘ └──────────┘ └────────┘  │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐             │
│  │Marketing │ │ Lead Gen │ │ IoT      │             │
│  │ Agent    │ │ Pipeline │ │ Monitor  │             │
│  └──────────┘ └──────────┘ └──────────┘             │
└────────────────────┬────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────┐
│                DATA LAYER                            │
│  PostgreSQL (CRM, notifications, kavling data)       │
│  Vector DB (product knowledge, FAQ, objections)      │
│  Redis (session state, rate limiting)                │
└─────────────────────────────────────────────────────┘
```

### Mengapa Agentflow, bukan Chatflow

Semua agent di sini menggunakan **Agentflow**, bukan Chatflow biasa. Alasannya:

-   **Chatflow** = flow deterministik, urutan node dikunci di canvas. Cocok untuk dialog linear (wizard form, FAQ satu arah).
-   **Agentflow** = agent _reason_ dan _pilih tool sendiri_ berdasarkan konteks. Agent bisa panggil RAG, lalu CRM, lalu Calendar dalam satu turn — atau skip beberapa tool kalau tidak relevan.

Semua use case di sini membutuhkan keputusan dinamis multi-tool, sehingga Agentflow adalah satu-satunya pilihan yang tepat. Satu pengecualian: drip campaign sequence (trigger → tunggu → send) yang benar-benar linier menggunakan **Scheduled Flow** (bukan conversation agent).

### Stack per Agent

| Agent           | Flow Type                  | LLM           | Memory                  | Tools                          | Output         |
| --------------- | -------------------------- | ------------- | ----------------------- | ------------------------------ | -------------- |
| CS Bot          | Agentflow                  | Claude Haiku  | Buffer Window (10 turn) | RAG, HTTP CRM, Calendar        | WA/Web reply   |
| Sales Asisten   | Agentflow                  | Claude Sonnet | Summary Memory          | HTTP CRM, Calendar, Draft Tool | Telegram/WA    |
| Admin Bot       | Agentflow                  | Claude Haiku  | None (stateless)        | PostgreSQL, PDF Gen, Email/TG  | Telegram       |
| Marketing       | Agentflow                  | Claude Sonnet | None                    | WebSearch, Calendar, Text Gen  | Telegram       |
| IoT Monitor     | Agentflow                  | Claude Haiku  | None                    | HTTP Sensor API, Alert Sender  | Telegram Alert |
| Notif Scheduler | Scheduled Flow             | Claude Haiku  | None                    | WA/TG sender, DB query         | WA/TG/Email    |
| Lead Pipeline   | Agentflow + Scheduled Flow | Claude Sonnet | Buffer Window           | HTTP CRM, Scoring, WA sender   | WA drip        |

---

## 5. Knowledge Base Requirements

### Dokumen yang harus di-ingest ke Vector DB:

1. Brosur produk lengkap (10 tipe kavling, harga terbaru)
2. FAQ CS (pertanyaan umum + jawaban standar)
3. Prosedur pembelian kavling (step by step)
4. Prosedur pemakaman darurat (prosedur 24 jam)
5. Peraturan dan tata tertib lahan
6. Konten syariah: dalil tentang pemakaman muslim, hukum-hukum jenazah
7. Peta lahan dan denah kavling per zona
8. Testimoni dan media coverage
9. Script objection handling salesman
10. Profil tim dan kontak internal

---

## 6. Integrasi Eksternal yang Diperlukan

| Integrasi                  | Kegunaan                     | Prioritas      |
| -------------------------- | ---------------------------- | -------------- |
| PostgreSQL AAMG            | CRM, data kavling, pembeli   | P0 (wajib)     |
| go-wa service              | Kirim/terima WA              | P0 (sudah ada) |
| go-telegram service        | Notifikasi internal          | P0 (sudah ada) |
| SMTP / Nodemailer          | Email admin & reminder       | P1             |
| Google Calendar API        | Booking konsultasi           | P1             |
| PDF Generator              | Dokumen sertifikat, kuitansi | P1             |
| MQTT / IoT API             | Data sensor lahan            | P2             |
| Meta Ads API               | Laporan performa iklan       | P2             |
| Google Sheets              | Export laporan harian        | P2             |
| Midtrans / payment gateway | Cek status pembayaran        | P2             |

---

## 7. Roadmap Implementasi

### Phase 1 — Foundation (Minggu 1-2)

-   [ ] Setup knowledge base: ingest semua dokumen produk ke Vector DB
-   [ ] Build CS Bot chatflow (FAQ, triage, handoff)
-   [ ] Integrasi CS Bot ke go-wa (WA bot customer)
-   [ ] Setup database CRM minimal (tabel: leads, kavlings, buyers)

### Phase 2 — Sales & Admin (Minggu 3-4)

-   [ ] Build Sales Asisten chatflow + Telegram bot salesman
-   [ ] Build Admin Bot (form intake, rekap harian)
-   [ ] Lead scoring engine
-   [ ] Drip campaign D+1/D+3/D+5/D+7

### Phase 3 — Scheduler & Automation (Minggu 5-6)

-   [ ] Notification Scheduler (reminder cicilan, ziarah, anniversary)
-   [ ] Scheduled reporting (harian, mingguan)
-   [ ] Lead re-engagement flow (prospek 30 hari tidak respons)

### Phase 4 — Marketing & IoT (Minggu 7-8)

-   [ ] Marketing Asisten (content generator, riset)
-   [ ] IoT monitoring integration (jika hardware sudah tersedia)
-   [ ] Dashboard analytics sederhana

---

## 8. Risiko & Mitigasi

| Risiko                                     | Dampak              | Mitigasi                                                                     |
| ------------------------------------------ | ------------------- | ---------------------------------------------------------------------------- |
| Bot gagal handle kasus berduka sensitif    | Reputasi rusak      | Wajib handoff ke manusia untuk kasus duka, respons empati tertulis di prompt |
| Harga kavling berubah, bot kasih info lama | Komplain pembeli    | Knowledge base update pipeline + tanggal berlaku harga di setiap dokumen     |
| WA bot kena banned Meta karena spam        | Layanan mati        | Rate limiting ketat, opt-out mudah, konten relevan dan tidak promosi massal  |
| Data pembeli bocor                         | Legal & kepercayaan | Tidak simpan data sensitif di log bot, enkripsi DB, access control ketat     |
| Salesman tidak mau pakai bot               | Adopsi gagal        | Onboarding training, bot bantu (bukan ganti) salesman, tampilkan value nyata |

---

## 9. Metrik Keberhasilan (3 Bulan)

| Metrik                              | Baseline        | Target     |
| ----------------------------------- | --------------- | ---------- |
| Response time CS                    | ~4 jam (manual) | < 30 detik |
| Leads yang di-follow-up dalam 2 jam | ~40%            | > 90%      |
| Konversi lead → konsultasi          | ~15%            | > 25%      |
| Volume konten marketing/bulan       | 4 post          | 15+ post   |
| Waktu input data admin/hari         | ~3 jam          | < 45 menit |
| Kepuasan pelanggan (CSAT)           | Belum diukur    | > 4.2/5    |

---

## 10. Catatan Implementasi untuk Flowise

-   Setiap agent adalah **agentflow terpisah** di Flowise, tidak satu monolith
-   Gunakan **Upsert API** untuk sync knowledge base dari dokumen baru
-   Scheduled Flow menggunakan **cron expression** via Flowise scheduler
-   Semua agent output melalui **go-wa atau go-telegram** yang sudah ada (tidak buat gateway baru)
-   Internal auth via `X-Internal-Key` header (sudah terkonfigurasi di kedua gateway)
-   Logging: setiap interaksi disimpan ke PostgreSQL untuk audit dan retraining
-   **Model default**: Claude Haiku untuk bot respons cepat (CS, Admin, IoT), Claude Sonnet untuk agent yang butuh reasoning (Sales, Lead Gen, Marketing)
