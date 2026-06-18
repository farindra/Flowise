package router

import (
	"context"
	"log"
	"regexp"
	"strings"

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
