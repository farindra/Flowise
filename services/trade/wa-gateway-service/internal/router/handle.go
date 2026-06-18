package router

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types/events"
)

// badWords is the manual profanity fallback list when AI is unavailable.
// Matches messageHandler.js lines 215, 368, 794.
var badWords = []string{
	"anjing", "anjir", "anjay", "bangsat", "babi", "kontol", "memek",
	"ngentot", "goblok", "goblog", "tolol", "bego", "bodoh", "idiot",
}

var skipRegistrationPhrases = []string{
	"langsung cari", "cari produk", "cari bearing", "cari barang",
	"skip registrasi", "lewati registrasi",
}

var negationWords = []string{
	"tidak", "bukan", "gak", "ga", "gk", "ngk", "ngga", "nggak", "no",
	"nope", "jangan", "tdk", "tak", "enggak", "engga", "engg", "gag",
	"ogah", "ndak", "nda", "kagak", "kaga", "kgk",
}

var continueWords = []string{
	"lanjut", "selesai", "skip", "next", "selanjutnya", "teruskan", "lanjutkan",
}

// handleSingleMessage ports messageHandler.handleSingleMessage (line 184-312).
func (r *Router) handleSingleMessage(evt *events.Message) error {
	ctx := context.Background()
	phone := evt.Info.Sender.User
	body := strings.TrimSpace(msgBody(evt))

	// Owner numbers: handle Excel document uploads → download, ask supplier+currency.
	if r.ownerPhones[phone] && r.trade != nil && hasExcelDoc(evt) {
		return r.handleOwnerSupplierUpload(ctx, evt)
	}

	// Owner numbers: if there's a pending upload, intercept text as supplier+currency reply.
	if r.ownerPhones[phone] && r.trade != nil {
		if p := r.takePendingUpload(phone); p != nil {
			if time.Since(p.at) > 10*time.Minute {
				r.reply(ctx, evt, "⏰ Upload expired. Kirim ulang file Excel-nya.")
				return nil
			}
			supplierName, currency, ok := parseSupplierReply(body)
			if !ok {
				r.setPendingUpload(phone, p) // put back
				r.reply(ctx, evt, "❓ Format tidak dikenali. Balas dengan:\nSUPPLIER: <nama>, CURRENCY: <USD/IDR/JPY/dll>\n\nAtau ketik skip untuk auto-detect.")
				return nil
			}
			go r.doUploadSupplierOffer(ctx, evt, p, supplierName, currency)
			return nil
		}
	}

	// Owner numbers: route all non-command text messages directly to owner assistant.
	if r.ownerPhones[phone] && r.ownerFlowise != nil && body != "" && !strings.HasPrefix(body, "/") {
		r.reply(ctx, evt, "⏳ Memproses...")
		go func() {
			bgCtx := context.Background()
			answer := r.ownerFlowise.AskDirect(bgCtx, body, "owner-wa-"+phone)
			if answer == "" {
				answer = "❌ Owner Assistant tidak merespons. Coba lagi."
			}
			r.reply(bgCtx, evt, answer)
			_ = r.store.AddToHistory(phone, "assistant", answer)
		}()
		return nil
	}

	// Empty message (no text and no media) → default greeting.
	if body == "" && !hasMedia(evt) {
		msg := "Halo 👋 Saya Bobi dari Ocean Bearing Indonesia.\n\nSilakan ketik nama atau kode produk yang ingin Anda cari, atau kirim gambar produk untuk saya identifikasi ya."
		r.reply(ctx, evt, msg)
		return r.store.AddToHistory(phone, "assistant", msg)
	}

	// AI profanity check with manual fallback.
	containsBadWord := false
	if body != "" {
		analysis, err := r.ai.AnalyzeMessage(ctx, body, phone)
		if err != nil {
			log.Printf("AI.AnalyzeMessage error for %s: %v", phone, err)
			lower := strings.ToLower(body)
			for _, w := range badWords {
				if strings.Contains(lower, w) {
					containsBadWord = true
					break
				}
			}
		} else if analysis != nil && analysis.ContainsProfanity {
			containsBadWord = true
		}
	}

	if containsBadWord {
		_ = r.store.Set(phone, "company", "Perorangan")
		_ = r.store.Set(phone, "region", "jakarta")
		_ = r.store.SetUserState(phone, "idle")
		msg := "Maaf atas ketidaknyamanannya. ✅ Registrasi telah dilewati dan Anda dapat langsung mencari produk sekarang.\n\nSilakan ketik nama atau kode produk yang ingin Anda cari (contoh: 6224 atau 6224.FAG)."
		r.reply(ctx, evt, msg)
		return r.store.AddToHistory(phone, "assistant", msg)
	}

	// Skip-registration shortcut phrases.
	lowerBody := strings.ToLower(body)
	wantsSkip := false
	for _, phrase := range skipRegistrationPhrases {
		if strings.Contains(lowerBody, phrase) {
			wantsSkip = true
			break
		}
	}
	if wantsSkip {
		var company string
		r.store.Get(phone, "company", &company) //nolint:errcheck
		if company == "" {
			_ = r.store.Set(phone, "company", "Perorangan")
			_ = r.store.Set(phone, "region", "jakarta")
			_ = r.store.SetUserState(phone, "idle")
			msg := "Baik, Anda dapat langsung mencari produk. Anda telah terdaftar sebagai pelanggan perorangan di wilayah Jakarta.\n\nSilakan ketik nama atau kode produk yang ingin Anda cari."
			r.reply(ctx, evt, msg)
			return r.store.AddToHistory(phone, "assistant", msg)
		}
	}

	// Slash commands.
	if strings.HasPrefix(body, "/") {
		if err := r.handleCommand(ctx, evt, body); err != nil {
			return err
		}
		return nil
	}

	// Media messages.
	if hasMedia(evt) {
		return r.handleMedia(ctx, evt)
	}

	// Text messages.
	return r.handleTextMessage(ctx, evt, body)
}

// hasExcelDoc returns true if the message carries an Excel document attachment.
func hasExcelDoc(evt *events.Message) bool {
	doc := evt.Message.GetDocumentMessage()
	if doc == nil {
		return false
	}
	mime := doc.GetMimetype()
	fname := doc.GetFileName()
	return mime == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" ||
		mime == "application/vnd.ms-excel" ||
		strings.HasSuffix(strings.ToLower(fname), ".xlsx") ||
		strings.HasSuffix(strings.ToLower(fname), ".xls")
}

// parseSupplierReply parses a reply like "SUPPLIER: SANKO, CURRENCY: USD"
// or "skip". Returns (supplierName, currency, ok).
func parseSupplierReply(text string) (supplierName, currency string, ok bool) {
	t := strings.TrimSpace(text)
	lower := strings.ToLower(t)
	if lower == "skip" || lower == "auto" || lower == "-" {
		return "", "USD", true
	}
	reSup := regexp.MustCompile(`(?i)supplier\s*:\s*([^,\n]+)`)
	reCur := regexp.MustCompile(`(?i)currency\s*:\s*([A-Za-z]{3})`)
	mSup := reSup.FindStringSubmatch(t)
	mCur := reCur.FindStringSubmatch(t)
	if mSup == nil && mCur == nil {
		return "", "", false
	}
	if mSup != nil {
		supplierName = strings.TrimSpace(mSup[1])
	}
	if mCur != nil {
		currency = strings.ToUpper(strings.TrimSpace(mCur[1]))
	} else {
		currency = "USD"
	}
	return supplierName, currency, true
}

// handleOwnerSupplierUpload downloads an Excel doc and asks the owner for
// supplier name + currency before uploading to TRADE.
func (r *Router) handleOwnerSupplierUpload(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User
	doc := evt.Message.GetDocumentMessage()
	fname := doc.GetFileName()
	if fname == "" {
		fname = "penawaran-supplier.xlsx"
	}

	r.reply(ctx, evt, "⏳ Mengunduh file dari WhatsApp...")

	go func() {
		bgCtx := context.Background()
		data, err := r.wa.Download(bgCtx, doc)
		if err != nil {
			log.Printf("handleOwnerSupplierUpload: download error for %s: %v", phone, err)
			r.reply(bgCtx, evt, "❌ Gagal mengunduh file. Coba kirim ulang.")
			return
		}

		r.setPendingUpload(phone, &pendingUpload{fileData: data, fileName: fname, at: time.Now()})

		msg := fmt.Sprintf(
			"📄 File *%s* (%.1f KB) siap diupload.\n\n"+
				"Balas dengan format:\n"+
				"SUPPLIER: <nama supplier>, CURRENCY: <USD/IDR/SGD/JPY/EUR>\n\n"+
				"Contoh: SUPPLIER: SANKO, CURRENCY: USD\n\n"+
				"Atau ketik *skip* untuk auto-detect dari file.",
			fname, float64(len(data))/1024,
		)
		r.reply(bgCtx, evt, msg)
	}()
	return nil
}

// doUploadSupplierOffer uploads the pending Excel file to TRADE.
func (r *Router) doUploadSupplierOffer(ctx context.Context, evt *events.Message, p *pendingUpload, supplierName, currency string) {
	phone := evt.Info.Sender.User
	r.reply(ctx, evt, fmt.Sprintf("⏳ Mengupload *%s* ke TRADE...", p.fileName))

	result, err := r.trade.UploadSupplierOffer(ctx, p.fileData, p.fileName, supplierName, currency)
	if err != nil {
		log.Printf("doUploadSupplierOffer: upload error for %s: %v", phone, err)
		r.reply(ctx, evt, "❌ Gagal upload ke TRADE: "+err.Error())
		return
	}

	supLabel := supplierName
	if supLabel == "" {
		supLabel = "_(auto-detect dari file)_"
	}
	msg := fmt.Sprintf(
		"✅ *File penawaran supplier diterima!*\n\n"+
			"📄 File: %s\n"+
			"🏢 Supplier: %s\n"+
			"💱 Currency: %s\n"+
			"🔑 Upload ID: %s\n\n"+
			"Proses auto-mapping produk sedang berjalan di background.\n"+
			"Cek hasil di TRADE → *Penawaran Supplier* dalam beberapa menit.",
		p.fileName, supLabel, currency, result.UploadID,
	)
	r.reply(ctx, evt, msg)
	_ = r.store.AddToHistory(phone, "assistant", msg)
}

var fallbackProductCodeRe = regexp.MustCompile(`\b\d{2,}[\/\-]?\d{0,3}[A-Za-z0-9]*\b`)

// handleTextMessage ports messageHandler.handleTextMessage (line 328-464).
func (r *Router) handleTextMessage(ctx context.Context, evt *events.Message, body string) error {
	phone := evt.Info.Sender.User
	lower := strings.ToLower(body)

	// Panduan / bantuan bobi triggers → handleHelp.
	for _, trigger := range []string{"panduan bobi", "bantuan bobi", "panduan"} {
		if strings.Contains(lower, trigger) {
			return r.handleHelp(ctx, evt)
		}
	}
	if lower == "bantuan" {
		return r.handleBantuanRequest(ctx, evt)
	}
	if lower == "help" {
		return r.handleHelp(ctx, evt)
	}

	// AI intent analysis.
	var intent, enhancedQuery string
	var aiProducts []string
	var aiQuantity int
	var containsProfanityAI bool
	analysis, err := r.ai.AnalyzeMessage(ctx, body, phone)
	if err != nil {
		log.Printf("AI.AnalyzeMessage (text) error for %s: %v", phone, err)
		intent = "general_search"
		enhancedQuery = body
	} else if analysis != nil {
		intent = analysis.Intent
		aiProducts = analysis.Products
		aiQuantity = analysis.Quantity
		enhancedQuery = analysis.EnhancedQuery
		containsProfanityAI = analysis.ContainsProfanity
		if enhancedQuery == "" {
			enhancedQuery = body
		}
	}

	// Manual bad-word fallback.
	manualBadWord := false
	for _, w := range badWords {
		if strings.Contains(lower, w) {
			manualBadWord = true
			break
		}
	}
	if containsProfanityAI || manualBadWord {
		_ = r.store.Set(phone, "company", "Perorangan")
		_ = r.store.Set(phone, "region", "jakarta")
		_ = r.store.SetUserState(phone, "idle")
		msg := "Maaf atas ketidaknyamanannya. ✅ Registrasi telah dilewati dan Anda dapat langsung mencari produk sekarang.\n\nSilakan ketik nama atau kode produk yang ingin Anda cari (contoh: 6224 atau 6224.FAG)."
		r.reply(ctx, evt, msg)
		return r.store.AddToHistory(phone, "assistant", msg)
	}

	// Greeting intent from AI.
	if intent == "greeting" {
		return r.handleGreeting(ctx, evt, "")
	}

	// "order" intent: extract product code directly from message body.
	// AI often splits product codes with spaces (e.g. "6205 ZZ (KOREA).FAG" →
	// ["6205","ZZ","FAG"]), so body extraction is more reliable than aiProducts.
	if intent == "order" {
		if code := extractOrderCode(body); code != "" {
			return r.handleDirectOrderRequest(ctx, evt, code)
		}
	}

	// Products detected by AI → route to product search or direct-order flow.
	if len(aiProducts) > 0 {
		_ = aiQuantity // used in 3g
		searchQuery := strings.Join(aiProducts, ", ")
		if len(aiProducts) == 1 {
			searchQuery = aiProducts[0]
		}
		return r.handleGeneralMessage(ctx, evt, searchQuery)
	}

	// AI found price/stock/order intent but no product code — route to general handler
	// which will call handleProductSearch → Flowise fallback for natural queries.
	if intent == "price_check" || intent == "stock_check" || intent == "order" {
		return r.handleGeneralMessage(ctx, evt, body)
	}

	// Negation words check.
	for _, w := range negationWords {
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(w) + `\b`)
		if re.MatchString(lower) {
			return r.handleNegationResponse(ctx, evt)
		}
	}

	// Continue words check.
	for _, w := range continueWords {
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(w) + `\b`)
		if re.MatchString(lower) {
			return r.handleContinueResponse(ctx, evt)
		}
	}

	// General message with AI-enhanced query.
	return r.handleGeneralMessage(ctx, evt, enhancedQuery)
}

// extractOrderCode strips common Indonesian order trigger words from the body
// and returns the remainder as the product code. Returns "" if nothing useful
// remains. Handles cases like "gw pesan 6205 ZZ (KOREA).FAG" → "6205 ZZ (KOREA).FAG".
func extractOrderCode(body string) string {
	lower := strings.ToLower(body)
	triggers := []string{
		"tolong pesan", "mau pesan", "saya pesan", "aku pesan",
		"gw pesan", "gue pesan", "sy pesan",
		"tolong order", "mau order", "saya order", "aku order",
		"gw order", "gue order",
		"tolong beli", "mau beli", "saya beli", "aku beli",
		"gw beli", "gue beli",
		"pesan", "order", "beli",
	}
	for _, t := range triggers {
		if idx := strings.Index(lower, t); idx != -1 {
			after := strings.TrimSpace(body[idx+len(t):])
			if after != "" {
				return after
			}
		}
	}
	return ""
}
