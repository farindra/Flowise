package router

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types/events"

	"wa-gateway-service/internal/notification"
	"wa-gateway-service/internal/state"
)

// handleCommand ports messageHandler.handleCommand (line 315-325): dispatches
// slash commands to their handlers.
func (r *Router) handleCommand(ctx context.Context, evt *events.Message, command string) error {
	phone := evt.Info.Sender.User
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil
	}
	cmdName := strings.ToLower(parts[0])

	switch cmdName {
	case "/help":
		return r.handleHelp(ctx, evt)
	case "/status":
		return r.handleStatusCommand(ctx, evt)
	case "/cart":
		return r.handleCartCommand(ctx, evt)
	case "/marketing":
		return r.handleMarketingCommand(ctx, evt)
	case "/reset":
		return r.handleResetCommand(ctx, evt)
	case "/katalog":
		return r.handleKatalogCommand(ctx, evt)
	case "/owner":
		return r.handleOwnerCommand(ctx, evt, parts)
	default:
		msg := "❌ Perintah tidak dikenal. Ketik \"bantuan\" untuk mendapatkan bantuan dari tim marketing kami."
		r.reply(ctx, evt, msg)
		_ = r.store.AddToHistory(phone, "assistant", msg)
	}
	return nil
}

// handleHelp ports messageHandler.handleHelp (line 1801-1862).
func (r *Router) handleHelp(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User
	customerName := "Customer"
	if c, err := r.cache.GetCustomerInfo(ctx, phone); err == nil && c != nil && c.Nama != "" {
		customerName = c.Nama
	}

	telp := os.Getenv("TELPON_KANTOR")
	if telp == "" {
		telp = "021-6231-8301"
	}

	response := fmt.Sprintf(`📋 *Bantuan Bobi*

Halo %s, berikut adalah cara menggunakan layanan kami:

🔍 *Cara Mencari Produk:*
• Ketik nama produk (contoh: "bearing toyota")
• Gunakan format "^ nama produk" untuk pencarian lebih akurat (contoh: "^ bearing toyota")• Ketik kode produk (contoh: "6224.FAG" atau "6319.SKF")
• Kirim foto bearing untuk identifikasi otomatis

📦 *Pencarian Multi-Produk:*
• Pisahkan dengan koma: "6203, 6204, 6205"
• Gunakan "dan": "6203 dan 6204"
• Format list bernomor / bullet points:
  1. 6203
  2. 6204 zz
  3. 6205 cm koyo
• Pisahkan dengan garis: "6203|6204|6205"

⌨️ *Perintah:*
• /help - Tampilkan bantuan
• /status - Cek status akun
• /cart - Lihat keranjang belanja
• /katalog - Download katalog produk lengkap (CSV)
• /owner [query] - Owner Assistant (khusus owner)
• /marketing - Lihat daftar marketing
• Ketik "bantuan" untuk menghubungi tim marketing

⏱️ *TERAKHIR MOHON DITUNGGU MINIMAL 20 DETIK UNTUK DIBALAS OLEH CHAT BOT*

📞 *Kontak:*
• Nomor khusus untuk bicara dengan manusia (bukan bot): 081315209620
• Jika membutuhkan bantuan lebih lanjut, hubungi tim kami di %s.`, customerName, telp)

	r.reply(ctx, evt, response)
	_ = r.store.AddToHistory(phone, "assistant", response)
	return nil
}

// handleStatusCommand ports messageHandler.handleStatusCommand (line 1866-1927).
func (r *Router) handleStatusCommand(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User
	customer, err := r.cache.GetCustomerInfo(ctx, phone)
	if err != nil {
		log.Printf("getCustomerInfo error for %s: %v", phone, err)
	}

	response := "📊 *Status Akun*\n\n"

	if customer != nil && customer.Nama != "" {
		response += "✅ *Terdaftar*\n"
		response += fmt.Sprintf("👤 *Nama:* %s\n", customer.Nama)

		if customer.IsVip && customer.VipInfo != nil && customer.VipInfo.Status != "blacklist" {
			response += "⭐ *Status VIP*\n"
		}
		if customer.Marketing != "" {
			response += fmt.Sprintf("👨‍💼 *Marketing:* %s\n", customer.Marketing)
		}
		if customer.Nomor != "" {
			response += fmt.Sprintf("📱 *Nomor Terdaftar:* %s\n", formatPhoneDisplay(customer.Nomor))
		}
	} else {
		response += "❌ *Belum Terdaftar*\n"
		response += "\nAnda belum terdaftar sebagai pelanggan.\n"
	}

	response += "\n🤖 *Versi Bot:* 2.0.0\n"
	response += "📞 *Kontak Kami*\n"
	response += "☎️ Telepon: 021-623-18301\n"
	response += "📱 WhatsApp 1: 0812-8830-9688\n"
	response += "📱 WhatsApp 2: 0812-8298-3305\n"
	response += "📧 Email: info@oceanbearings.co.id"
	response += "\n🏢 *Alamat:* Jl. Mangga Besar VIII No.39B, RT.11/RW.1, Kota Tua, Kec. Taman Sari, Kota Jakarta Barat, Daerah Khusus Ibukota Jakarta 11150\n"

	r.reply(ctx, evt, response)
	_ = r.store.AddToHistory(phone, "assistant", response)
	return nil
}

// handleResetCommand ports messageHandler.handleResetCommand (line 1972-2013).
func (r *Router) handleResetCommand(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User
	customerName := "Pelanggan"
	if c, err := r.cache.GetCustomerInfo(ctx, phone); err == nil && c != nil && c.Nama != "" {
		customerName = c.Nama
	}

	_ = r.store.SetUserState(phone, state.StateIdle)
	r.deleteActiveConv(phone)
	_ = r.store.Set(phone, "cart", nil)
	if err := r.store.ClearUserData(phone); err != nil {
		log.Printf("ClearUserData error for %s: %v", phone, err)
	}

	response := fmt.Sprintf("✅ *Percakapan Direset*\n\nHalo %s, percakapan Anda telah direset dan keranjang belanja telah dikosongkan. Anda dapat memulai percakapan baru dengan:\n\n• Mencari produk dengan mengetik nama atau kode produk\n• Mengirim gambar produk untuk identifikasi\n• Mengetik \"bantuan\" untuk berbicara dengan tim marketing\n• Mengetik \"/help\" untuk melihat panduan lengkap", customerName)

	r.reply(ctx, evt, response)
	_ = r.store.AddToHistory(phone, "assistant", response)
	return nil
}

// handleCartCommand full implementation is in cart.go.

// handleMarketingCommand ports messageHandler.handleMarketingCommand (line
// 2843): groups MARKETING_MAP regions by marketing name and formats the list.
func (r *Router) handleMarketingCommand(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User

	customerName := "Customer"
	if c, err := r.cache.GetCustomerInfo(ctx, phone); err == nil && c != nil && c.Nama != "" {
		customerName = c.Nama
	}

	// Group regions by marketing name (mirrors marketingGroups in Node).
	type mktGroup struct {
		number  string
		regions []string
	}
	groups := map[string]*mktGroup{}
	for _, region := range notification.AvailableRegions() {
		info := notification.GetMarketingInfo(region)
		g, ok := groups[info.Name]
		if !ok {
			g = &mktGroup{number: notification.FormatDisplayNumber(info.Number)}
			groups[info.Name] = g
		}
		g.regions = append(g.regions, notification.TitleCase(region))
	}

	response := fmt.Sprintf("📋 *Daftar Marketing Ocean Bearing*\n\nHalo %s, berikut adalah daftar marketing kami beserta region yang ditangani:\n\n", customerName)

	// Stable output order: Celvin first, Puput second (matches Node object
	// iteration order which happens to be insertion order — Celvin was inserted
	// first in MARKETING_MAP).
	order := []string{}
	seen := map[string]bool{}
	for _, region := range notification.AvailableRegions() {
		name := notification.GetMarketingInfo(region).Name
		if !seen[name] {
			seen[name] = true
			order = append(order, name)
		}
	}
	for _, name := range order {
		g := groups[name]
		response += fmt.Sprintf("👨‍💼 *%s*\n📱 Kontak: %s\n🌏 Region: %s\n\n",
			name, g.number, strings.Join(g.regions, ", "))
	}

	response += `Untuk bantuan lebih lanjut, silakan ketik "bantuan" untuk terhubung dengan marketing yang sesuai dengan region Anda.`

	r.reply(ctx, evt, response)
	_ = r.store.AddToHistory(phone, "assistant", response)
	return nil
}

// sendErrorMessage ports messageHandler.sendErrorMessage (line 4326-4347).
func (r *Router) sendErrorMessage(ctx context.Context, evt *events.Message) {
	phone := evt.Info.Sender.User
	response := "Maaf, terjadi kesalahan sistem. 😔\n\nSilakan coba:\n✅ Ketik ulang pesan Anda\n✅ Ketik nama atau kode produk yang ingin dicari\n✅ Ketik /help untuk bantuan\n\nJika masalah berlanjut, hubungi marketing kami."
	r.reply(ctx, evt, response)
	_ = r.store.AddToHistory(phone, "assistant", response)
}

// formatPhoneDisplay converts a E.164 Indonesian phone (62812...) to local
// format (0812...) for display, matching customerService.formatPhoneNumber.
func formatPhoneDisplay(nomor string) string {
	nomor = strings.TrimSpace(nomor)
	switch {
	case strings.HasPrefix(nomor, "+62"):
		return "0" + nomor[3:]
	case strings.HasPrefix(nomor, "62"):
		return "0" + nomor[2:]
	}
	return nomor
}

// handleOwnerCommand routes the owner to the Owner Assistant Flowise chatflow.
// Usage: /owner [query] — if query is provided, send immediately; otherwise show help.
// Only accessible by phones listed in OWNER_PHONES env var.
func (r *Router) handleOwnerCommand(ctx context.Context, evt *events.Message, parts []string) error {
	phone := evt.Info.Sender.User

	if !r.ownerPhones[phone] {
		msg := "❌ Fitur ini hanya tersedia untuk owner Ocean Bearings."
		r.reply(ctx, evt, msg)
		return r.store.AddToHistory(phone, "assistant", msg)
	}

	if r.ownerFlowise == nil {
		msg := "⚠️ Owner Assistant belum dikonfigurasi. Hubungi admin untuk setup OWNER_CHATFLOW_ID."
		r.reply(ctx, evt, msg)
		return r.store.AddToHistory(phone, "assistant", msg)
	}

	query := strings.TrimSpace(strings.TrimPrefix(strings.Join(parts, " "), "/owner"))
	if query == "" {
		msg := "👋 *Owner Assistant aktif*\n\nKamu bisa tanya langsung, contoh:\n" +
			"• `/owner stok kosong apa aja?`\n" +
			"• `/owner harga supplier FAG terbaru`\n" +
			"• `/owner ringkasan perdagangan bulan ini`\n" +
			"• `/owner buat draft P.O. untuk stok kosong FAG`\n\n" +
			"Atau kirim pertanyaan langsung — nomor ini sudah terdaftar sebagai owner."
		r.reply(ctx, evt, msg)
		return r.store.AddToHistory(phone, "assistant", msg)
	}

	r.reply(ctx, evt, "⏳ Memproses permintaan owner...")

	go func() {
		bgCtx := context.Background()
		answer := r.ownerFlowise.AskDirect(bgCtx, query, "owner-wa-"+phone)
		if answer == "" {
			answer = "❌ Owner Assistant tidak merespons. Coba lagi."
		}
		r.reply(bgCtx, evt, answer)
		_ = r.store.AddToHistory(phone, "assistant", answer)
	}()
	return nil
}

// handleKatalogCommand sends the full product catalog as a CSV file.
// The fetch + upload is done in a goroutine so the message queue is not blocked.
func (r *Router) handleKatalogCommand(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User

	r.reply(ctx, evt, "⏳ Sedang menyiapkan katalog produk (36rb+ item)... mohon tunggu sebentar.")

	go func() {
		bgCtx := context.Background()

		data, err := fetchURL(bgCtx, r.catalogExportURL)
		if err != nil {
			log.Printf("handleKatalogCommand: fetch catalog error for %s: %v", phone, err)
			msg := "❌ Gagal mengambil katalog. Silakan coba lagi."
			r.reply(bgCtx, evt, msg)
			_ = r.store.AddToHistory(phone, "assistant", msg)
			return
		}

		filename := fmt.Sprintf("katalog-ob-trade-%s.csv", time.Now().Format("2006-01-02"))
		if err := r.replyDocument(bgCtx, evt, data, filename, "text/csv"); err != nil {
			log.Printf("handleKatalogCommand: send document error for %s: %v", phone, err)
			msg := "❌ Gagal mengirim file katalog. Silakan coba lagi."
			r.reply(bgCtx, evt, msg)
			_ = r.store.AddToHistory(phone, "assistant", msg)
			return
		}

		msg := "✅ Katalog produk Ocean Bearings berhasil dikirim!\n\n📊 File berisi seluruh produk dengan harga.\n⚠️ *Catatan:* Harga di katalog adalah harga umum. Untuk harga customer/khusus, silakan hubungi marketing kami."
		r.reply(bgCtx, evt, msg)
		_ = r.store.AddToHistory(phone, "assistant", msg)
	}()

	return nil
}

// createdAtDisplay formats a creation timestamp string for display.
func createdAtDisplay(createdAt string) string {
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return createdAt
	}
	months := []string{
		"", "Januari", "Februari", "Maret", "April", "Mei", "Juni",
		"Juli", "Agustus", "September", "Oktober", "November", "Desember",
	}
	return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()], t.Year())
}
