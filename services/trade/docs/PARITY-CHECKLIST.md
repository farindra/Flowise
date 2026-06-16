# Parity Checklist — Go Bot vs Node Bot

Static audit of Go `wa-gateway-service` against `messageHandler.js` (5266 lines).
Covers all 30 test categories from `PANDUAN-PENGGUNAAN-BOT.md`.
WA manual test column requires actual phone testing (Phase 6 / cutover).

Legend: ✅ code-verified | ⚠️ minor difference | ❌ gap | 🔲 manual test pending

---

## Category 1 — Product Code Search (11 scenarios)

| # | Input | Expected | Go impl | Status |
|---|-------|----------|---------|--------|
| S01 | `6205` | Search results for bearing 6205 | `productCodeRe` → `handleProductSearch` | ✅ |
| S02 | `6203` | Search results for bearing 6203 | same | ✅ |
| S03 | `16016` | Search results for 5-digit code | same | ✅ |
| S04 | `32912 A` | Search results incl. variant | same | ✅ |
| S05 | `51208` | Search results for thrust bearing | same | ✅ |
| S06 | `SKF 6205` | Search results filtered to SKF | `hasBrand` → `handleProductSearch` | ✅ |
| S07 | `6224.FAG` | Search results for code.brand | `dotPatternRe` → search | ✅ |
| S08 | `6205-2RS` | Search results incl. suffix | `productCodeRe` (hyphen ok) → search | ✅ |
| S09 | `FAG NU318` | Search results — prefix code | `hasBrand` → search | ✅ |
| S10 | `TIMKEN HM89449` | Search results — complex | `hasBrand` → search | ✅ |
| S11 | `HM89449/HM89410` | Search results — slash taper | `productCodeRe` (has `/`) → search | ✅ |

**Notes:**
- All 11 inputs hit `productCodeRe` / `hasBrand` / `dotPatternRe` detection in `handleGeneralMessage` before the registered-user FAQ section, so they route to search regardless of registration status.
- `formatSearchResults` shows `GetCustomerPrice` per-item (VIP discount applied), stock status, and order instructions — mirrors Node exactly.

---

## Category 2 — Caret (^) Search (5 scenarios)

| # | Input | Expected | Go impl | Status |
|---|-------|----------|---------|--------|
| S12 | `^SKF` | SKF-specific results | `isCaretSearch` → strip `^` → `handleProductSearch("SKF")` | ✅ |
| S13 | `^NSK` | NSK-specific results | same | ✅ |
| S14 | `^FAG` | FAG-specific results | same | ✅ |
| S15 | `^NTN` | NTN-specific results | same | ✅ |
| S16 | `^toyota` | Toyota-specific results | same | ✅ |

**Notes:**
- Caret stripping happens in BOTH active-conv branch and unregistered-user branch of `handleGeneralMessage`.
- `^bearing 6205` also works: `isCaretSearch` → strip → search "bearing 6205".
- `IsGreeting` returns false for messages starting with `^` (explicit guard) — so `^halo` won't trigger greeting.

---

## Category 3 — Image Upload / AI Vision (3 scenarios)

| # | Input | Expected | Go impl | Status |
|---|-------|----------|---------|--------|
| S17 | Photo of bearing label | Detect codes → search each | `handleMedia` → `ai.AnalyzeImage` → search up to 2 products | ✅ |
| S18 | Handwritten note with codes | Detect handwritten codes | same (Gemini prompt tuned for handwriting) | ✅ |
| S19 | Screenshot of chat with codes | Extract codes from screenshot | same (prompt includes screenshot handling) | ✅ |

**Notes:**
- `handleMedia` downloads image via whatsmeow, converts to base64, calls `/analyze-image`.
- Results deduplicated by `kode+nama` key before formatting.
- Node's `isTextExtraction` branch skipped — `ai-vision-service` doesn't return that field; behavior matches since Gemini handles it the same way.

---

## Category 4 — Natural Conversation (8 scenarios)

| # | Input | Expected | Go impl | Status |
|---|-------|----------|---------|--------|
| S20 | `halo` | Warm greeting from Bobi | `IsGreeting` → `handleGreeting` → AI greeting or fallback | ✅ |
| S21 | `selamat pagi` | Time-of-day greeting | same | ✅ |
| S22 | `dimana alamat toko?` | Office address info | `IsLocationQuestion` → `handleLocationQuestion` | ✅ |
| S23 | `siapa kamu?` | Bobi self-introduction | `IsBotIdentityQuestion` → `handleBotIdentityQuestion` | ✅ |
| S24 | `harga bearing 6205?` | Price info for 6205 | `IsPriceQuestion` + productCode → `handlePriceQuestion` → `handleProductSearch` | ✅ |
| S25 | `stok 6205 ada?` | Stock info for 6205 | `IsStockQuestion` → `handleStockQuestion` → `handleProductSearch` | ✅ |
| S26 | `ppn berapa?` | PPN 11% explanation | `IsPPNQuestion` → `handlePPNQuestion` | ✅ |
| S27 | `bantuan` | Marketing contact options | `handleBantuanRequest` → Celvin/Puput selection | ✅ |

**Notes:**
- Greeting uses AI `/generate-natural` (isGreeting=true) with time-of-day awareness; falls back to `getFallbackGreeting` hardcoded list if AI down.
- `handleGreeting` tracks `lastGreet` per day — repeat greeting same day gets "ada yang bisa dibantu lagi?" style response.
- Marketing selection (1/2) after bantuan → `handleMarketingSelectionForBantuan` → sends WA to marketing.

---

## Category 5 — Practical Flow Scenarios (3 scenarios)

| # | Flow | Steps | Go impl | Status |
|---|------|-------|---------|--------|
| S28 | Search → Select → Cart | "6205" → results → "1" or "1x2" → cart confirmation | `handleNumberSelection` → `addSelectionToCart` → `handleCartCommand` | ✅ |
| S29 | Hashtag order | "#6205 #2" → order summary with pricing | `searchHashtagRe` → `handleHashtagOrder` → format with `GetCustomerPrice` | ✅ |
| S30 | Checkout flow | "checkout" → region selection → konfirmasi → order to marketing | `handleCheckout` → state machine → `processConfirmedCheckout` → `NotifyInternal` | ✅ |

---

## Additional Scenarios (from PANDUAN implicit testing)

| # | Input | Expected | Go impl | Status |
|---|-------|----------|---------|--------|
| S31 | `halo, saya cari bearing 6205` | Greeting + search combined | AI extracts product → `handleGeneralMessage("6205")` → search | ✅ |
| S32 | `lagi ngapain?` | Natural small talk | AI natural response via `/generate-natural` | ✅ |
| S33 | `/status` | Bot status info | `handleStatusCommand` | ✅ |
| S34 | `/reset` | Reset state | `handleResetCommand` | ✅ |
| S35 | `/cart` | Cart contents | `handleCartCommand` | ✅ |
| S36 | `/marketing` | Marketing contact | `handleMarketingCommand` | ✅ |
| S37 | `tidak` / `gak` | Negation → suggest alternatives | `handleNegationResponse` | ✅ |
| S38 | `ya` (when zero stock) | Warehouse search offer | `handleWarehouseSearchRequest` (ac.LastResults has zero-stock item) | ✅ |
| S39 | `pesan 1x5` | Direct order prefix | `(pesan\|order\|beli)\s+(.+)` → `processDirectOrder` → checkout | ✅ |
| S40 | Profanity input | Auto-register + skip | `containsBadWord` / AI profanity → set Perorangan/jakarta, prompt search | ✅ |

---

## Known Behavioral Differences

| Diff | Node | Go | Impact |
|------|----|------|--------|
| Natural greeting/response | Gemini via `generateNaturalGreeting` / `generateNaturalResponse` | Same via `/generate-natural` endpoint; hardcoded fallback if AI down | Low — same fallback behavior |
| `isMultiProductSearch` | Calls `productService.isMultiProductSearch` (regex + AI) | Covered by `AnalyzeMessage` AI extraction in `handleTextMessage` | Low — AI catches multi-product anyway |
| `generateResponse` in registration | Gemini free-form response for new user welcome | Go uses `/generate-natural` (isGreeting=true); same effect | Low — same fallback text |
| Registered user product keyword routing | Dead code in Node ("DUPLICATE LOGIC REMOVED") | Go: `r.isProductSearch()` runs as explicit check | Improves parity — catches edge cases AI misses |
| MongoDB chat_history format | Node: `$push messages []` per phone doc | Go: same `$push messages` bulkWrite pattern | None |
| Batch flush timing | Node: setInterval 30s, batchSize=10, threshold=50 | Go: same constants | None |

---

## Commands Checklist

| Command | Node handler | Go impl | Status |
|---------|-------------|---------|--------|
| `/help` | `handleHelp` | `handleHelp` in conversation.go | ✅ |
| `/status` | `handleStatusCommand` | `handleStatusCommand` in router.go | ✅ |
| `/reset` | `handleResetCommand` | `handleResetCommand` in router.go | ✅ |
| `/cart` | `handleCartCommand` | `handleCartCommand` in cart.go | ✅ |
| `/marketing` | `handleMarketingCommand` | `handleMarketingCommand` in notification | ✅ |

---

## State Machine Checklist

| State | Trigger | Go handler | Status |
|-------|---------|-----------|--------|
| `ASK_COMPANY_NAME_CHECKOUT` | `handleCheckout` when unregistered | `handleCompanyNameInput` | ✅ |
| `ASK_REGION_CHECKOUT` | after company name entered | `handleCheckoutRegionInput` | ✅ |
| `ASK_MARKETING_CHECKOUT` | after region if marketing unknown | `handleMarketingSelectionForCheckout` | ✅ |
| `CONFIRM_CHECKOUT` | `processCheckoutWithRegion` | `processConfirmedCheckout` or "batal" | ✅ |
| `ASK_MARKETING_BANTUAN` | `handleBantuanRequest` | `handleMarketingSelectionForBantuan` | ✅ |
| `ASK_MARKETING_WAREHOUSE` | `handleWarehouseSearchRequest` | `handleMarketingSelectionForWarehouse` | ✅ |
| `product_search` (ac.Context) | any product search | number selection / new search | ✅ |

---

## Data Sync Checklist

| Data | Node | Go | Status |
|------|------|------|--------|
| Chat history (local) | file JSON | sqlite `chat_history` table | ✅ |
| Chat history (cloud) | MongoDB `chat_history` ($push bulkWrite) | same via `mongo.Client.SyncHistory` | ✅ |
| User data (local) | in-memory + file JSON | sqlite `user_data` (JSON blob per phone) | ✅ |
| User data (cloud) | MongoDB `users` (upsert) | same via `mongo.Client.SyncUserData` | ✅ |
| Customer cache | 30s TTL, negative-cache flag | `state.CustomerCache` same logic | ✅ |
| Cart | in-memory array | sqlite key "cart" (JSON) | ✅ |

---

## Manual WA Test Log

*To be filled during Phase 6 cutover. Run each S01–S40 scenario on the Go bot phone number and record actual response.*

| Scenario | Input sent | Response received | Pass? | Notes |
|----------|-----------|------------------|-------|-------|
| S01 | 6205 | | 🔲 | |
| S02 | 6203 | | 🔲 | |
| S03 | 16016 | | 🔲 | |
| S04 | 32912 A | | 🔲 | |
| S05 | 51208 | | 🔲 | |
| S06 | SKF 6205 | | 🔲 | |
| S07 | 6224.FAG | | 🔲 | |
| S08 | 6205-2RS | | 🔲 | |
| S09 | FAG NU318 | | 🔲 | |
| S10 | TIMKEN HM89449 | | 🔲 | |
| S11 | HM89449/HM89410 | | 🔲 | |
| S12 | ^SKF | | 🔲 | |
| S13 | ^NSK | | 🔲 | |
| S14 | ^FAG | | 🔲 | |
| S15 | ^NTN | | 🔲 | |
| S16 | ^toyota | | 🔲 | |
| S17 | [image: bearing label] | | 🔲 | |
| S18 | [image: handwritten] | | 🔲 | |
| S19 | [image: screenshot] | | 🔲 | |
| S20 | halo | | 🔲 | |
| S21 | selamat pagi | | 🔲 | |
| S22 | dimana alamat toko? | | 🔲 | |
| S23 | siapa kamu? | | 🔲 | |
| S24 | harga bearing 6205? | | 🔲 | |
| S25 | stok 6205 ada? | | 🔲 | |
| S26 | ppn berapa? | | 🔲 | |
| S27 | bantuan | | 🔲 | |
| S28 | 6205 → 1 → checkout | | 🔲 | |
| S29 | #6205 #2 | | 🔲 | |
| S30 | checkout (with cart) | | 🔲 | |
| S31 | halo, saya cari bearing 6205 | | 🔲 | |
| S32 | lagi ngapain? | | 🔲 | |
| S33 | /status | | 🔲 | |
| S34 | /reset | | 🔲 | |
| S35 | /cart | | 🔲 | |
| S36 | /marketing | | 🔲 | |
| S37 | tidak / gak | | 🔲 | |
| S38 | ya (setelah hasil zero-stok) | | 🔲 | |
| S39 | pesan 1x5 | | 🔲 | |
| S40 | [profanity] | | 🔲 | |
