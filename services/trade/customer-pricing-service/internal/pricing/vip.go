package pricing

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"sync"
	"time"
)

// VipCustomer mirrors one entry of customer_vip.json's vip_customers[].
type VipCustomer struct {
	ID                 int      `json:"id"`
	Nama               string   `json:"nama"`
	Wilayah            string   `json:"wilayah"`
	Grade              string   `json:"grade"`
	DiscountPercentage float64  `json:"discount_percentage"`
	PhoneNumbers       []string `json:"phone_numbers"`
	Status             string   `json:"status"`
	CreatedAt          string   `json:"created_at"`
	UpdatedAt          string   `json:"updated_at"`
}

type vipFile struct {
	VipCustomers []VipCustomer `json:"vip_customers"`
}

// VipStore loads customer_vip.json (manually managed seed data) and
// hot-reloads it on file mtime change, mirroring the chokidar watcher in
// local-data-manager.js.
type VipStore struct {
	mu        sync.RWMutex
	customers []VipCustomer
	filePath  string
	lastMod   time.Time
}

func NewVipStore(filePath string) *VipStore {
	return &VipStore{filePath: filePath}
}

func (v *VipStore) Load() error {
	info, err := os.Stat(v.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	data, err := os.ReadFile(v.filePath)
	if err != nil {
		return err
	}

	var f vipFile
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}

	v.mu.Lock()
	v.customers = f.VipCustomers
	v.lastMod = info.ModTime()
	v.mu.Unlock()
	return nil
}

// WatchAndReload polls the file mtime every 30s and reloads on change,
// mirroring the 30s checkCustomerFileUpdates loop in customerService.js.
func (v *VipStore) WatchAndReload(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info, err := os.Stat(v.filePath)
			if err != nil {
				continue
			}
			v.mu.RLock()
			changed := info.ModTime().After(v.lastMod)
			v.mu.RUnlock()
			if changed {
				_ = v.Load()
			}
		}
	}
}

func (v *VipStore) All() []VipCustomer {
	v.mu.RLock()
	defer v.mu.RUnlock()
	out := make([]VipCustomer, len(v.customers))
	copy(out, v.customers)
	return out
}

// lastN returns the last n characters of s, or s itself if shorter -
// mirrors JS's String.prototype.slice(-n).
func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// FindVipCustomer - ported verbatim from local-data-manager.findVipCustomer.
func (v *VipStore) FindVipCustomer(phoneNumber string) *VipCustomer {
	if phoneNumber == "" {
		return nil
	}

	customers := v.All()
	if len(customers) == 0 {
		return nil
	}

	normalizedPhone := nonDigitPattern.ReplaceAllString(phoneNumber, "")

	for i := range customers {
		vip := &customers[i]
		for _, vipPhone := range vip.PhoneNumbers {
			if vipPhone == "" {
				continue
			}
			normalizedVipPhone := nonDigitPattern.ReplaceAllString(vipPhone, "")

			if normalizedVipPhone == normalizedPhone {
				return vip
			}
			if hasSuffix(normalizedVipPhone, lastN(normalizedPhone, 10)) {
				return vip
			}
			if hasSuffix(normalizedPhone, lastN(normalizedVipPhone, 10)) {
				return vip
			}
		}
	}

	return nil
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// VipPriceResult mirrors the return shape of local-data-manager.calculateVipPrice.
type VipPriceResult struct {
	IsVip         bool
	Price         float64
	Discount      float64
	OriginalPrice float64
	VipCustomer   *VipCustomer
}

// CalculateVipPrice - ported verbatim from local-data-manager.calculateVipPrice.
// hargaCustomer is product._hargaNum.customer; the JS fallback to
// product.hargaCustomer doesn't apply here since callers only ever pass
// _hargaNum via the /price request body.
func (v *VipStore) CalculateVipPrice(hargaCustomer float64, phoneNumber string) VipPriceResult {
	vip := v.FindVipCustomer(phoneNumber)
	if vip == nil || hargaCustomer == 0 {
		return VipPriceResult{IsVip: false}
	}

	discountPercent := vip.DiscountPercentage
	discountAmount := hargaCustomer * (discountPercent / 100)
	finalPrice := math.Round(hargaCustomer - discountAmount)

	return VipPriceResult{
		IsVip:         true,
		Price:         finalPrice,
		Discount:      discountPercent,
		OriginalPrice: hargaCustomer,
		VipCustomer:   vip,
	}
}
