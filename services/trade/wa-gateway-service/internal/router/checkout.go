package router

import (
	"context"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types/events"

	"wa-gateway-service/internal/client"
	"wa-gateway-service/internal/notification"
	"wa-gateway-service/internal/shared"
)

// CheckoutCustomer holds checkout-time customer data.
type CheckoutCustomer struct {
	Nama              string
	Region            string
	SelectedMarketing string // "Celvin" or "Puput"
}

// CheckoutData is stored in ActiveConv.Checkout during the checkout flow.
type CheckoutData struct {
	Items    []shared.CartItem
	Customer CheckoutCustomer
}

// handleCheckout ports messageHandler.handleCheckout (line 2905).
func (r *Router) handleCheckout(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User

	customer, _ := r.cache.GetCustomerInfo(ctx, phone)
	customerName := "Pelanggan"
	if customer != nil && customer.Nama != "" {
		customerName = customer.Nama
	}

	cart := r.loadCart(phone)
	if len(cart) == 0 {
		response := fmt.Sprintf("🛒 Keranjang belanja Anda masih kosong, %s.\n\nUntuk menambahkan produk, silakan cari produk terlebih dahulu.", customerName)
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}

	isRegistered := customer != nil && customer.Nama != "" && customer.Nama != "Pelanggan"

	var co CheckoutCustomer
	if customer != nil {
		co.Nama = customer.Nama
	}

	if !isRegistered {
		return r.askForCompanyNameForCheckout(ctx, evt, cart, co)
	}

	var region string
	r.store.Get(phone, "region", &region) //nolint:errcheck

	if region == "" {
		return r.askForRegionForCheckout(ctx, evt, cart, co)
	}

	co.Region = region
	return r.processCheckoutWithRegion(ctx, evt, cart, co)
}

// askForRegionForCheckout ports messageHandler.askForRegionForCheckout (line 2958).
func (r *Router) askForRegionForCheckout(ctx context.Context, evt *events.Message, items []shared.CartItem, customer CheckoutCustomer) error {
	phone := evt.Info.Sender.User

	r.mu.Lock()
	ac := r.activeConvs[phone]
	if ac == nil {
		ac = &ActiveConv{}
		r.activeConvs[phone] = ac
	}
	ac.State = "ASK_REGION_CHECKOUT"
	ac.Checkout = &CheckoutData{Items: items, Customer: customer}
	r.mu.Unlock()

	response := "🛒 *CHECKOUT PESANAN*\n\n" +
		"Untuk memproses pesanan Anda, kami perlu mengetahui region/wilayah Anda agar dapat menghubungkan dengan tim marketing yang tepat.\n\n" +
		"📍 *Pilih tim marketing Anda:*\n" +
		"1️⃣ Celvin (0812-8830-9688) - Jawa Barat, Jawa Tengah, Kalimantan, Riau, Jawa Timur, Palembang, Jambi, Bangka Belitung\n" +
		"2️⃣ Puput (0812-8298-3305) - Sulawesi, Sumatra, Papua, Maluku, NTT, NTB, Bali, Lampung\n\n" +
		"💬 Ketik nomor pilihan Anda (1-2)"

	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}

// askForMarketingForCheckout ports messageHandler.askForMarketingForCheckout (line 2993).
func (r *Router) askForMarketingForCheckout(ctx context.Context, evt *events.Message, items []shared.CartItem, customer CheckoutCustomer) error {
	phone := evt.Info.Sender.User

	r.mu.Lock()
	ac := r.activeConvs[phone]
	if ac == nil {
		ac = &ActiveConv{}
		r.activeConvs[phone] = ac
	}
	ac.State = "ASK_MARKETING_CHECKOUT"
	ac.Checkout = &CheckoutData{Items: items, Customer: customer}
	r.mu.Unlock()

	response := "🛒 *CHECKOUT PESANAN*\n\n" +
		"Silakan pilih tim marketing yang akan menangani pesanan Anda:\n\n" +
		"👨‍💼 *Tim Marketing:*\n" +
		"1️⃣ Celvin (0812-8830-9688)\n" +
		"2️⃣ Puput (0812-8298-3305)\n\n" +
		"💬 Ketik nomor pilihan Anda (1-2)"

	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}

// askForCompanyNameForCheckout ports messageHandler.askForCompanyNameForCheckout (line 3030).
func (r *Router) askForCompanyNameForCheckout(ctx context.Context, evt *events.Message, items []shared.CartItem, customer CheckoutCustomer) error {
	phone := evt.Info.Sender.User

	r.mu.Lock()
	ac := r.activeConvs[phone]
	if ac == nil {
		ac = &ActiveConv{}
		r.activeConvs[phone] = ac
	}
	ac.State = "ASK_COMPANY_NAME_CHECKOUT"
	ac.Checkout = &CheckoutData{Items: items, Customer: customer}
	r.mu.Unlock()

	response := "🛒 *CHECKOUT PESANAN*\n\n" +
		"Untuk melanjutkan checkout, mohon masukkan nama perusahaan atau nama Anda:\n\n" +
		"Contoh: PT Ocean Bearing atau Budi Santoso"

	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}

// handleCompanyNameInput ports messageHandler.handleCompanyNameInput (line 3066).
func (r *Router) handleCompanyNameInput(ctx context.Context, evt *events.Message, messageBody string) error {
	phone := evt.Info.Sender.User
	companyName := strings.TrimSpace(messageBody)

	if len([]rune(companyName)) < 3 {
		response := "❌ Nama perusahaan atau nama Anda terlalu pendek. Mohon masukkan nama yang valid."
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}

	r.mu.Lock()
	ac := r.activeConvs[phone]
	r.mu.Unlock()

	if ac == nil || ac.Checkout == nil {
		response := "❌ Data checkout tidak ditemukan. Silakan mulai checkout ulang dari keranjang."
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}

	r.mu.Lock()
	ac.Checkout.Customer.Nama = companyName
	ac.State = ""
	items := ac.Checkout.Items
	co := ac.Checkout.Customer
	r.mu.Unlock()

	_ = r.store.Set(phone, "company", companyName)

	if co.Region == "" {
		return r.askForRegionForCheckout(ctx, evt, items, co)
	}
	return r.processCheckoutWithRegion(ctx, evt, items, co)
}

// processCheckoutWithRegion ports messageHandler.processCheckoutWithRegion (line 3116):
// shows confirmation message and waits for "konfirmasi" / "batal".
func (r *Router) processCheckoutWithRegion(ctx context.Context, evt *events.Message, items []shared.CartItem, customer CheckoutCustomer) error {
	phone := evt.Info.Sender.User

	cart := r.loadCart(phone)
	if len(cart) == 0 {
		response := "🛒 Keranjang belanja Anda masih kosong. Silakan tambahkan produk terlebih dahulu."
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}

	var totalPrice float64
	for _, item := range cart {
		totalPrice += item.Harga * float64(item.Quantity)
	}

	response := "🛒 *KONFIRMASI CHECKOUT*\n\n"
	response += fmt.Sprintf("Anda akan melakukan checkout untuk %d produk dengan total %s.\n\n", len(cart), shared.FormatCurrency(totalPrice))
	response += "💬 *Pilihan:*\n"
	response += "• Ketik \"konfirmasi\" untuk melanjutkan checkout\n"
	response += "• Ketik \"batal\" untuk membatalkan checkout\n"

	r.reply(ctx, evt, response)
	_ = r.store.AddToHistory(phone, "assistant", response)

	r.mu.Lock()
	ac := r.activeConvs[phone]
	if ac == nil {
		ac = &ActiveConv{}
		r.activeConvs[phone] = ac
	}
	ac.State = "CONFIRM_CHECKOUT"
	ac.Checkout = &CheckoutData{Items: items, Customer: customer}
	r.mu.Unlock()

	return nil
}

// processConfirmedCheckout ports messageHandler.processConfirmedCheckout (line 3179):
// called when user types "konfirmasi" in CONFIRM_CHECKOUT state.
func (r *Router) processConfirmedCheckout(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User

	r.mu.Lock()
	ac := r.activeConvs[phone]
	r.mu.Unlock()

	if ac == nil || ac.Checkout == nil {
		response := "Maaf, data checkout tidak ditemukan. Silakan mulai proses checkout lagi dengan mengetik \"checkout\"."
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}

	r.mu.Lock()
	co := ac.Checkout.Customer
	items := ac.Checkout.Items
	r.mu.Unlock()

	if co.Region == "" {
		return r.askForRegionForCheckout(ctx, evt, items, co)
	}
	if co.SelectedMarketing == "" {
		return r.askForMarketingForCheckout(ctx, evt, items, co)
	}

	// Use live cart for accurate totals.
	cart := r.loadCart(phone)
	if len(cart) == 0 {
		cart = items
	}

	var totalPrice float64
	for _, item := range cart {
		totalPrice += item.Harga * float64(item.Quantity)
	}

	var mktInfo notification.MarketingInfo
	switch co.SelectedMarketing {
	case "Celvin":
		mktInfo = notification.MarketingInfo{Name: "Celvin", Number: "6281288309688"}
	case "Puput":
		mktInfo = notification.MarketingInfo{Name: "Puput", Number: "6281282983305"}
	default:
		return r.askForMarketingForCheckout(ctx, evt, items, co)
	}

	customerName := co.Nama
	if customerName == "" {
		customerName = "Pelanggan"
	}

	response := "✅ *PESANAN BERHASIL DIBUAT*\n\n"
	response += fmt.Sprintf("👤 *Customer:* %s\n", customerName)
	response += fmt.Sprintf("📱 *Phone:* %s\n\n", phone)
	response += "📦 *Detail Pesanan:*\n"
	for i, item := range cart {
		itemTotal := item.Harga * float64(item.Quantity)
		response += fmt.Sprintf("%d. %s x%d\n", i+1, item.Kode, item.Quantity)
		response += fmt.Sprintf("   %s\n", item.Nama)
		response += fmt.Sprintf("   %s\n\n", shared.FormatPrice(itemTotal))
	}
	response += "💰 *Ringkasan Harga:*\n"
	response += fmt.Sprintf("• *Total: %s*\n\n", shared.FormatPrice(totalPrice))
	response += fmt.Sprintf("📞 *Tim Marketing Anda:*\n%s - %s\n\n",
		mktInfo.Name, notification.FormatDisplayNumber(mktInfo.Number))
	response += "✅ Pesanan telah diteruskan ke tim marketing. Mereka akan segera menghubungi Anda untuk konfirmasi dan pengaturan pengiriman.\n\n"
	response += "🙏 Terima kasih telah berbelanja di Ocean Bearing!"

	r.reply(ctx, evt, response)
	_ = r.store.AddToHistory(phone, "assistant", response)

	// Notify marketing (non-blocking).
	var custPtr *client.Customer
	if c, err := r.cache.GetCustomerInfo(ctx, phone); err == nil && c != nil {
		cp := c.Customer
		custPtr = &cp
	}
	addData := &notification.NotifyAdditionalData{
		OrderItems: cart,
		TotalPrice: totalPrice,
	}
	go func() {
		r.notif.NotifyInternal(ctx, phone, custPtr, mktInfo, "checkout_request", mktInfo.Number, addData)
	}()

	// Clear cart and checkout state.
	_ = r.store.Set(phone, "cart", nil)
	r.mu.Lock()
	if ac := r.activeConvs[phone]; ac != nil {
		ac.Checkout = nil
		ac.State = ""
	}
	r.mu.Unlock()

	return nil
}

// handleCheckoutRegionInput ports messageHandler.handleCheckoutRegionInput (line 3336).
func (r *Router) handleCheckoutRegionInput(ctx context.Context, evt *events.Message, messageBody string) error {
	phone := evt.Info.Sender.User
	input := strings.TrimSpace(messageBody)

	regionByNum := map[string]string{
		"1": "jawa barat",
		"2": "sulawesi",
	}
	marketingByRegion := map[string]string{
		"jawa barat": "Celvin",
		"sulawesi":   "Puput",
	}

	var selectedRegion string
	if reg, ok := regionByNum[input]; ok {
		selectedRegion = reg
	} else {
		inputLower := strings.ToLower(input)
		if strings.Contains(inputLower, "celvin") || strings.Contains(inputLower, "1") {
			selectedRegion = "jawa barat"
		} else if strings.Contains(inputLower, "puput") || strings.Contains(inputLower, "2") {
			selectedRegion = "sulawesi"
		}
	}

	if selectedRegion == "" {
		response := "❌ Pilihan tidak valid. Silakan pilih nomor 1-2:\n\n" +
			"📍 *Pilih tim marketing Anda:*\n" +
			"1️⃣ Celvin (0812-8830-9688) - Jawa Barat, Jawa Tengah, Kalimantan, Riau, Jawa Timur, Palembang, Jambi, Bangka Belitung\n" +
			"2️⃣ Puput (0812-8298-3305) - Sulawesi, Sumatra, Papua, Maluku, NTT, NTB, Bali, Lampung\n\n" +
			"💬 Ketik nomor pilihan Anda (1-2)"
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}

	selectedMarketing := marketingByRegion[selectedRegion]

	r.mu.Lock()
	ac := r.activeConvs[phone]
	r.mu.Unlock()

	if ac == nil || ac.Checkout == nil {
		response := "❌ Data checkout tidak ditemukan. Silakan mulai checkout ulang dari keranjang."
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}

	r.mu.Lock()
	ac.Checkout.Customer.Region = selectedRegion
	ac.Checkout.Customer.SelectedMarketing = selectedMarketing
	ac.State = ""
	items := ac.Checkout.Items
	co := ac.Checkout.Customer
	r.mu.Unlock()

	return r.processCheckoutWithRegion(ctx, evt, items, co)
}

// handleMarketingSelectionForCheckout ports messageHandler.handleMarketingSelectionForCheckout (line 4027).
func (r *Router) handleMarketingSelectionForCheckout(ctx context.Context, evt *events.Message, selection string) error {
	phone := evt.Info.Sender.User

	type mktEntry struct{ Name, Number string }
	marketingTeams := map[string]mktEntry{
		"1": {Name: "Celvin", Number: "6281288309688"},
		"2": {Name: "Puput", Number: "6281282983305"},
	}
	selected, ok := marketingTeams[selection]
	if !ok {
		response := "❌ Pilihan tidak valid. Silakan pilih 1 atau 2."
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}

	r.mu.Lock()
	ac := r.activeConvs[phone]
	r.mu.Unlock()

	if ac == nil || ac.Checkout == nil {
		response := "❌ Data checkout tidak ditemukan. Silakan mulai checkout ulang."
		r.reply(ctx, evt, response)
		return r.store.AddToHistory(phone, "assistant", response)
	}

	r.mu.Lock()
	ac.Checkout.Customer.SelectedMarketing = selected.Name
	ac.State = ""
	items := ac.Checkout.Items
	co := ac.Checkout.Customer
	r.mu.Unlock()

	return r.processCheckoutWithRegion(ctx, evt, items, co)
}
