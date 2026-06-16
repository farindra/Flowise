package router

import (
	"context"
	"fmt"
	"log"

	"go.mau.fi/whatsmeow/types/events"

	"wa-gateway-service/internal/client"
	"wa-gateway-service/internal/shared"
)

// makeCartSummary ports messageHandler.makeCartSummary (line 2659): formats
// the cart as a WhatsApp message using formatCurrency. Pure function — no I/O.
func makeCartSummary(cart []shared.CartItem) string {
	if len(cart) == 0 {
		return "🛒 Keranjang belanja Anda masih kosong."
	}

	summary := "🛒 *KERANJANG BELANJA ANDA*\n\n"
	var total float64

	for i, item := range cart {
		harga := item.Harga
		subtotal := harga * float64(item.Quantity)
		total += subtotal

		summary += fmt.Sprintf("%d. *%s - %s*\n", i+1, item.Kode, item.Nama)
		summary += fmt.Sprintf("   Jumlah: %d\n", item.Quantity)
		summary += fmt.Sprintf("   Harga: %s\n", shared.FormatCurrency(harga))
		summary += fmt.Sprintf("   Subtotal: %s\n\n", shared.FormatCurrency(subtotal))
	}

	summary += fmt.Sprintf("💰 *Total: %s*\n\n", shared.FormatCurrency(total))
	summary += "   *notes : *Harga belum PPN*\n\n"
	summary += "📝 *Opsi Keranjang:*\n"
	summary += "• Ketik \"checkout\" untuk melanjutkan pemesanan\n"
	summary += "• Ketik \"hapus [nomor]\" untuk menghapus produk (contoh: hapus 1)\n"
	summary += "• Ketik \"ubah [nomor] [jumlah]\" untuk mengubah jumlah (contoh: ubah 1 5)\n"
	summary += "• Ketik \"kembali\" untuk kembali ke pencarian produk"

	return summary
}

// loadCart reads the cart from the store for a phone number.
func (r *Router) loadCart(phone string) []shared.CartItem {
	var cart []shared.CartItem
	_, _ = r.store.Get(phone, "cart", &cart)
	if cart == nil {
		cart = []shared.CartItem{}
	}
	return cart
}

// saveCart writes the cart to the store.
func (r *Router) saveCart(phone string, cart []shared.CartItem) error {
	return r.store.Set(phone, "cart", cart)
}

// handleCartCommand ports messageHandler.handleCartCommand (line 1939).
// This replaces the 3b stub in commands.go.
func (r *Router) handleCartCommand(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User

	customerName := "Pelanggan"
	if c, err := r.cache.GetCustomerInfo(ctx, phone); err == nil && c != nil && c.Nama != "" {
		customerName = c.Nama
	}

	cart := r.loadCart(phone)

	if len(cart) == 0 {
		response := fmt.Sprintf("🛒 Keranjang belanja Anda masih kosong, %s.\n\nUntuk menambahkan produk, silakan cari produk terlebih dahulu.", customerName)
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}

	summary := makeCartSummary(cart)
	r.reply(ctx, evt, summary)
	return r.store.AddToHistory(phone, "assistant", summary)
}

// handleAddToCart ports messageHandler.handleAddToCart (line 2585).
// Called from number-selection in 3g; exposed here so it is available before 3g.
func (r *Router) handleAddToCart(ctx context.Context, evt *events.Message, product client.Product, quantity int) error {
	phone := evt.Info.Sender.User

	if product.Stok <= 0 {
		msg := fmt.Sprintf("Maaf, produk %s - %s sedang tidak tersedia (stok habis).", product.Kode, product.Nama)
		r.reply(ctx, evt, msg)
		return r.store.AddToHistory(phone, "assistant", msg)
	}

	customer, _ := r.cache.GetCustomerDetailWithIsland(ctx, phone)

	cart := r.loadCart(phone)

	// Check if product already in cart.
	found := false
	for i, item := range cart {
		if item.Kode == product.Kode {
			cart[i].Quantity += quantity
			if int64(cart[i].Quantity) > product.Stok {
				cart[i].Quantity = int(product.Stok)
			}
			found = true
			break
		}
	}

	if !found {
		finalPrice, err := r.cache.GetCustomerPrice(ctx, product, phone)
		if err != nil {
			log.Printf("handleAddToCart: GetCustomerPrice error for %s: %v", phone, err)
			finalPrice = float64(product.HargaNum.NonCustomer)
		}
		cart = append(cart, shared.CartItem{
			Kode:     product.Kode,
			Nama:     product.Nama,
			Harga:    finalPrice,
			Quantity: quantity,
			Stok:     product.Stok,
		})
	}

	if err := r.saveCart(phone, cart); err != nil {
		log.Printf("handleAddToCart: saveCart error for %s: %v", phone, err)
	}

	response := fmt.Sprintf("✅ Produk *%s - %s* telah ditambahkan ke keranjang belanja.\n\n📦 Produk yang ditambahkan:\n1. %s x%d\n\n💬 Pilihan selanjutnya:\n• Ketik \"/cart\" untuk melihat keranjang\n• Ketik \"checkout\" untuk melanjutkan pemesanan\n• Lanjutkan mencari produk lain",
		product.Kode, product.Nama, product.Kode, quantity)
	r.reply(ctx, evt, response)
	_ = r.store.AddToHistory(phone, "assistant", response)

	// Show cart summary after adding, mirroring Node behaviour.
	var cartForSummary []shared.CartItem
	if customer != nil {
		_ = customer // customer available if needed for future pricing display
	}
	cartForSummary = cart
	summary := makeCartSummary(cartForSummary)
	r.reply(ctx, evt, summary)
	return r.store.AddToHistory(phone, "assistant", summary)
}

// handleUpdateCartQuantity ports messageHandler.handleUpdateCartQuantity
// (line 2705). index is 0-based (caller converts from user-visible 1-based).
func (r *Router) handleUpdateCartQuantity(ctx context.Context, evt *events.Message, index, quantity int) error {
	phone := evt.Info.Sender.User

	cart := r.loadCart(phone)

	if index < 0 || index >= len(cart) {
		response := "Maaf, nomor produk tidak valid. Silakan periksa kembali nomor produk di keranjang belanja Anda dengan mengetik */cart*."
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}
	if quantity <= 0 {
		response := "Maaf, jumlah produk harus lebih dari 0. Silakan coba lagi."
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}

	product := cart[index]
	cart[index].Quantity = quantity

	if err := r.saveCart(phone, cart); err != nil {
		log.Printf("handleUpdateCartQuantity: saveCart error for %s: %v", phone, err)
	}

	cartSummary := makeCartSummary(cart)
	response := fmt.Sprintf("✅ Jumlah *%s - %s* berhasil diubah menjadi *%d*.\n\n%s",
		product.Kode, product.Nama, quantity, cartSummary)
	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}

// handleBackToSearch ports messageHandler.handleBackToSearch (line 2763):
// clears lastResults/lastQuery/selectedProduct from the active conversation.
func (r *Router) handleBackToSearch(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User

	r.mu.Lock()
	if ac := r.activeConvs[phone]; ac != nil {
		ac.LastResults = nil
	}
	r.mu.Unlock()

	response := "✅ Kembali ke pencarian produk.\n\n" +
		"🔍 *Cara Mencari Produk:*\n" +
		"• Ketik kode angka produk (contoh: \"6224\" atau \"6319\")\n" +
		"• Gunakan format \"^ [nama merek]\" untuk pencarian lebih akurat (contoh: \"^ SKF\")\n" +
		"• Kirim foto bearing untuk identifikasi otomatis"
	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}

// handleRemoveFromCart ports messageHandler.handleRemoveFromCart (line 2799).
// index is 0-based.
func (r *Router) handleRemoveFromCart(ctx context.Context, evt *events.Message, index int) error {
	phone := evt.Info.Sender.User

	cart := r.loadCart(phone)

	if index < 0 || index >= len(cart) {
		response := "Maaf, nomor produk tidak valid. Silakan periksa kembali nomor produk di keranjang belanja Anda dengan mengetik */cart*."
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}

	removed := cart[index]
	cart = append(cart[:index], cart[index+1:]...)

	if err := r.saveCart(phone, cart); err != nil {
		log.Printf("handleRemoveFromCart: saveCart error for %s: %v", phone, err)
	}

	var afterMsg string
	if len(cart) > 0 {
		afterMsg = fmt.Sprintf("🛒 Anda masih memiliki %d produk di keranjang.\n\nKetik */cart* untuk melihat keranjang belanja yang diperbarui.", len(cart))
	} else {
		afterMsg = "🛒 Keranjang belanja Anda sekarang kosong."
	}

	response := fmt.Sprintf("✅ Produk *%s - %s* telah dihapus dari keranjang belanja.\n\n%s",
		removed.Kode, removed.Nama, afterMsg)
	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}
