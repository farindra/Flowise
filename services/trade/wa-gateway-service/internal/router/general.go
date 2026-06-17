package router

import (
	"context"
	"log"
	"math/rand"
	"regexp"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types/events"

	"wa-gateway-service/internal/shared"
)

// frustratedPhrases mirrors the frustratedWords array in handleGeneralMessage
// (line 675-681).
var frustratedPhrases = []string{
	"muak", "kesal", "bodo", "bodoh", "bgt", "banget", "males", "malas",
	"capek", "cape", "bosen", "bosan", "gak usah", "ga usah", "skip",
	"lewati", "lanjut", "langsung", "ribet", "rumit", "lama", "lambat",
	"cepetan", "cepat", "buruk", "jelek", "anjing", "anjir", "anjay",
	"bodo amat", "bodo amatan", "gak penting", "ga penting", "ngapain",
	"ngapain sih", "ngapain juga", "gak perlu", "ga perlu", "gak butuh",
	"ga butuh", "gak mau", "ga mau", "gak peduli", "ga peduli",
	"gak ngerti", "ga ngerti", "gak paham", "ga paham", "gak jelas",
	"ga jelas", "gak tau", "ga tau", "gak tahu", "ga tahu", "gak bisa",
	"ga bisa", "gak bsa", "ga bsa", "gak bs", "ga bs",
}

var (
	updateCartRe  = regexp.MustCompile(`(?i)^ubah\s+(\d+)\s+(\d+)$`)
	removeCartRe  = regexp.MustCompile(`(?i)^hapus\s+(\d+)`)
	productCodeRe = regexp.MustCompile(`\b\d{2,}[\/\-]?\d{0,3}[A-Za-z0-9]*\b`)
	standaloneRe  = regexp.MustCompile(`^\s*\d{2,}[\/\-]?\d{0,3}[A-Za-z0-9]*\s*$`)
	dotPatternRe  = regexp.MustCompile(`\b[A-Za-z0-9]{3,}\.[A-Za-z]{2,}\b`)

	unexpectedInputRe1 = regexp.MustCompile(`^[!@#$%^&*()\-_=+\[\]{};':"\\|,.<>\/?]+$`)
	unexpectedInputRe2 = regexp.MustCompile(`^[a-zA-Z0-9!@#$%^&*()\-_=+\[\]{};':"\\|,.<>\/?]{1,5}$`)
	atLeastTwoDigits   = regexp.MustCompile(`^\d{2,}$`)

	generalProductExclusions = []*regexp.Regexp{
		regexp.MustCompile(`(?i)katalog\s+produk`),
		regexp.MustCompile(`(?i)daftar\s+produk`),
		regexp.MustCompile(`(?i)list\s+produk`),
		regexp.MustCompile(`(?i)brosur\s+produk`),
		regexp.MustCompile(`(?i)info\s+produk`),
		regexp.MustCompile(`(?i)jenis\s+produk`),
		regexp.MustCompile(`(?i)barang\s+rusak`),
		regexp.MustCompile(`(?i)produk\s+rusak`),
		regexp.MustCompile(`(?i)barang\s+cacat`),
		regexp.MustCompile(`(?i)komplain\s+barang`),
		regexp.MustCompile(`(?i)keluhan\s+barang`),
	}
)

// handleGeneralMessage ports messageHandler.handleGeneralMessage (line 467-1096).
// All sub-handlers not yet ported (3c-3h) are replaced with stubReply.
func (r *Router) handleGeneralMessage(ctx context.Context, evt *events.Message, messageBody string) error {
	phone := evt.Info.Sender.User
	if messageBody == "" {
		messageBody = ""
	}
	cleanMessage := strings.ToLower(strings.TrimSpace(messageBody))

	// Cart / keranjang command.
	if cleanMessage == "keranjang" || cleanMessage == "cart" {
		return r.handleCartCommand(ctx, evt)
	}

	// Checkout command.
	if cleanMessage == "checkout" {
		return r.handleCheckout(ctx, evt)
	}

	// Cart quantity update: "ubah <index> <qty>".
	if m := updateCartRe.FindStringSubmatch(cleanMessage); m != nil {
		idx, _ := strconv.Atoi(m[1])
		qty, _ := strconv.Atoi(m[2])
		return r.handleUpdateCartQuantity(ctx, evt, idx-1, qty) // 1-based → 0-based
	}

	// Back to search.
	if cleanMessage == "kembali" {
		return r.handleBackToSearch(ctx, evt)
	}

	// Remove from cart: "hapus <index>".
	if m := removeCartRe.FindStringSubmatch(cleanMessage); m != nil {
		idx, _ := strconv.Atoi(m[1])
		return r.handleRemoveFromCart(ctx, evt, idx-1) // 1-based → 0-based
	}

	// Active conversation state machine (checkout / bantuan flows).
	r.mu.Lock()
	ac := r.activeConvs[phone]
	r.mu.Unlock()

	if ac != nil {
		// Priority: bantuan marketing selection (mirrors handleFollowUpMessage
		// priority checks, line 3497-3512 in messageHandler.js).
		trimmedBody := strings.TrimSpace(messageBody)
		if ac.BantuanRequested && bantuanSelectionRe.MatchString(trimmedBody) {
			customerName := "Pelanggan"
			if c, err := r.cache.GetCustomerInfo(ctx, phone); err == nil && c != nil && c.Nama != "" {
				customerName = c.Nama
			}
			return r.handleMarketingSelectionForBantuan(ctx, evt, trimmedBody, customerName)
		}
		if ac.WarehouseSearchRequested && warehouseSelectionRe.MatchString(trimmedBody) {
			return r.handleMarketingSelectionForWarehouse(ctx, evt, trimmedBody)
		}

		// Active conversation "ya"/"iya" + any zero-stock lastResult → warehouse search.
		lower := strings.ToLower(trimmedBody)
		if (lower == "ya" || lower == "iya") && len(ac.LastResults) > 0 {
			hasZeroStock := false
			for _, p := range ac.LastResults {
				if p.Stok <= 0 {
					hasZeroStock = true
					break
				}
			}
			if hasZeroStock {
				return r.handleWarehouseSearchRequest(ctx, evt)
			}
		}

		switch ac.State {
		case "AWAITING_QTY_DIRECT":
			trimmedBody := strings.TrimSpace(messageBody)
			match := regexp.MustCompile(`^\d+`).FindString(trimmedBody)
			if match == "" {
				msg := "Masukkan jumlah yang valid ya (contoh: 5, 10, 100) 😊"
				r.reply(ctx, evt, msg)
				return r.store.AddToHistory(phone, "assistant", msg)
			}
			qty, _ := strconv.Atoi(match)
			if qty <= 0 {
				msg := "Jumlah harus lebih dari 0 ya 😊"
				r.reply(ctx, evt, msg)
				return r.store.AddToHistory(phone, "assistant", msg)
			}
			r.mu.Lock()
			ac2 := r.activeConvs[phone]
			r.mu.Unlock()
			if ac2 == nil || len(ac2.LastResults) == 0 {
				return r.handleGeneralMessage(ctx, evt, messageBody)
			}
			product := ac2.LastResults[0]
			r.mu.Lock()
			r.activeConvs[phone].State = ""
			r.mu.Unlock()
			return r.handleAddToCart(ctx, evt, product, qty)

		case "ASK_COMPANY_NAME_CHECKOUT":
			return r.handleCompanyNameInput(ctx, evt, messageBody)
		case "ASK_REGION_CHECKOUT":
			return r.handleCheckoutRegionInput(ctx, evt, messageBody)
		case "ASK_MARKETING_CHECKOUT":
			mb := strings.TrimSpace(messageBody)
			if mb == "1" || mb == "2" {
				return r.handleMarketingSelectionForCheckout(ctx, evt, mb)
			}
			msg := "❌ Pilihan tidak valid. Silakan pilih 1 atau 2."
			r.reply(ctx, evt, msg)
			return r.store.AddToHistory(phone, "assistant", msg)
		case "CONFIRM_CHECKOUT":
			lower := strings.ToLower(messageBody)
			if lower == "konfirmasi" || lower == "ya" || lower == "lanjut" {
				return r.processConfirmedCheckout(ctx, evt)
			} else if lower == "batal" || lower == "tidak" || lower == "cancel" {
				r.mu.Lock()
				if r.activeConvs[phone] != nil {
					r.activeConvs[phone].State = ""
				}
				r.mu.Unlock()
				msg := "✅ Checkout dibatalkan. Keranjang belanja Anda tetap tersimpan.\n\nKetik \"/cart\" untuk melihat keranjang atau lanjutkan mencari produk lain."
				r.reply(ctx, evt, msg)
				_ = r.store.AddToHistory(phone, "assistant", msg)
			}
			return nil
		}
	}

	// Auto-register unrecognised users with default data.
	var company string
	if _, err := r.store.Get(phone, "company", &company); err != nil {
		log.Printf("store.Get company error: %v", err)
	}
	if company == "" {
		_ = r.store.Set(phone, "company", "Perorangan")
		_ = r.store.Set(phone, "region", "jakarta")
		_ = r.store.SetUserState(phone, "idle")
	}

	// Active product-search conversation context.
	if ac != nil && ac.Active && ac.Context == "product_search" &&
		!shared.IsGreeting(messageBody) && !strings.HasPrefix(messageBody, "/") {

		isCaretSearch := strings.HasPrefix(strings.TrimSpace(messageBody), "^")
		trimmed := strings.TrimSpace(messageBody)

		hasProductCode := productCodeRe.MatchString(trimmed) || standaloneRe.MatchString(trimmed)

		// Brand detection from embedded list.
		upperMsg := strings.ToUpper(messageBody)
		upperWords := strings.Fields(strings.ToUpper(messageBody))
		hasBrand := false
		for _, brand := range shared.KnownBrands {
			brandUpper := strings.ToUpper(brand)
			if len(brandUpper) <= 2 && !isCaretSearch {
				continue
			}
			for _, w := range upperWords {
				if w == brandUpper {
					hasBrand = true
					break
				}
			}
			if !hasBrand && strings.Contains(upperMsg, brandUpper) {
				hasBrand = true
			}
			if hasBrand {
				break
			}
		}

		// Product code / brand / caret → search immediately (before number selection,
		// so "6205" or "NTN" in active context is not treated as a failed selection).
		if isCaretSearch || hasProductCode || hasBrand {
			query := messageBody
			if isCaretSearch {
				query = strings.TrimPrefix(strings.TrimSpace(messageBody), "^")
				query = strings.TrimSpace(query)
			}
			return r.handleProductSearch(ctx, evt, query)
		}

		// Number selection from last search results.
		if ac.LastResults != nil && len(ac.LastResults) > 0 {
			// Natural question (not a code/brand/number) → Flowise, skip number-selection.
			if !hasProductCode && !hasBrand {
				var history []string
				_, _ = r.store.Get(phone, "conversationHistory", &history)
				name := ""
				if c, err := r.cache.GetCustomerInfo(ctx, phone); err == nil && c != nil {
					name = c.Nama
				}
				if aiMsg := r.generateNatural(ctx, messageBody, phone, name, history, false, false); aiMsg != "" {
					r.reply(ctx, evt, aiMsg)
					return r.store.AddToHistory(phone, "assistant", aiMsg)
				}
			}

			handled, err := r.handleNumberSelection(ctx, evt, trimmed)
			if err != nil {
				return err
			}
			if handled {
				return nil
			}
		}

		// Fall-through: natural language query in product_search context → search.
		return r.handleProductSearch(ctx, evt, messageBody)
	}

	// Frustration detection.
	lower := strings.ToLower(messageBody)
	isFrustrated := false
	for _, phrase := range frustratedPhrases {
		if strings.Contains(lower, phrase) {
			isFrustrated = true
			break
		}
	}

	// Product code detection (same logic as active-conversation branch).
	trimmed := strings.TrimSpace(messageBody)
	hasProductCode := productCodeRe.MatchString(trimmed) || standaloneRe.MatchString(trimmed) || dotPatternRe.MatchString(trimmed)

	// Enhanced customer info.
	enhancedCustomer, err := r.cache.GetCustomerInfoEnhanced(ctx, phone)
	if err != nil {
		log.Printf("GetCustomerInfoEnhanced error for %s: %v", phone, err)
	}

	// Frustration shortcut — skip to product search or default message.
	if isFrustrated && !hasProductCode {
		var storedCompany string
		r.store.Get(phone, "company", &storedCompany) //nolint:errcheck
		if storedCompany == "" {
			_ = r.store.Set(phone, "company", "Perorangan")
			_ = r.store.Set(phone, "region", "jakarta")
			_ = r.store.SetUserState(phone, "idle")
			msg := "Maaf atas ketidaknyamanannya. ✅ Registrasi telah dilewati dan Anda dapat langsung mencari produk sekarang.\n\nSilakan ketik nama atau kode produk yang ingin Anda cari (contoh: 6224 atau 6224.FAG)."
			r.reply(ctx, evt, msg)
			return r.store.AddToHistory(phone, "assistant", msg)
		}
	}

	// Product code detected → product search.
	if hasProductCode {
		var storedCompany string
		r.store.Get(phone, "company", &storedCompany) //nolint:errcheck
		if storedCompany == "" {
			_ = r.store.Set(phone, "company", "Perorangan")
			_ = r.store.Set(phone, "region", "jakarta")
			_ = r.store.SetUserState(phone, "idle")
		}
		return r.handleProductSearch(ctx, evt, messageBody)
	}

	// FAQ detectors.
	if shared.IsGreeting(messageBody) {
		name := ""
		if enhancedCustomer != nil {
			name = enhancedCustomer.Nama
		}
		return r.handleGreeting(ctx, evt, name)
	}
	if shared.IsLocationQuestion(messageBody) {
		return r.handleLocationQuestion(ctx, evt)
	}
	if shared.IsMarketingQuestion(messageBody) {
		return r.handleMarketingQuestion(ctx, evt)
	}
	if shared.IsBotIdentityQuestion(messageBody) {
		return r.handleBotIdentityQuestion(ctx, evt)
	}
	if shared.IsPriceQuestion(messageBody) {
		return r.handlePriceQuestion(ctx, evt, messageBody)
	}
	if shared.IsStockQuestion(messageBody) {
		return r.handleStockQuestion(ctx, evt, messageBody)
	}
	if shared.IsPPNQuestion(messageBody) {
		return r.handlePPNQuestion(ctx, evt)
	}

	// Registration check for unregistered users.
	var isRegistered bool
	r.store.Get(phone, "isRegistered", &isRegistered) //nolint:errcheck
	var storedCompany string
	r.store.Get(phone, "company", &storedCompany) //nolint:errcheck
	var storedRegion string
	r.store.Get(phone, "region", &storedRegion) //nolint:errcheck
	hasBasicData := storedCompany != "" && storedRegion != ""

	if enhancedCustomer == nil && !isRegistered && !hasBasicData {
		// Unified product search detection for new/unregistered users.
		if r.isProductSearch(messageBody) {
			// Set active conversation context.
			r.setActiveConv(phone, &ActiveConv{
				Active:          true,
				Context:         "product_search",
				LastMessageTime: nowMs(),
			})
			_ = r.store.Set(phone, "activeConversation", map[string]interface{}{
				"active":          true,
				"context":         "product_search",
				"lastMessageTime": nowMs(),
			})
			_ = r.store.Set(phone, "company", "Perorangan")
			_ = r.store.Set(phone, "region", "jakarta")
			_ = r.store.SetUserState(phone, "idle")
			_ = r.store.Set(phone, "isRegistered", false)

			query := messageBody
			if strings.Contains(messageBody, "^") {
				parts := strings.SplitN(messageBody, "^", 2)
				if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
					query = strings.TrimSpace(parts[1])
				} else if strings.TrimSpace(parts[0]) != "" {
					query = strings.TrimSpace(parts[0])
				} else {
					query = strings.ReplaceAll(messageBody, "^", "")
				}
			}
			return r.handleProductSearch(ctx, evt, query)
		}
		// Not a product search → start registration.
		return r.startRegistration(ctx, evt)
	}

	// Registered user — greeting and FAQ re-check (after registration check).
	if shared.IsGreeting(messageBody) {
		name := ""
		if enhancedCustomer != nil {
			name = enhancedCustomer.Nama
		}
		return r.handleGreeting(ctx, evt, name)
	}
	if strings.Contains(lower, "alamat") || strings.Contains(lower, "dimana") || strings.Contains(lower, "lokasi") {
		return r.handleLocationQuestion(ctx, evt)
	}
	if strings.Contains(lower, "siapa") || strings.Contains(lower, "kamu siapa") || strings.Contains(lower, "bot") || strings.Contains(lower, "asisten") || strings.Contains(lower, "namamu") {
		return r.handleBotIdentityQuestion(ctx, evt)
	}
	if strings.Contains(lower, "harga") || strings.Contains(lower, "berapa") || strings.Contains(lower, "diskon") || strings.Contains(lower, "promo") {
		return r.handlePriceQuestion(ctx, evt, messageBody)
	}
	if strings.Contains(lower, "stok") || strings.Contains(lower, "stock") || strings.Contains(lower, "ready") || strings.Contains(lower, "tersedia") || strings.Contains(lower, "ada barang") {
		return r.handleStockQuestion(ctx, evt, messageBody)
	}
	if shared.IsPPNQuestion(messageBody) {
		return r.handlePPNQuestion(ctx, evt)
	}

	// Product search detection for registered users — same unified logic as
	// unregistered path (mirrors Node's isProductSearch call for ALL users).
	if r.isProductSearch(messageBody) {
		r.mu.Lock()
		if r.activeConvs[phone] == nil {
			r.activeConvs[phone] = &ActiveConv{}
		}
		r.activeConvs[phone].Active = true
		r.activeConvs[phone].Context = "product_search"
		r.activeConvs[phone].LastMessageTime = nowMs()
		r.mu.Unlock()
		return r.handleProductSearch(ctx, evt, messageBody)
	}

	// Unexpected input check.
	isUnexpected := unexpectedInputRe1.MatchString(messageBody) ||
		(unexpectedInputRe2.MatchString(messageBody) && !atLeastTwoDigits.MatchString(trimmed)) ||
		(len([]rune(messageBody)) < 3 && !atLeastTwoDigits.MatchString(trimmed))
	if isUnexpected {
		unexpectedResponses := []string{
			"Maaf, saya tidak mengerti pesan Anda. Silakan ketik nama produk yang Anda cari atau ketik /help untuk bantuan.",
			"Mohon maaf, saya tidak dapat memahami pesan tersebut. Coba ketik nama atau kode bearing yang Anda butuhkan.",
			"Saya tidak mengerti maksud Anda. Silakan coba lagi dengan mengetik nama produk atau ketik \"bantuan\" untuk informasi lebih lanjut.",
		}
		msg := unexpectedResponses[rand.Intn(len(unexpectedResponses))]
		r.reply(ctx, evt, msg)
		return r.store.AddToHistory(phone, "assistant", msg)
	}

	// Try AI natural response (mirrors Node's generateNaturalResponse fallback at line 1060).
	var history []string
	_, _ = r.store.Get(phone, "conversationHistory", &history)
	aiMsg := r.generateNatural(ctx, messageBody, phone, "", history, false, false)
	if aiMsg != "" {
		r.reply(ctx, evt, aiMsg)
		return r.store.AddToHistory(phone, "assistant", aiMsg)
	}

	defaultResponses := []string{
		"Silakan ketik nama produk yang Anda cari atau gunakan format \"^ nama produk\" untuk pencarian lebih akurat. Contoh: \"^ bearing toyota\" atau \"^ laher honda\". Ketik /help untuk bantuan.",
		"Mau cari bearing apa? Ketik nama atau kode produk, atau gunakan \"^ nama produk\" untuk pencarian spesifik. Contoh: \"^ bearing suzuki\" atau \"^ toyota\".",
		"Saya siap membantu Anda mencari produk. Ketik nama bearing atau gunakan format \"^ nama produk\" untuk hasil lebih tepat. Contoh: \"^ bearing daihatsu\" atau \"^ mitsubishi\".",
		"Butuh bantuan mencari bearing? Ketik nama produk atau gunakan \"^ nama produk\" untuk pencarian yang lebih akurat. Contoh: \"^ bearing isuzu\" atau \"^ hino\".",
	}
	msg := defaultResponses[rand.Intn(len(defaultResponses))]
	r.reply(ctx, evt, msg)
	return r.store.AddToHistory(phone, "assistant", msg)
}

// isProductSearch ports the "UNIFIED PRODUCT SEARCH DETECTION" logic in
// handleGeneralMessage (line 820-925) used for new/unregistered users.
func (r *Router) isProductSearch(messageBody string) bool {
	lower := strings.ToLower(messageBody)
	trimmed := strings.TrimSpace(messageBody)

	// Legacy product code pattern.
	legacyCodeRe := regexp.MustCompile(`^\d{2,4}\/?\d{0,4}\s*[a-zA-Z0-9]*$|` + `\b[A-Za-z0-9]{3,}\.[A-Za-z]{2,}\b`)
	isProductCodeSearch := legacyCodeRe.MatchString(trimmed)

	// Brand detection.
	upperMsg := strings.ToUpper(messageBody)
	upperWords := strings.Fields(strings.ToUpper(messageBody))
	cleanUpperMsg := strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(messageBody), "^"))
	hasBrand := false
	for _, brand := range shared.KnownBrands {
		brandUpper := strings.ToUpper(brand)
		for _, w := range upperWords {
			if w == brandUpper || strings.Contains(w, brandUpper) {
				hasBrand = true
				break
			}
		}
		if !hasBrand && strings.Contains(upperMsg, brandUpper) {
			hasBrand = true
		}
		if !hasBrand && strings.Contains(cleanUpperMsg, brandUpper) {
			hasBrand = true
		}
		if hasBrand {
			break
		}
	}

	hasGreaterThan := strings.Contains(messageBody, "^")

	// Product keywords.
	productKeywords := []string{"bearing", "laher", "produk", "barang", "toyota", "honda", "motor", "ntn", "skf", "fag", "timken", "mesin", "mobil", "sepeda", "ball", "roller", "seal", "deep groove", "thrust", "angular contact"}
	hasProductKw := false
	for _, kw := range productKeywords {
		if strings.Contains(lower, kw) {
			hasProductKw = true
			break
		}
	}

	// Context-aware product code patterns.
	hasContextCode := contextProductCodeRe.MatchString(messageBody)

	// Natural language patterns.
	hasNaturalSearch := naturalSearchRe.MatchString(messageBody)

	isValid := r.isValidProductQuery(messageBody)

	return (isProductCodeSearch || hasBrand || hasGreaterThan ||
		(len([]rune(messageBody)) > 2 && hasProductKw) ||
		hasContextCode || hasNaturalSearch) && isValid
}

var contextProductCodeRe = regexp.MustCompile(
	`(?i)(?:bearing|cari|cek|ada|stock|stok|harga|butuh|mau|tolong)\s+\d{2,4}\/?\d{0,4}(?:[a-zA-Z0-9\s]*)` +
		`|(?:tolong\s+)?carikan\s+\d{2,4}\/?\d{0,4}(?:[a-zA-Z0-9\s]*)` +
		`|\d{2,4}\/?\d{0,4}[a-zA-Z0-9\s]*\s+(?:ada|gak|tidak|dong|berapa|stock|stok)` +
		`|^\d{2,4}\/?\d{0,4}(?:[a-zA-Z0-9\s]*)$` +
		`|\b[A-Za-z0-9]{3,}\.[A-Za-z]{2,}\b`,
)

var naturalSearchRe = regexp.MustCompile(
	`(?i)(?:apakah\s+)?ada\s+bearing\s+[A-Za-z0-9\/\s]+` +
		`|(?:tolong\s+)?cek\s+(?:bearing\s+)?[0-9]{2,4}\/?\d{0,4}[A-Za-z0-9\s]*` +
		`|(?:saya\s+)?(?:mau\s+)?cari\s+(?:bearing\s+)?[A-Za-z0-9\/\s]+` +
		`|(?:ada\s+)?stok?\s+(?:bearing\s+)?[A-Za-z0-9\/\s]+` +
		`|(?:harga\s+)?bearing\s+[A-Za-z0-9\/\s]+` +
		`|(?:mau\s+)?beli\s+bearing\s+[A-Za-z0-9\/\s]+` +
		`|(?:butuh\s+)?bearing\s+[A-Za-z0-9\/\s]+`,
)
