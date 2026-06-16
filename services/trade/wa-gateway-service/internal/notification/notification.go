// Package notification ports the marketing notification logic from
// messageHandler.js: static MARKETING_MAP, getMarketingInfo, notifyMarketing,
// and notifyMarketingInternal.
package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"wa-gateway-service/internal/client"
	"wa-gateway-service/internal/shared"
)

// MarketingInfo holds a single marketing contact entry from the static map.
type MarketingInfo struct {
	Number string // E.164 without +, e.g. "6281288309688"
	Name   string
}

// marketingMap is the verbatim port of MessageHandler.MARKETING_MAP
// (messageHandler.js line 2138-2212). Keys are lower-cased region names.
var marketingMap = map[string]MarketingInfo{
	"default":         {Number: "6281288309688", Name: "Tim Marketing"},
	"jawa barat":      {Number: "6281288309688", Name: "Celvin"},
	"jawa tengah":     {Number: "6281288309688", Name: "Celvin"},
	"kalimantan":      {Number: "6281288309688", Name: "Celvin"},
	"riau":            {Number: "6281288309688", Name: "Celvin"},
	"kepulauan riau":  {Number: "6281288309688", Name: "Celvin"},
	"jawa timur":      {Number: "6281288309688", Name: "Celvin"},
	"palembang":       {Number: "6281288309688", Name: "Celvin"},
	"jambi":           {Number: "6281288309688", Name: "Celvin"},
	"bangka belitung": {Number: "6281288309688", Name: "Celvin"},
	"sulawesi":        {Number: "6281282983305", Name: "Puput"},
	"sumatra":         {Number: "6281282983305", Name: "Puput"},
	"papua":           {Number: "6281282983305", Name: "Puput"},
	"maluku":          {Number: "6281282983305", Name: "Puput"},
	"ntt":             {Number: "6281282983305", Name: "Puput"},
	"ntb":             {Number: "6281282983305", Name: "Puput"},
	"bali":            {Number: "6281282983305", Name: "Puput"},
	"lampung":         {Number: "6281282983305", Name: "Puput"},
}

// AvailableRegions returns the non-default region keys from marketingMap,
// matching MessageHandler.availableRegions.
func AvailableRegions() []string {
	regions := make([]string, 0, len(marketingMap)-1)
	for k := range marketingMap {
		if k != "default" {
			regions = append(regions, k)
		}
	}
	return regions
}

// GetMarketingInfo returns the MarketingInfo for a region string, using the
// same lookup precedence as getMarketingInfo (line 2221):
// exact match → partial match → default.
func GetMarketingInfo(region string) MarketingInfo {
	if region == "" {
		return marketingMap["default"]
	}
	norm := strings.ToLower(strings.TrimSpace(region))
	if info, ok := marketingMap[norm]; ok {
		return info
	}
	for key, info := range marketingMap {
		if key == "default" {
			continue
		}
		if strings.Contains(norm, key) || strings.Contains(key, norm) {
			return info
		}
	}
	return marketingMap["default"]
}

// GetAllMarketingNumbers ports getAllMarketingNumbers (line 5223): collects
// all unique phone numbers from the static map plus any additional numbers
// from the MARKETING_MAP env variable (JSON object, values are phone strings).
func GetAllMarketingNumbers() []string {
	seen := map[string]bool{}
	var nums []string

	for _, info := range marketingMap {
		if info.Number != "" && !seen[info.Number] {
			seen[info.Number] = true
			nums = append(nums, info.Number)
		}
	}

	// Optional env override / additions.
	if raw := os.Getenv("MARKETING_MAP"); raw != "" {
		var extra map[string]string
		if err := json.Unmarshal([]byte(raw), &extra); err != nil {
			log.Printf("notification: error parsing MARKETING_MAP env: %v", err)
		} else {
			for _, number := range extra {
				number = strings.Replace(number, "@c.us", "", 1)
				if number != "" && !seen[number] {
					seen[number] = true
					nums = append(nums, number)
				}
			}
		}
	}
	return nums
}

// NotifyAdditionalData carries optional context for notifyMarketingInternal.
// OrderItems is a slice of cart items (shared.CartItem already holds Quantity
// and Harga, mirroring the Node {product:{kode,nama,harga,stok}, quantity}
// shape that processConfirmedCheckout builds from the cart).
type NotifyAdditionalData struct {
	OrderItems  []shared.CartItem
	TotalPrice  float64
	SearchQuery string
	LastResults []client.Product
}

// Notifier sends marketing notifications via WhatsApp. It is safe for
// concurrent use; the throttle map prevents duplicate notifications.
type Notifier struct {
	wa       *whatsmeow.Client
	mu       sync.Mutex
	throttle map[string]int64 // key="{phone}_{marketingContact}_{requestType}", val=last send ms
}

// NewNotifier creates a Notifier backed by the given whatsmeow client.
func NewNotifier(wa *whatsmeow.Client) *Notifier {
	return &Notifier{
		wa:       wa,
		throttle: make(map[string]int64),
	}
}

// sendWA sends a plain text WhatsApp message to a given E.164 phone number.
func (n *Notifier) sendWA(ctx context.Context, toNumber, text string) {
	jid := types.NewJID(toNumber, types.DefaultUserServer)
	msg := &waE2E.Message{Conversation: proto.String(text)}
	if _, err := n.wa.SendMessage(ctx, jid, msg); err != nil {
		log.Printf("notification: SendMessage to %s error: %v", toNumber, err)
	}
}

// wibNow returns current time in WIB (UTC+7) for display in notifications.
func wibNow() time.Time {
	return time.Now().In(time.FixedZone("WIB", 7*3600))
}

// Notify is the simple bantuan_request notifier from notifyMarketing (line
// 2244). Called during registration completion (completeRegistration line
// 1688) and simple bantuan flows. No throttle; callers manage timing.
func (n *Notifier) Notify(ctx context.Context, phone string, customerNama, customerRegion string) {
	if n.wa == nil {
		return
	}
	info := GetMarketingInfo(customerRegion)
	toNumber := effectiveNumber(info.Number)

	customerDetails := ""
	if customerNama != "" {
		customerDetails = fmt.Sprintf("\nNama: %s\nWilayah: %s", customerNama, customerRegion)
	}

	notifMsg := fmt.Sprintf("🔔 *Permintaan Bantuan*\nDari: %s%s\n\nMohon segera ditindaklanjuti.", phone, customerDetails)
	n.sendWA(ctx, toNumber, notifMsg)

	confirmMsg := fmt.Sprintf("Permintaan bantuan Anda telah diteruskan ke %s dari tim marketing kami. Beliau akan segera menghubungi Anda.", info.Name)
	n.sendWA(ctx, phone, confirmMsg)
}

// NotifyInternal ports notifyMarketingInternal (line 4087). It throttles at
// 30 minutes per (phone, marketingContact, requestType) combo and supports
// three requestType values: "bantuan_request", "checkout_request",
// "warehouse_search_request". MARKETING_NOTIFY_OVERRIDE replaces the target
// marketing number when set (testing safety net).
func (n *Notifier) NotifyInternal(
	ctx context.Context,
	phone string,
	customer *client.Customer,
	mktInfo MarketingInfo,
	requestType string,
	overrideNumber string,
	addData *NotifyAdditionalData,
) {
	if n.wa == nil {
		return
	}

	toNumber := mktInfo.Number
	if overrideNumber != "" {
		toNumber = overrideNumber
	}
	toNumber = effectiveNumber(toNumber)

	// 30-minute throttle per (phone, toNumber, requestType).
	throttleKey := phone + "_" + toNumber + "_" + requestType
	now := time.Now().UnixMilli()
	n.mu.Lock()
	last := n.throttle[throttleKey]
	if now-last < 30*60*1000 {
		n.mu.Unlock()
		log.Printf("notification: skipping duplicate %s for %s → %s", requestType, phone, toNumber)
		return
	}
	n.throttle[throttleKey] = now
	n.mu.Unlock()

	// Build notification message.
	ts := wibNow().Format("02/01/2006, 15.04.05")
	var notifMsg string

	switch requestType {
	case "bantuan_request":
		notifMsg = "🔔 *Permintaan Bantuan*\n\n"
		if customer != nil && customer.Nama != "" {
			notifMsg += fmt.Sprintf("👤 *Customer:* %s\n", customer.Nama)
		}
		notifMsg += fmt.Sprintf("📱 *Phone:* %s\n", phone)
		if customer != nil && customer.Address != "" && strings.ToLower(customer.Address) != "jakarta" {
			notifMsg += fmt.Sprintf("📍 *Region:* %s\n", customer.Address)
		}
		notifMsg += fmt.Sprintf("\n💬 Customer meminta bantuan umum.\n⏰ Waktu: %s\n\nMohon segera ditindaklanjuti.", ts)

	case "checkout_request":
		notifMsg = "🛒 *Pesanan Baru*\n\n"
		if customer != nil && customer.Nama != "" {
			notifMsg += fmt.Sprintf("👤 *Customer:* %s\n", customer.Nama)
		}
		notifMsg += fmt.Sprintf("📱 *Phone:* %s\n\n", phone)
		if addData != nil && len(addData.OrderItems) > 0 {
			notifMsg += "📦 *Detail Pesanan:*\n"
			for i, item := range addData.OrderItems {
				itemTotal := item.Harga * float64(item.Quantity)
				notifMsg += fmt.Sprintf("%d. %s x%d\n   %s\n   %s\n\n",
					i+1, item.Kode, item.Quantity, item.Nama, shared.FormatPrice(itemTotal))
			}
		}
		if addData != nil && addData.TotalPrice > 0 {
			notifMsg += fmt.Sprintf("💰 *Total: %s*\n\n", shared.FormatPrice(addData.TotalPrice))
		}
		notifMsg += fmt.Sprintf("💬 Customer telah membuat pesanan baru.\n⏰ Waktu: %s\n\nMohon segera ditindaklanjuti untuk konfirmasi dan pengaturan pengiriman.", ts)

	default: // "warehouse_search_request"
		notifMsg = "🔍 *Permintaan Cek Gudang Lain*\n\n"
		if customer != nil && customer.Nama != "" {
			notifMsg += fmt.Sprintf("👤 *Customer:* %s\n", customer.Nama)
		}
		notifMsg += fmt.Sprintf("📱 *Phone:* %s\n", phone)
		if customer != nil && customer.Address != "" && strings.ToLower(customer.Address) != "jakarta" {
			notifMsg += fmt.Sprintf("📍 *Region:* %s\n", customer.Address)
		}
		notifMsg += "\n"
		if addData != nil && addData.SearchQuery != "" {
			notifMsg += fmt.Sprintf("🔎 *Pencarian:* %s\n", addData.SearchQuery)
		}
		if addData != nil && len(addData.LastResults) > 0 {
			notifMsg += "📦 *Produk yang dicari:*\n"
			limit := 3
			if len(addData.LastResults) < limit {
				limit = len(addData.LastResults)
			}
			for i, p := range addData.LastResults[:limit] {
				notifMsg += fmt.Sprintf("%d. %s - %s\n", i+1, p.Kode, p.Nama)
			}
			if len(addData.LastResults) > 3 {
				notifMsg += fmt.Sprintf("... dan %d produk lainnya\n", len(addData.LastResults)-3)
			}
		}
		notifMsg += fmt.Sprintf("\n💬 Customer meminta pengecekan stok di gudang lain.\n⏰ Waktu: %s\n\nMohon bantu cek ketersediaan di gudang lain.", ts)
	}

	n.sendWA(ctx, toNumber, notifMsg)

	// Customer confirmation.
	var confirmMsg string
	switch requestType {
	case "bantuan_request":
		confirmMsg = fmt.Sprintf("✅ Permintaan bantuan Anda telah diteruskan ke *%s* dari tim marketing kami. Beliau akan segera menghubungi Anda.", mktInfo.Name)
	case "checkout_request":
		confirmMsg = fmt.Sprintf("✅ Pesanan Anda telah diteruskan ke *%s* dari tim marketing kami. Beliau akan segera menghubungi Anda untuk konfirmasi dan pengaturan pengiriman.", mktInfo.Name)
	default:
		confirmMsg = fmt.Sprintf("✅ Permintaan pengecekan gudang lain telah diteruskan ke *%s* dari tim marketing kami. Beliau akan membantu mengecek ketersediaan produk di gudang lain.", mktInfo.Name)
	}
	n.sendWA(ctx, phone, confirmMsg)

	log.Printf("notification: sent %s to %s (marketing: %s)", requestType, toNumber, mktInfo.Name)
}

// effectiveNumber applies the MARKETING_NOTIFY_OVERRIDE env var: when set,
// all marketing-directed messages go to that number instead. This avoids
// accidentally notifying real marketing contacts during testing.
func effectiveNumber(number string) string {
	if override := os.Getenv("MARKETING_NOTIFY_OVERRIDE"); override != "" {
		return override
	}
	return number
}

// TitleCase converts a space-separated string to title case, matching the
// Node `.map(word => word.charAt(0).toUpperCase() + word.slice(1)).join(' ')`
// used in handleMarketingCommand.
func TitleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		runes := []rune(w)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
		}
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}

// FormatDisplayNumber converts an E.164 number (628...) to local display
// format by replacing the "62" prefix with "0". Matches the Node
// `info.number.replace('62', '0')` in handleMarketingCommand.
func FormatDisplayNumber(number string) string {
	return strings.Replace(number, "62", "0", 1)
}
