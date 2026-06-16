package router

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types/events"

	"wa-gateway-service/internal/client"
	"wa-gateway-service/internal/notification"
	"wa-gateway-service/internal/shared"
)

// handleGreeting ports messageHandler.handleGreeting (line 1713).
func (r *Router) handleGreeting(ctx context.Context, evt *events.Message, customerName string) error {
	phone := evt.Info.Sender.User
	msgBody := strings.TrimSpace(msgBody(evt))

	today := time.Now().Format("Mon Jan 02 2006")
	var lastGreet string
	_, _ = r.store.Get(phone, "lastGreet", &lastGreet)
	isFirstTimeToday := lastGreet != today

	if isFirstTimeToday {
		_ = r.store.Set(phone, "lastGreet", today)
	}

	// Try AI-generated response first (mirrors Node's generateNaturalGreeting /
	// generateNaturalResponse calls). Falls back to hardcoded if AI unavailable.
	var history []string
	_, _ = r.store.Get(phone, "conversationHistory", &history)
	response := r.generateNatural(ctx, msgBody, phone, customerName, history, true, isFirstTimeToday)

	if response == "" {
		// Hardcoded fallback matches getFallbackGreeting() in aiService.js.
		hour := time.Now().Hour()
		greeting := "Halo"
		if hour >= 5 && hour < 11 {
			greeting = "Selamat pagi"
		} else if hour >= 11 && hour < 15 {
			greeting = "Selamat siang"
		} else if hour >= 15 && hour < 19 {
			greeting = "Selamat sore"
		} else {
			greeting = "Selamat malam"
		}
		greetings := []string{
			fmt.Sprintf("%s 😊 Gimana kabarnya hari ini? Saya Bobi dari Ocean Bearing nih. Ada yang bisa dibantu?", greeting),
			fmt.Sprintf("%s 👋 Senang banget ada yang mampir. Saya Bobi, siap bantu-bantu. Gimana, lagi cari apa nih?", greeting),
			fmt.Sprintf("%s. Hehe, saya Bobi dari Ocean Bearing. Semoga harinya lancar ya. Ada yang perlu bantuan?", greeting),
			fmt.Sprintf("%s 😄 Salam kenal, saya Bobi. Lagi santai atau ada yang mau dicari nih?", greeting),
		}
		response = greetings[rand.Intn(len(greetings))]
	}

	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}

// handleNegationResponse ports messageHandler.handleNegationResponse (line 1758).
func (r *Router) handleNegationResponse(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User
	response := `mau Bobi tawarkan merek lain?

Silakan ketik merek yang Anda inginkan dengan format: "^ [nama merek]"
Contoh: "^ NTN" atau "^ SKF"

*Catatan:* Penggunaan tanda "^" di awal pencarian akan membantu Bobi menemukan merek yang tepat untuk Anda.`
	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}

// handleContinueResponse ports messageHandler.handleContinueResponse (line 1779).
func (r *Router) handleContinueResponse(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User

	customerName := "Customer"
	if c, err := r.cache.GetCustomerInfo(ctx, phone); err == nil && c != nil && c.Nama != "" {
		customerName = c.Nama
	}

	response := fmt.Sprintf(`Baik %s, ada yang bisa Bobi bantu lagi?

Anda dapat mencari produk dengan cara:
• Ketik kode produk (contoh: "6224.FAG")
• Ketik nama produk dengan format "^ [nama merek]" (contoh: "^ NTN")
• Kirim foto bearing untuk identifikasi otomatis

*Catatan:* Penggunaan tanda "^" di awal pencarian akan membantu Bobi menemukan merek yang tepat untuk Anda.`, customerName)

	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}

// handleBotIdentityQuestion ports messageHandler.handleBotIdentityQuestion
// (line 2051): picks one of three randomised responses.
func (r *Router) handleBotIdentityQuestion(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User

	customerName := "Customer"
	if c, err := r.cache.GetCustomerInfo(ctx, phone); err == nil && c != nil && c.Nama != "" {
		customerName = c.Nama
	}

	responses := []string{
		fmt.Sprintf("Halo %s, saya adalah Bobi, asisten virtual dari Ocean Bearing. Saya di sini untuk membantu Anda mencari produk bearing, menjawab pertanyaan, dan menghubungkan Anda dengan tim marketing kami. 🤖", customerName),
		fmt.Sprintf("Perkenalkan, saya Bobi, asisten AI dari Ocean Bearing yang siap membantu kebutuhan bearing Anda. Saya dapat mencari produk, memberikan informasi, dan menghubungkan Anda dengan tim kami. Ada yang bisa saya bantu, %s? 🤖", customerName),
		fmt.Sprintf("Saya Bobi, asisten digital Ocean Bearing yang dirancang untuk membantu Anda menemukan bearing yang tepat dan menjawab pertanyaan seputar produk kami. Bagaimana saya bisa membantu Anda hari ini, %s? 🤖", customerName),
	}
	response := responses[rand.Intn(len(responses))]
	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}

// handleBantuanRequest ports messageHandler.handleBantuanRequest (line 2094):
// sets BantuanRequested flag on the active conversation and presents the
// two-choice marketing selection menu.
func (r *Router) handleBantuanRequest(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User

	customerName := "Pelanggan"
	if c, err := r.cache.GetCustomerInfo(ctx, phone); err == nil && c != nil && c.Nama != "" {
		customerName = c.Nama
	}

	r.mu.Lock()
	ac := r.activeConvs[phone]
	if ac == nil {
		ac = &ActiveConv{}
		r.activeConvs[phone] = ac
	}
	ac.BantuanRequested = true
	r.mu.Unlock()

	response := fmt.Sprintf(`🙋‍♂️ *Permintaan Bantuan*

Baik %s, saya akan menghubungkan Anda dengan tim marketing kami.

Untuk memproses permintaan Anda dengan lebih baik, mohon pilih marketing yang sesuai dengan region/wilayah Anda:

📍 *Pilih tim marketing Anda:*
1️⃣ Celvin (0812-8830-9688) - Jawa Barat, Jawa Tengah, Kalimantan, Riau, Jawa Timur, Palembang, Jambi, Bangka Belitung
2️⃣ Puput (0812-8298-3305) - Sulawesi, Sumatra, Papua, Maluku, NTT, NTB, Bali, Lampung

💬 Ketik nomor pilihan Anda (1-2)`, customerName)

	r.reply(ctx, evt, response)
	_ = r.store.AddToHistory(phone, "user", "Meminta bantuan")
	return r.store.AddToHistory(phone, "assistant", response)
}

// bantuanTeams mirrors the hardcoded marketingTeams inside
// handleMarketingSelectionForBantuan (line 3923).
var bantuanTeams = map[string]notification.MarketingInfo{
	"1": {Number: "6281288309688", Name: "Celvin"},
	"2": {Number: "6281282983305", Name: "Puput"},
}

var bantuanTeamRegions = map[string]string{
	"1": "Jawa Barat, Jawa Tengah, Kalimantan, Riau, Jawa Timur, Palembang, Jambi, Bangka Belitung",
	"2": "Sulawesi, Sumatra, Papua, Maluku, NTT, NTB, Bali, Lampung",
}

// bantuanSelectionRe and warehouseSelectionRe match the Node regexes used in
// handleFollowUpMessage to detect bantuan / warehouse number input.
var bantuanSelectionRe = regexp.MustCompile(`^[1-3]$`)
var warehouseSelectionRe = regexp.MustCompile(`^[1-2]$`)

// handleMarketingSelectionForBantuan ports line 3914.
func (r *Router) handleMarketingSelectionForBantuan(ctx context.Context, evt *events.Message, selection, customerName string) error {
	phone := evt.Info.Sender.User

	mkt, ok := bantuanTeams[selection]
	if !ok {
		errMsg := "❌ Pilihan tidak valid. Silakan pilih 1 atau 2."
		r.reply(ctx, evt, errMsg)
		return r.store.AddToHistory(phone, "assistant", errMsg)
	}

	r.mu.Lock()
	if ac := r.activeConvs[phone]; ac != nil {
		ac.BantuanRequested = false
	}
	r.mu.Unlock()

	regions := bantuanTeamRegions[selection]
	response := fmt.Sprintf(`✅ *Permintaan Bantuan Diteruskan*

Terima kasih %s. Permintaan bantuan Anda telah diteruskan ke %s (%s).

Tim marketing kami akan segera menghubungi Anda untuk membantu kebutuhan Anda. Mohon tunggu sebentar ya.

Jika ada pertanyaan spesifik tentang produk, Anda juga bisa langsung mencari dengan mengetik nama atau kode produk.`, customerName, mkt.Name, regions)

	r.reply(ctx, evt, response)
	_ = r.store.AddToHistory(phone, "assistant", response)

	// Fetch customer for notification (best-effort; nil is acceptable).
	enhanced, _ := r.cache.GetCustomerInfoEnhanced(ctx, phone)
	mktCopy := mkt
	go func() {
		var custPtr *client.Customer
		if enhanced != nil {
			c := enhanced.Customer
			custPtr = &c
		}
		r.notif.NotifyInternal(ctx, phone, custPtr, mktCopy, "bantuan_request", mktCopy.Number, nil)
	}()
	return nil
}

// handleWarehouseSearchRequest ports messageHandler.handleWarehouseSearchRequest
// (line 3875): sets WarehouseSearchRequested flag and presents the two-choice
// marketing selection menu.
func (r *Router) handleWarehouseSearchRequest(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User

	customerName := "Pelanggan"
	if c, err := r.cache.GetCustomerInfo(ctx, phone); err == nil && c != nil && c.Nama != "" {
		customerName = c.Nama
	}

	r.mu.Lock()
	ac := r.activeConvs[phone]
	if ac == nil {
		ac = &ActiveConv{}
		r.activeConvs[phone] = ac
	}
	ac.WarehouseSearchRequested = true
	r.mu.Unlock()

	response := fmt.Sprintf(`🏪 *Pencarian Gudang Lain*

Baik %s, saya akan membantu mencarikan stok di gudang cabang lain.

Untuk memproses permintaan Anda, kami perlu mengetahui region/wilayah Anda agar dapat menghubungkan dengan tim marketing yang tepat.

📍 *Pilih tim marketing Anda:*
1️⃣ Celvin (0812-8830-9688) - Jawa Barat, Jawa Tengah, Kalimantan, Riau, Jawa Timur, Palembang, Jambi, Bangka Belitung
2️⃣ Puput (0812-8298-3305) - Sulawesi, Sumatra, Papua, Maluku, NTT, NTB, Bali, Lampung

💬 Ketik nomor pilihan Anda (1-2)`, customerName)

	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}

// handleMarketingSelectionForWarehouse ports line 3975.
func (r *Router) handleMarketingSelectionForWarehouse(ctx context.Context, evt *events.Message, selection string) error {
	phone := evt.Info.Sender.User

	warehouseTeams := map[string]notification.MarketingInfo{
		"1": {Number: "6281288309688", Name: "Celvin"},
		"2": {Number: "6281282983305", Name: "Puput"},
	}
	mkt, ok := warehouseTeams[selection]
	if !ok {
		msg := "❌ Pilihan tidak valid. Silakan pilih 1 atau 2."
		r.reply(ctx, evt, msg)
		return r.store.AddToHistory(phone, "assistant", msg)
	}

	r.mu.Lock()
	if ac := r.activeConvs[phone]; ac != nil {
		ac.WarehouseSearchRequested = false
	}
	r.mu.Unlock()

	displayNum := notification.FormatDisplayNumber(mkt.Number)
	response := fmt.Sprintf(`✅ *Permintaan Pencarian Gudang Diterima*

Terima kasih! Permintaan pencarian stok di gudang lain Anda telah diteruskan ke tim marketing %s.

Tim marketing akan segera menghubungi Anda untuk membantu mencari stok yang dibutuhkan di gudang cabang lain.

📞 *Kontak Marketing:* %s (%s)

💬 Silakan lanjutkan pencarian produk atau ketik *bantuan* jika memerlukan bantuan lainnya.`, mkt.Name, mkt.Name, displayNum)

	r.reply(ctx, evt, response)
	_ = r.store.AddToHistory(phone, "assistant", response)

	enhanced, _ := r.cache.GetCustomerInfoEnhanced(ctx, phone)
	mktCopy := mkt
	go func() {
		var custPtr *client.Customer
		if enhanced != nil {
			c := enhanced.Customer
			custPtr = &c
		}
		r.notif.NotifyInternal(ctx, phone, custPtr, mktCopy, "warehouse_search_request", mktCopy.Number, nil)
	}()
	return nil
}

// handlePriceQuestion ports messageHandler.handlePriceQuestion (line 3690):
// if there's a single last result show its price; if multiple ask which;
// if none ask for a product code or name.
func (r *Router) handlePriceQuestion(ctx context.Context, evt *events.Message, messageBody string) error {
	phone := evt.Info.Sender.User

	customerName := "Pelanggan"
	if c, err := r.cache.GetCustomerInfo(ctx, phone); err == nil && c != nil && c.Nama != "" {
		customerName = c.Nama
	}

	r.mu.Lock()
	ac := r.activeConvs[phone]
	r.mu.Unlock()

	if ac != nil && len(ac.LastResults) > 0 {
		if len(ac.LastResults) == 1 {
			product := ac.LastResults[0]
			price, err := r.cache.GetCustomerPrice(ctx, product, phone)
			if err != nil {
				log.Printf("handlePriceQuestion: GetCustomerPrice error: %v", err)
				price = float64(product.HargaNum.NonCustomer)
			}
			response := fmt.Sprintf("💰 *Informasi Harga*\n\n*%s - %s*\nHarga: %s\n\nApakah Anda ingin memesan produk ini? Ketik \"pesan\" untuk melanjutkan atau \"bantuan\" untuk berbicara dengan tim marketing kami.",
				product.Kode, product.Nama, shared.FormatPrice(price))
			r.reply(ctx, evt, response)
			return r.store.AddToHistory(phone, "assistant", response)
		}
		limit := len(ac.LastResults)
		if limit > 10 {
			limit = 10
		}
		response := fmt.Sprintf("Saya menemukan beberapa produk dalam pencarian terakhir Anda. Produk mana yang ingin Anda ketahui harganya? Silakan ketik nomor (1-%d):\n\n", limit)
		for i, p := range ac.LastResults[:limit] {
			response += fmt.Sprintf("%d. *%s - %s*\n", i+1, p.Kode, p.Nama)
		}
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}

	// Try to detect a product code in the message and route to search.
	productCodeWithText := regexp.MustCompile(`\b\d{4}\b\s+\w+`)
	productCodeOnly := regexp.MustCompile(`\b\d{4}\b`)
	if productCodeWithText.MatchString(messageBody) || productCodeOnly.MatchString(messageBody) {
		return r.handleProductSearch(ctx, evt, messageBody)
	}

	response := fmt.Sprintf("Maaf %s, untuk mengetahui harga produk, silakan sebutkan kode atau nama produk yang Anda cari terlebih dahulu. Anda dapat mengetikkan nama produk atau kode produk 4 digit.", customerName)
	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}

// handleStockQuestion ports messageHandler.handleStockQuestion (line 3767).
func (r *Router) handleStockQuestion(ctx context.Context, evt *events.Message, messageBody string) error {
	phone := evt.Info.Sender.User

	customerName := "Pelanggan"
	if c, err := r.cache.GetCustomerInfo(ctx, phone); err == nil && c != nil && c.Nama != "" {
		customerName = c.Nama
	}

	r.mu.Lock()
	ac := r.activeConvs[phone]
	r.mu.Unlock()

	if ac != nil && len(ac.LastResults) > 0 {
		if len(ac.LastResults) == 1 {
			product := ac.LastResults[0]
			var stockStatus string
			if product.Stok > 0 {
				stockStatus = fmt.Sprintf("✅ Tersedia (%d unit)", product.Stok)
			} else {
				stockStatus = "❌ Stok habis"
			}
			response := fmt.Sprintf("📦 *Informasi Stok*\n\n*%s - %s*\nStatus: %s\n\nApakah Anda ingin memesan produk ini? Ketik \"pesan\" untuk melanjutkan atau \"bantuan\" untuk berbicara dengan tim marketing kami.",
				product.Kode, product.Nama, stockStatus)
			r.reply(ctx, evt, response)
			return r.store.AddToHistory(phone, "assistant", response)
		}
		limit := len(ac.LastResults)
		if limit > 10 {
			limit = 10
		}
		response := fmt.Sprintf("Saya menemukan beberapa produk dalam pencarian terakhir Anda. Produk mana yang ingin Anda ketahui stoknya? Silakan ketik nomor (1-%d):\n\n", limit)
		for i, p := range ac.LastResults[:limit] {
			response += fmt.Sprintf("%d. *%s - %s*\n", i+1, p.Kode, p.Nama)
		}
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}

	productCodeOnly := regexp.MustCompile(`\b\d{4}\b`)
	if productCodeOnly.MatchString(messageBody) {
		return r.handleProductSearch(ctx, evt, messageBody)
	}

	response := fmt.Sprintf("Maaf %s, untuk mengetahui stok produk, silakan sebutkan kode atau nama produk yang Anda cari terlebih dahulu. Anda dapat mengetikkan nama produk atau kode produk 4 digit.", customerName)
	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}

// handleLocationQuestion ports messageHandler.handleLocationQuestion (line 3836).
func (r *Router) handleLocationQuestion(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User

	customerName := "Pelanggan"
	if c, err := r.cache.GetCustomerInfo(ctx, phone); err == nil && c != nil && c.Nama != "" {
		customerName = c.Nama
	}

	alamat := os.Getenv("ALAMAT_KANTOR")
	if alamat == "" {
		alamat = "Jl. Mangga Besar VIII No.39B, Jakarta Barat"
	}
	alamatFormatted := strings.ReplaceAll(alamat, ", ", ",\n")
	telp := os.Getenv("TELPON_KANTOR")
	if telp == "" {
		telp = "021-6231-8301"
	}
	email := os.Getenv("EMAIL_KANTOR")
	if email == "" {
		email = "oceanbearings@gmail.com"
	}

	response := fmt.Sprintf(`🏢 *Informasi Lokasi Kami*

Halo %s, berikut adalah informasi lokasi kami:

📍 *Alamat:*
%s

☎️ *Telepon:* %s
📧 *Email:* %s

Jam operasional kami adalah Senin-Jumat (08.00-17.00) dan Sabtu (08.00-15.00).

Ada yang bisa kami bantu lainnya?`, customerName, alamatFormatted, telp, email)

	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}

// handleMarketingQuestion ports messageHandler.handleMarketingQuestion (line 4352):
// looks up the customer's stored region and shows the appropriate contact.
func (r *Router) handleMarketingQuestion(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User

	var region string
	_, _ = r.store.Get(phone, "region", &region)
	if region == "" {
		region = "jakarta"
	}

	mktInfo := notification.GetMarketingInfo(region)
	response := "📞 *Informasi Kontak Marketing*\n\n"
	response += fmt.Sprintf("Untuk wilayah *%s*, Anda dapat menghubungi:\n\n", strings.ToUpper(region))
	response += fmt.Sprintf("👨‍💼 *%s*\n", mktInfo.Name)
	response += fmt.Sprintf("📱 WhatsApp: %s\n\n", notification.FormatDisplayNumber(mktInfo.Number))
	response += `Apakah Anda ingin saya menghubungkan Anda dengan marketing kami sekarang? Ketik *YA* untuk konfirmasi.`

	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}

// handlePPNQuestion ports messageHandler.handlePPNQuestion (line 4404).
func (r *Router) handlePPNQuestion(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User
	response := "💰 *Informasi PPN*\n\nHarga yang ditampilkan belum termasuk PPN.\n\nUntuk informasi harga final termasuk PPN dan detail lainnya, silakan hubungi tim marketing kami atau ketik '/marketing'."
	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}
