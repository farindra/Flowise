package router

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types/events"

	"wa-gateway-service/internal/client"
	"wa-gateway-service/internal/shared"
)

type hashtagOrderItem struct {
	ProductCode string
	Quantity    int
}

type numberSelection struct {
	Index    int
	Quantity int
}

type selectedProduct struct {
	Product  client.Product
	Quantity int
	Index    int // 1-based, for display
}

var searchHashtagRe = regexp.MustCompile(`#([A-Za-z0-9.-]+)(?:\s+#([0-9]+))?`)

// extractProductKeywords ports messageHandler.extractProductKeywords (line 5117).
func (r *Router) extractProductKeywords(message string) string {
	productKeywords := []string{
		"bearing", "laher", "ball bearing", "roller bearing", "thrust bearing",
		"seal", "gasket", "o-ring", "belt", "chain", "gear", "pulley",
		"coupling", "shaft", "bushing", "washer", "bolt", "nut", "screw",
		"spring", "valve", "pump", "motor", "fan", "filter", "oil",
		"grease", "lubricant", "hydraulic", "pneumatic",
	}
	stopWords := map[string]bool{
		"saya": true, "mau": true, "ingin": true, "cari": true, "butuh": true,
		"perlu": true, "ada": true, "ready": true, "stock": true, "stok": true,
		"harga": true, "price": true, "berapa": true, "bisa": true, "bantu": true,
		"tolong": true, "help": true, "dong": true, "nih": true, "ya": true,
		"kan": true, "lah": true, "kah": true, "dan": true, "atau": true,
		"dengan": true, "untuk": true, "dari": true, "ke": true, "di": true,
		"pada": true, "yang": true, "ini": true, "itu": true, "tersebut": true,
		"adalah": true, "akan": true, "sudah": true, "belum": true, "tidak": true,
		"jangan": true, "kalau": true, "jika": true, "bila": true, "apakah": true,
	}

	punctRe := regexp.MustCompile(`[^\w\s]`)
	spaceRe := regexp.MustCompile(`\s+`)
	digitRe := regexp.MustCompile(`\d`)
	alphaRe := regexp.MustCompile(`[a-z]`)
	pureNumRe := regexp.MustCompile(`^\d{3,}$`)
	wordNumRe := regexp.MustCompile(`\w*\d+\w*`)

	cleanMsg := strings.TrimSpace(spaceRe.ReplaceAllString(
		punctRe.ReplaceAllString(strings.ToLower(message), " "), " "))
	words := strings.Fields(cleanMsg)

	var relevant []string
words:
	for _, word := range words {
		if len([]rune(word)) < 2 {
			continue
		}
		if stopWords[word] {
			continue
		}
		for _, kw := range productKeywords {
			if strings.Contains(kw, word) || strings.Contains(word, kw) {
				relevant = append(relevant, word)
				continue words
			}
		}
		for _, brand := range shared.KnownBrands {
			if strings.ToLower(brand) == word {
				relevant = append(relevant, word)
				continue words
			}
		}
		if digitRe.MatchString(word) && alphaRe.MatchString(word) {
			relevant = append(relevant, word)
			continue
		}
		if pureNumRe.MatchString(word) {
			relevant = append(relevant, word)
			continue
		}
		if wordNumRe.MatchString(word) && len([]rune(word)) >= 3 {
			relevant = append(relevant, word)
		}
	}

	extracted := strings.Join(relevant, " ")
	if extracted == "" {
		for i := len(words) - 1; i >= 0; i-- {
			w := words[i]
			if !stopWords[w] && len([]rune(w)) >= 3 {
				return w
			}
		}
		return strings.TrimSpace(message)
	}
	return extracted
}

// isValidProductQuery ports messageHandler.isValidProductQuery (line 5188).
func (r *Router) isValidProductQuery(query string) bool {
	if strings.TrimSpace(query) == "" {
		return false
	}
	cleanQuery := strings.ToLower(strings.TrimSpace(query))
	genericTerms := map[string]bool{
		"bearing": true, "laher": true, "bantalan": true,
		"seal": true, "belt": true, "chain": true, "gear": true,
	}
	if regexp.MustCompile(`\d`).MatchString(cleanQuery) {
		return true
	}
	words := strings.Fields(cleanQuery)
	for _, word := range words {
		for _, brand := range shared.KnownBrands {
			if strings.ToLower(brand) == word {
				return true
			}
		}
	}
	for _, word := range words {
		if !genericTerms[word] {
			return true
		}
	}
	return false
}

// deduplicateSearchResults ports messageHandler.deduplicateSearchResults (line 1589).
func deduplicateSearchResults(results []client.Product) []client.Product {
	seen := make(map[string]bool)
	var out []client.Product
	for _, p := range results {
		key := p.Kode + p.Nama
		if !seen[key] {
			seen[key] = true
			out = append(out, p)
		}
	}
	return out
}

// formatSearchResults ports messageHandler.formatSearchResults (line 4238).
func (r *Router) formatSearchResults(ctx context.Context, phone string, results []client.Product, query string) (string, error) {
	if len(results) == 0 {
		return fmt.Sprintf("❌ *Pencarian \"%s\"*\n\nMaaf, tidak ada produk yang ditemukan.\n\n💡 Silakan periksa ejaan atau coba dengan kata kunci yang berbeda.", query), nil
	}

	telp := os.Getenv("TELPON_KANTOR")
	if telp == "" {
		telp = "021-6231-8301"
	}
	email := os.Getenv("EMAIL_KANTOR")
	if email == "" {
		email = "oceanbearings@gmail.com"
	}

	response := fmt.Sprintf("🔍 *Hasil Pencarian \"%s\"*\n", query)
	response += fmt.Sprintf("📦 *Ditemukan %d produk:*\n\n", len(results))

	for i, product := range results {
		finalPrice, err := r.cache.GetCustomerPrice(ctx, product, phone)
		if err != nil {
			log.Printf("formatSearchResults: GetCustomerPrice error for %s: %v", phone, err)
			finalPrice = float64(product.HargaNum.NonCustomer)
		}

		response += fmt.Sprintf("⭐ *%d. %s*\n", i+1, product.Kode)
		response += fmt.Sprintf("   📝 Nama: %s\n", product.Nama)
		if product.Stok > 0 {
			response += fmt.Sprintf("   📦 Stok: %d tersedia\n", product.Stok)
		} else {
			response += fmt.Sprintf("   ⚠️ Stok: %d tersedia\n", product.Stok)
		}
		response += fmt.Sprintf("   💰 Harga: %s\n", shared.FormatCurrency(finalPrice))
		if i < len(results)-1 {
			response += "\n"
		}
	}

	response += "\n   *notes : *Harga belum PPN*\n\n"
	response += "🛒 *Cara Memilih Produk:*\n"
	response += "• Ketik nomor produk (contoh: 1, 2, 3) untuk melihat detail\n"
	response += "• Ketik nomor dengan jumlah (contoh: 1x5, 2x3) untuk langsung pesan\n"
	response += "• Ketik beberapa nomor (contoh: 1,2,3 atau 1x2,3x5) untuk pilih multiple\n"
	response += "• Ketik \"pesan 1x5\" atau \"order 2,3\" untuk langsung checkout\n\n"
	response += "💬 *Untuk informasi lebih lanjut atau pemesanan:*\n"
	response += fmt.Sprintf("Telepon: %s\n", telp)
	response += fmt.Sprintf("Email: %s\n\n", email)
	response += "\nButuh bantuan? Ketik /help"

	return response, nil
}

// handleProductSearch ports messageHandler.handleProductSearch (line 1099).
func (r *Router) handleProductSearch(ctx context.Context, evt *events.Message, query string) error {
	phone := evt.Info.Sender.User

	_ = r.store.AddToHistory(phone, "user", query)

	// Hashtag order format (#KODE #QTY).
	hashtagMatches := searchHashtagRe.FindAllStringSubmatch(query, -1)
	if len(hashtagMatches) > 0 {
		var items []hashtagOrderItem
		for _, m := range hashtagMatches {
			qty := 1
			if len(m) > 2 && m[2] != "" {
				if n, err := strconv.Atoi(m[2]); err == nil {
					qty = n
				}
			}
			items = append(items, hashtagOrderItem{ProductCode: m[1], Quantity: qty})
		}
		return r.handleHashtagOrder(ctx, evt, items)
	}

	// AI analysis for keywords, profanity, enhancedQuery.
	cleanedQuery := query
	var queries []string
	var hasProfanity bool

	analysis, err := r.ai.AnalyzeMessage(ctx, query, phone)
	if err != nil {
		log.Printf("handleProductSearch: AnalyzeMessage error for %s: %v", phone, err)
	} else if analysis != nil {
		hasProfanity = analysis.ContainsProfanity
		if analysis.EnhancedQuery != "" {
			cleanedQuery = analysis.EnhancedQuery
		}
		if len(analysis.Keywords) > 0 {
			queries = analysis.Keywords
		}
	}

	if hasProfanity {
		msg := "Mohon gunakan bahasa yang sopan dalam berkomunikasi. Kami tetap akan memproses permintaan Anda."
		r.reply(ctx, evt, msg)
		_ = r.store.AddToHistory(phone, "assistant", msg)
	}

	if len(queries) == 0 {
		extracted := r.extractProductKeywords(cleanedQuery)
		if r.isValidProductQuery(extracted) {
			queries = []string{extracted}
		} else {
			log.Printf("handleProductSearch: query too generic for %s: %q", phone, extracted)
			helpMsg := "Untuk pencarian yang lebih akurat, mohon sertakan:\n" +
				"• Kode produk (contoh: bearing 6205)\n" +
				"• Pencarian Merk spesifik menggunakan tanda ^ (contoh: ^ SKF)\n" +
				"• Spesifikasi lengkap (contoh: bearing 6205 2RS)\n" +
				"• Upload foto berisi list kode bearing juga bisa\n\n" +
				"Atau ketik */help* untuk panduan lengkap."
			r.reply(ctx, evt, helpMsg)
			return r.store.AddToHistory(phone, "assistant", helpMsg)
		}
	}

	// Search products for each query term.
	var allResults []client.Product
	for _, q := range queries {
		found, err := r.search.Search(ctx, q, 10)
		if err != nil {
			log.Printf("handleProductSearch: Search error for %s q=%q: %v", phone, q, err)
			continue
		}
		allResults = append(allResults, found...)
	}

	// Deduplicate (by kode only, matching Node's dedup in handleProductSearch).
	seen := make(map[string]bool)
	var searchResults []client.Product
	for _, p := range allResults {
		if !seen[p.Kode] {
			seen[p.Kode] = true
			searchResults = append(searchResults, p)
		}
	}

	if len(searchResults) == 0 {
		customerName := "Pelanggan"
		if c, err := r.cache.GetCustomerInfo(ctx, phone); err == nil && c != nil && c.Nama != "" {
			customerName = c.Nama
		}

		queryStr := strings.Join(queries, ", ")
		isBearingSearch := false
		bearingKwRe := regexp.MustCompile(`\b(bearing|laher)\b`)
		digitRe := regexp.MustCompile(`\d`)
		brandRe := regexp.MustCompile(`(?i)\b(skf|nsk|fag|ntn|koyo|timken|ina|nachi|fbj|asahi|thk|iko)\b`)
		for _, q := range queries {
			ql := strings.ToLower(strings.TrimSpace(q))
			words := strings.Fields(ql)
			if bearingKwRe.MatchString(ql) && len(words) <= 2 &&
				!digitRe.MatchString(ql) && !brandRe.MatchString(ql) {
				isBearingSearch = true
				break
			}
		}

		var response string
		if isBearingSearch {
			response = "🔍 Saya siap membantu Anda mencari bearing yang tepat.\n\n" +
				"Untuk pencarian yang lebih akurat, mohon sertakan:\n" +
				"• Kode produk (contoh: bearing 6205)\n" +
				"• Pencarian Merk spesifik menggunakan tanda ^ (contoh: ^ SKF)\n" +
				"• Spesifikasi lengkap (contoh: bearing 6205 2RS)\n" +
				"• Upload foto berisi list kode bearing juga bisa\n\n" +
				"Atau ketik */help* untuk panduan lengkap."
		} else {
			response = fmt.Sprintf("😔 Maaf %s, produk \"%s\" tidak ditemukan.\n\nSilakan coba:\n• Gunakan kata kunci yang lebih umum\n• Periksa ejaan kode produk\n• Coba cari dengan nama produk lain\n\nAtau ketik \"bantuan\" untuk berbicara dengan tim marketing kami.", customerName, queryStr)
		}
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}

	// Format and send results.
	response, err := r.formatSearchResults(ctx, phone, searchResults, strings.Join(queries, ", "))
	if err != nil {
		log.Printf("handleProductSearch: formatSearchResults error for %s: %v", phone, err)
		r.sendErrorMessage(ctx, evt)
		return nil
	}
	r.reply(ctx, evt, response)
	_ = r.store.AddToHistory(phone, "assistant", response)

	// Store results in active conversation.
	r.mu.Lock()
	ac := r.activeConvs[phone]
	if ac == nil {
		ac = &ActiveConv{}
		r.activeConvs[phone] = ac
	}
	ac.Active = true
	ac.Context = "product_search"
	ac.LastResults = searchResults
	ac.LastMessageTime = nowMs()
	r.mu.Unlock()

	_ = r.store.Set(phone, "activeConversation", map[string]interface{}{
		"active":          true,
		"context":         "product_search",
		"lastMessageTime": nowMs(),
	})
	_ = r.store.Set(phone, "lastSearchQuery", query)

	return nil
}

// handleMedia ports messageHandler.handleMedia (line 1375).
func (r *Router) handleMedia(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User

	// Reset active conversation context to image_analysis.
	r.mu.Lock()
	if ac := r.activeConvs[phone]; ac != nil {
		ac.LastResults = nil
		ac.Context = "image_analysis"
		ac.LastMessageTime = nowMs()
	} else {
		r.activeConvs[phone] = &ActiveConv{
			Active:          true,
			Context:         "image_analysis",
			LastMessageTime: nowMs(),
		}
	}
	r.mu.Unlock()

	imgMsg := evt.Message.GetImageMessage()
	if imgMsg == nil {
		r.reply(ctx, evt, "Maaf, tidak dapat memproses gambar. Silakan ketik nama produk yang Anda cari.")
		return nil
	}

	r.reply(ctx, evt, "Bobi sedang menganalisis gambar, mohon ditunggu sebentar ya ⏳")

	imgData, err := r.wa.Download(ctx, imgMsg)
	if err != nil {
		log.Printf("handleMedia: Download error for %s: %v", phone, err)
		r.reply(ctx, evt, "Maaf, tidak dapat memproses gambar. Silakan ketik nama produk yang Anda cari.")
		return nil
	}

	imgBase64 := base64.StdEncoding.EncodeToString(imgData)
	mimetype := imgMsg.GetMimetype()
	if mimetype == "" {
		mimetype = "image/jpeg"
	}

	analysis, err := r.ai.AnalyzeImage(ctx, imgBase64, mimetype, phone)
	if err != nil {
		log.Printf("handleMedia: AnalyzeImage error for %s: %v", phone, err)
		r.reply(ctx, evt, "Maaf, tidak dapat memproses gambar. Silakan ketik nama produk yang Anda cari.")
		return nil
	}

	if analysis == nil || len(analysis.Products) == 0 {
		r.reply(ctx, evt, "Maaf, saya tidak dapat mendeteksi produk dari gambar tersebut. Silakan kirim gambar produk yang lebih jelas atau ketik nama/kode produk yang Anda cari.")
		return nil
	}

	// Show all detected codes first.
	allCodes := strings.Join(analysis.Products, ", ")
	r.reply(ctx, evt, fmt.Sprintf("📋 *Kode bearing yang terdeteksi dari gambar:*\n\n%s", allCodes))

	// Search each detected code (limit 2 per code, matching Node image path).
	var searchResults []client.Product
	searchedCodes := make(map[string]bool)
	for _, code := range analysis.Products {
		if searchedCodes[code] {
			continue
		}
		searchedCodes[code] = true
		results, err := r.search.Search(ctx, code, 2)
		if err != nil {
			log.Printf("handleMedia: Search error for %s code=%q: %v", phone, code, err)
			continue
		}
		searchResults = append(searchResults, results...)
	}

	if len(searchResults) == 0 {
		namesList := strings.Join(analysis.Products, ", ")
		r.reply(ctx, evt, fmt.Sprintf("Produk terdeteksi (%s), tetapi tidak ditemukan dalam database kami. Silakan coba dengan nama lain.", namesList))
		return nil
	}

	uniqueResults := deduplicateSearchResults(searchResults)
	response, err := r.formatSearchResults(ctx, phone, uniqueResults, "gambar")
	if err != nil {
		log.Printf("handleMedia: formatSearchResults error for %s: %v", phone, err)
		r.sendErrorMessage(ctx, evt)
		return nil
	}
	r.reply(ctx, evt, fmt.Sprintf("🔍 *Produk yang terdeteksi dari gambar:*\n\n%s", response))
	_ = r.store.AddToHistory(phone, "assistant", response)

	// Store results in active conversation.
	r.mu.Lock()
	ac := r.activeConvs[phone]
	if ac == nil {
		ac = &ActiveConv{}
		r.activeConvs[phone] = ac
	}
	ac.LastResults = uniqueResults
	ac.Active = true
	ac.Context = "product_search"
	r.mu.Unlock()

	_ = r.store.Set(phone, "lastSearchQuery", "gambar")
	_ = r.store.Set(phone, "activeConversation", map[string]interface{}{
		"active":          true,
		"context":         "product_search",
		"lastMessageTime": nowMs(),
	})

	return nil
}

// handleHashtagOrder ports messageHandler.handleHashtagOrder (line 4414).
func (r *Router) handleHashtagOrder(ctx context.Context, evt *events.Message, items []hashtagOrderItem) error {
	phone := evt.Info.Sender.User

	if len(items) == 0 {
		response := "❌ Format pemesanan tidak valid. Gunakan format: #KODE_PRODUK #JUMLAH"
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}

	type orderDetail struct {
		product  client.Product
		quantity int
		price    float64
		subtotal float64
	}

	var orderDetails []orderDetail
	var invalidProducts []string
	totalItems := 0

	for _, item := range items {
		results, err := r.search.Search(ctx, item.ProductCode, 1)
		if err != nil || len(results) == 0 {
			invalidProducts = append(invalidProducts, item.ProductCode)
			continue
		}
		product := results[0]
		price, err := r.cache.GetCustomerPrice(ctx, product, phone)
		if err != nil {
			log.Printf("handleHashtagOrder: GetCustomerPrice error for %s: %v", phone, err)
			price = float64(product.HargaNum.NonCustomer)
		}
		subtotal := price * float64(item.Quantity)
		orderDetails = append(orderDetails, orderDetail{
			product:  product,
			quantity: item.Quantity,
			price:    price,
			subtotal: subtotal,
		})
		totalItems += item.Quantity
	}

	if len(invalidProducts) > 0 {
		response := fmt.Sprintf("❌ Produk tidak ditemukan: %s\n\nGunakan kode produk yang valid.", strings.Join(invalidProducts, ", "))
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}

	var totalPrice float64
	for _, od := range orderDetails {
		totalPrice += od.subtotal
	}

	response := "🛒 *Pesanan Anda*\n\n"
	for i, od := range orderDetails {
		response += fmt.Sprintf("%d. %s - %s\n", i+1, od.product.Kode, od.product.Nama)
		response += fmt.Sprintf("   Jumlah: %d pcs\n", od.quantity)
		response += fmt.Sprintf("   Harga: %s / pcs\n", shared.FormatCurrency(od.price))
		response += fmt.Sprintf("   Subtotal: %s\n\n", shared.FormatCurrency(od.subtotal))
	}
	response += "📋 *Ringkasan Pesanan*\n"
	response += fmt.Sprintf("Total Item: %d pcs\n", totalItems)
	response += fmt.Sprintf("*Total: %s*\n\n", shared.FormatCurrency(totalPrice))
	response += "✅ *Konfirmasi Pesanan*\n"
	response += "Untuk konfirmasi pesanan, silakan ketik:\n"
	response += "*KONFIRMASI PESANAN*\n\n"
	response += "Untuk membatalkan, ketik:\n"
	response += "*BATAL PESANAN*\n\n"
	response += "📞 *Butuh bantuan?*\n"
	response += "Hubungi kami di 0812-9383-8000 atau ketik *bantuan*"

	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}

// parseNumberSelections ports messageHandler.parseNumberSelections (line 4632).
func parseNumberSelections(text string, maxIndex int) []numberSelection {
	if text == "" {
		return nil
	}
	cleanText := strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(text, " "))
	parts := regexp.MustCompile(`[,]|(?i)(\s+dan\s+)`).Split(cleanText, -1)

	qtyRe := regexp.MustCompile(`^(\d+)[xX](\d+)$`)
	numRe := regexp.MustCompile(`^(\d+)$`)

	var selections []numberSelection
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if m := qtyRe.FindStringSubmatch(part); m != nil {
			idx, _ := strconv.Atoi(m[1])
			qty, _ := strconv.Atoi(m[2])
			idx-- // 1-based → 0-based
			if idx >= 0 && idx < maxIndex && qty > 0 {
				selections = append(selections, numberSelection{Index: idx, Quantity: qty})
			}
			continue
		}
		if m := numRe.FindStringSubmatch(part); m != nil {
			idx, _ := strconv.Atoi(m[1])
			idx-- // 1-based → 0-based
			if idx >= 0 && idx < maxIndex {
				selections = append(selections, numberSelection{Index: idx, Quantity: 1})
			}
		}
	}
	return selections
}

// handleNumberSelection ports messageHandler.handleNumberSelection (line 4524).
// Returns (handled bool, err error); false means "let another handler process this".
func (r *Router) handleNumberSelection(ctx context.Context, evt *events.Message, messageBody string) (bool, error) {
	phone := evt.Info.Sender.User

	// Pattern that looks like a product code search rather than a selection.
	if regexp.MustCompile(`(?i)^\d{4,5}(\s+[a-zA-Z0-9]+)+`).MatchString(strings.TrimSpace(messageBody)) {
		return false, nil
	}

	r.mu.Lock()
	ac := r.activeConvs[phone]
	r.mu.Unlock()

	if ac == nil || len(ac.LastResults) == 0 {
		msg := "Maaf, saya tidak menemukan hasil pencarian sebelumnya. Silakan cari produk terlebih dahulu."
		r.reply(ctx, evt, msg)
		return true, r.store.AddToHistory(phone, "assistant", msg)
	}

	results := ac.LastResults

	// Detect "pesan/order/beli <selection>" direct-order prefix.
	var selectionText string
	isDirectOrder := false
	if m := regexp.MustCompile(`(?i)^(pesan|order|beli)\s+(.+)$`).FindStringSubmatch(messageBody); m != nil {
		selectionText = m[2]
		isDirectOrder = true
	} else {
		selectionText = messageBody
	}

	selections := parseNumberSelections(selectionText, len(results))
	if len(selections) == 0 {
		lower := strings.ToLower(messageBody)
		if len([]rune(messageBody)) > 5 ||
			strings.Contains(lower, "cari") ||
			strings.Contains(lower, "bearing") ||
			strings.Contains(lower, "halo") ||
			strings.Contains(lower, "hai") ||
			strings.Contains(lower, "mau") {
			return false, nil
		}
		msg := "Format pemilihan tidak valid. Silakan pilih dengan format angka (contoh: \"1\" atau \"1,2\" atau \"1x2\")."
		r.reply(ctx, evt, msg)
		return true, r.store.AddToHistory(phone, "assistant", msg)
	}

	// Filter to valid selections.
	var validSels []numberSelection
	for _, sel := range selections {
		if sel.Index >= 0 && sel.Index < len(results) && sel.Quantity > 0 {
			validSels = append(validSels, sel)
		}
	}
	if len(validSels) == 0 {
		msg := fmt.Sprintf("❌ Nomor produk tidak valid. Silakan pilih nomor 1-%d.", len(results))
		r.reply(ctx, evt, msg)
		return true, r.store.AddToHistory(phone, "assistant", msg)
	}

	var selected []selectedProduct
	for _, sel := range validSels {
		selected = append(selected, selectedProduct{
			Product:  results[sel.Index],
			Quantity: sel.Quantity,
			Index:    sel.Index + 1,
		})
	}

	if isDirectOrder {
		return true, r.processDirectOrder(ctx, evt, selected)
	}
	return true, r.showSelectionSummary(ctx, evt, selected)
}

// showSelectionSummary ports messageHandler.showSelectionSummary (line 4673):
// adds selection to cart and shows the cart.
func (r *Router) showSelectionSummary(ctx context.Context, evt *events.Message, selected []selectedProduct) error {
	if err := r.addSelectionToCart(ctx, evt, selected); err != nil {
		return err
	}
	return r.handleCartCommand(ctx, evt)
}

// addSelectionToCart ports messageHandler.addSelectionToCart (line 4726).
func (r *Router) addSelectionToCart(ctx context.Context, evt *events.Message, selected []selectedProduct) error {
	phone := evt.Info.Sender.User

	cart := r.loadCart(phone)
	for _, item := range selected {
		found := false
		for i, ci := range cart {
			if ci.Kode == item.Product.Kode {
				cart[i].Quantity += item.Quantity
				found = true
				break
			}
		}
		if !found {
			price, err := r.cache.GetCustomerPrice(ctx, item.Product, phone)
			if err != nil {
				log.Printf("addSelectionToCart: GetCustomerPrice error for %s: %v", phone, err)
				price = float64(item.Product.HargaNum.NonCustomer)
			}
			cart = append(cart, shared.CartItem{
				Kode:     item.Product.Kode,
				Nama:     item.Product.Nama,
				Quantity: item.Quantity,
				Harga:    price,
				Stok:     item.Product.Stok,
			})
		}
	}
	if err := r.saveCart(phone, cart); err != nil {
		log.Printf("addSelectionToCart: saveCart error for %s: %v", phone, err)
	}

	response := "🛒 *Produk berhasil ditambahkan ke keranjang!*\n\n📦 *Produk yang ditambahkan:*\n"
	for i, item := range selected {
		response += fmt.Sprintf("%d. %s x%d\n", i+1, item.Product.Kode, item.Quantity)
	}
	response += "\n💬 *Pilihan selanjutnya:*\n"
	response += "• Ketik \"/cart\" untuk melihat keranjang\n"
	response += "• Ketik \"checkout\" untuk melanjutkan pemesanan\n"
	response += "• Lanjutkan mencari produk lain"

	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}

// processDirectOrder ports messageHandler.processDirectOrder (line 4792):
// adds selection to cart then triggers checkout.
func (r *Router) processDirectOrder(ctx context.Context, evt *events.Message, selected []selectedProduct) error {
	if err := r.addSelectionToCart(ctx, evt, selected); err != nil {
		return err
	}
	return r.handleCheckout(ctx, evt)
}
