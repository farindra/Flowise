package router

import (
	"context"
	"log"
	"strings"

	"go.mau.fi/whatsmeow/types/events"

	"wa-gateway-service/internal/notification"
	"wa-gateway-service/internal/state"
)

// startRegistration ports messageHandler.startRegistration (line 1600).
//
// The Node implementation immediately sets company="Perorangan" and
// region="jakarta" (old handleCompanyInput/handleRegionInput flow was removed),
// then tries an AI-generated welcome message and falls back to a hardcoded one.
// Node tries aiService.generateResponse (free-form prompt) first, falls back
// to the hardcoded message below. We use GenerateNatural (isGreeting=true)
// which matches the same spirit; if AI unavailable, the fallback fires.
func (r *Router) startRegistration(ctx context.Context, evt *events.Message) error {
	phone := evt.Info.Sender.User
	body := strings.TrimSpace(msgBody(evt))

	_ = r.store.Set(phone, "company", "Perorangan")
	_ = r.store.Set(phone, "region", "jakarta")
	_ = r.store.SetUserState(phone, state.StateIdle)

	response := r.generateNatural(ctx, body, phone, "", nil, true, true)
	if response == "" {
		response = "Selamat datang di Ocean Bearing 🌊\n\nAnda dapat langsung mencari produk sekarang.\n\nSilakan ketik nama atau kode produk yang ingin Anda cari (contoh: 6224 atau 6224.FAG)."
	}
	r.reply(ctx, evt, response)
	return r.store.AddToHistory(phone, "assistant", response)
}

// completeRegistration ports messageHandler.completeRegistration (line 1645).
// NOTE: this function has zero callers in the current Node codebase (the
// handleCompanyInput/handleRegionInput flow that led here was removed).
// Ported for completeness; if it is re-introduced in a future sub-phase the
// implementation is ready.
func (r *Router) completeRegistration(ctx context.Context, evt *events.Message, company, region string) error {
	phone := evt.Info.Sender.User

	mktInfo := notification.GetMarketingInfo(region)
	marketingName := mktInfo.Name
	marketingNumber := strings.Replace(mktInfo.Number, "62", "0", 1)

	_ = r.store.SetUserState(phone, state.StateIdle)

	// Try to look up existing customer by company name.
	customerInfo := "Harga yang ditampilkan adalah harga non-customer."
	existing, err := r.cache.GetCustomerByCompany(ctx, company)
	if err != nil {
		log.Printf("completeRegistration: GetCustomerByCompany error for %s: %v", phone, err)
	}
	if existing != nil && existing.Nama != "" {
		if existing.Marketing == "" {
			existing.Marketing = marketingName
		}
		if err := r.store.Set(phone, "customer", existing); err != nil {
			log.Printf("completeRegistration: store customer error for %s: %v", phone, err)
		}
		customerInfo = "Kami menemukan data Anda sebagai pelanggan kami dengan nama: " + existing.Nama + "."
	}

	response := "✅ Registrasi selesai. " + customerInfo +
		"\n\nMarketing: " + marketingName + " (" + marketingNumber + ")" +
		"\n\nSilakan langsung ketik nama atau kode produk untuk mencari ya."

	r.reply(ctx, evt, response)
	_ = r.store.AddToHistory(phone, "assistant", response)

	// Notify marketing — simple wrapper (no throttle), mirrors notifyMarketing call
	// at completeRegistration line 1688.
	go r.notif.Notify(ctx, phone, company, region)
	return nil
}
