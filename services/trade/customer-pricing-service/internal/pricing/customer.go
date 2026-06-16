package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"customer-pricing-service/internal/jurnal"
)

var nonDigitPattern = regexp.MustCompile(`\D`)
var whitespacePattern = regexp.MustCompile(`\s+`)

// Customer mirrors the customer record shape produced by saveCustomers() in
// local-data-manager.js, plus a few extra fields used only for the
// "vip_only" synthetic customer built by getCustomerInfoEnhanced().
type Customer struct {
	ID                     *int64       `json:"id,omitempty"`
	Nomor                  string       `json:"nomor"`
	Nama                   string       `json:"nama"`
	Email                  string       `json:"email"`
	Address                string       `json:"address"`
	Company                string       `json:"company"`
	Marketing              string       `json:"marketing"`
	Active                 bool         `json:"active"`
	Balance                float64      `json:"balance"`
	ReceivablesBalance     float64      `json:"receivables_balance"`
	PayablesBalance        float64      `json:"payables_balance"`
	TaxNo                  string       `json:"tax_no"`
	Fax                    string       `json:"fax"`
	BillingAddressProvinsi string       `json:"billing_address_provinsi"`
	Pulau                  string       `json:"pulau"`
	LastUpdated            string       `json:"lastUpdated,omitempty"`
	Wilayah                string       `json:"wilayah,omitempty"`
	Source                 string       `json:"source,omitempty"`
	IsVip                  bool         `json:"isVip,omitempty"`
	VipInfo                *VipCustomer `json:"vipInfo,omitempty"`
}

// Store holds the in-memory customer snapshot, backed by an atomically
// updated JSON file (data/customer-pricing/customers.json).
type Store struct {
	mu        sync.RWMutex
	customers []Customer
	dataDir   string
	jurnal    *jurnal.Client
}

func NewStore(dataDir string, jurnalClient *jurnal.Client) *Store {
	return &Store{dataDir: dataDir, jurnal: jurnalClient}
}

func (s *Store) snapshotPath() string {
	return filepath.Join(s.dataDir, "customers.json")
}

// LoadSnapshot loads customers.json from disk if present. Missing file is
// not an error (serve empty until the first sync completes).
func (s *Store) LoadSnapshot() error {
	data, err := os.ReadFile(s.snapshotPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var customers []Customer
	if err := json.Unmarshal(data, &customers); err != nil {
		return err
	}
	s.mu.Lock()
	s.customers = customers
	s.mu.Unlock()
	return nil
}

// saveSnapshot writes customers atomically (temp file + rename).
func (s *Store) saveSnapshot(customers []Customer) error {
	data, err := json.MarshalIndent(customers, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.snapshotPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.snapshotPath())
}

// All returns a copy of the current customer slice.
func (s *Store) All() []Customer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Customer, len(s.customers))
	copy(out, s.customers)
	return out
}

func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.customers)
}

// FormatPhoneNumber - ported verbatim from customerService.formatPhoneNumber.
func FormatPhoneNumber(phoneNumber string) string {
	if phoneNumber == "" {
		return ""
	}

	phone := strings.TrimSpace(phoneNumber)

	if strings.Contains(phone, "/") {
		phone = strings.TrimSpace(strings.SplitN(phone, "/", 2)[0])
	}
	if strings.Contains(phone, " ") && len(phone) > 15 {
		parts := strings.SplitN(phone, " ", 2)
		phone = strings.TrimSpace(parts[0])
	}

	cleaned := nonDigitPattern.ReplaceAllString(phone, "")

	if len(cleaned) < 8 || len(cleaned) > 15 {
		return ""
	}

	if strings.HasPrefix(cleaned, "0") {
		if strings.HasPrefix(cleaned, "08") {
			cleaned = "62" + cleaned[1:]
		} else {
			cleaned = cleaned[1:]
		}
	}

	if strings.HasPrefix(cleaned, "62") {
		cleaned = cleaned[2:]
	}

	return cleaned
}

// PhoneNumbersMatch - ported verbatim from customerService.phoneNumbersMatch.
func PhoneNumbersMatch(phone1, phone2 string) bool {
	if phone1 == "" || phone2 == "" {
		return false
	}

	phone1Clean := nonDigitPattern.ReplaceAllString(phone1, "")
	phone2Clean := nonDigitPattern.ReplaceAllString(phone2, "")

	if phone1Clean == "" || phone2Clean == "" {
		return false
	}

	norm1 := phone1Clean
	norm2 := phone2Clean

	if strings.HasPrefix(norm1, "0") {
		norm1 = norm1[1:]
	}
	if strings.HasPrefix(norm2, "0") {
		norm2 = norm2[1:]
	}
	if strings.HasPrefix(norm1, "62") {
		norm1 = norm1[2:]
	}
	if strings.HasPrefix(norm2, "62") {
		norm2 = norm2[2:]
	}

	if norm1 == norm2 {
		return true
	}

	if len(norm1) >= 8 && len(norm2) >= 8 {
		if norm1[len(norm1)-8:] == norm2[len(norm2)-8:] {
			return true
		}
	}

	return false
}

// FindCustomerByPhoneSimple - ported verbatim from
// customerService.findCustomerByPhoneSimple.
func (s *Store) FindCustomerByPhoneSimple(phoneNumber string) *Customer {
	if phoneNumber == "" || phoneNumber == "-" {
		return nil
	}

	cleanPhone := nonDigitPattern.ReplaceAllString(phoneNumber, "")
	normalizedPhone := cleanPhone
	if strings.HasPrefix(normalizedPhone, "0") {
		normalizedPhone = normalizedPhone[1:]
	} else if strings.HasPrefix(normalizedPhone, "62") {
		normalizedPhone = normalizedPhone[2:]
	}

	customers := s.All()
	for i := range customers {
		c := &customers[i]
		if c.Nomor == "" {
			continue
		}

		customerPhone := nonDigitPattern.ReplaceAllString(c.Nomor, "")
		normalizedCustomerPhone := customerPhone
		if strings.HasPrefix(normalizedCustomerPhone, "0") {
			normalizedCustomerPhone = normalizedCustomerPhone[1:]
		} else if strings.HasPrefix(normalizedCustomerPhone, "62") {
			normalizedCustomerPhone = normalizedCustomerPhone[2:]
		}

		if normalizedPhone == normalizedCustomerPhone {
			return c
		}

		if len(normalizedPhone) >= 8 && len(normalizedCustomerPhone) >= 8 {
			suffix1 := normalizedPhone[len(normalizedPhone)-8:]
			suffix2 := normalizedCustomerPhone[len(normalizedCustomerPhone)-8:]
			if suffix1 == suffix2 {
				return c
			}
		}
	}

	return nil
}

// VerifyCustomer - ported verbatim from customerService.verifyCustomer
// (without the 30s phone cache, which is a pure perf detail).
func (s *Store) VerifyCustomer(phoneNumber string) *Customer {
	if phoneNumber == "" {
		return nil
	}
	formatted := FormatPhoneNumber(phoneNumber)
	return s.FindCustomerByPhoneSimple(formatted)
}

// FindCustomerByCompanyName - ported verbatim from
// customerService.findCustomerByCompanyName.
func (s *Store) FindCustomerByCompanyName(companyName string) *Customer {
	companyName = strings.TrimSpace(companyName)
	if companyName == "" {
		return nil
	}

	customers := s.All()
	if len(customers) == 0 {
		return nil
	}

	normalizedInput := whitespacePattern.ReplaceAllString(strings.ToLower(companyName), "")
	searchName := strings.ToLower(companyName)

	for i := range customers {
		c := &customers[i]
		if c.Nomor == "" || c.Nomor == "-" || strings.TrimSpace(c.Nomor) == "" {
			continue
		}
		namaNormalized := whitespacePattern.ReplaceAllString(strings.ToLower(c.Nama), "")
		if namaNormalized == normalizedInput {
			return c
		}
	}

	for i := range customers {
		c := &customers[i]
		if c.Nomor == "" || c.Nomor == "-" || strings.TrimSpace(c.Nomor) == "" {
			continue
		}
		customerName := strings.ToLower(strings.TrimSpace(c.Nama))
		if strings.Contains(customerName, searchName) || strings.Contains(searchName, customerName) {
			return c
		}
	}

	return nil
}

// GetCustomerDetailWithIsland - ported from messageHandler.getCustomerDetailWithIsland
// + the relevant parts of getCustomerInfoEnhanced (phone lookup and the
// VIP-only fallback; the session-cache and company-name lookup branches are
// out of scope for this stateless service).
func (s *Store) GetCustomerDetailWithIsland(phoneNumber string, vipStore *VipStore) *Customer {
	var customer *Customer

	customerByPhone := s.VerifyCustomer(phoneNumber)
	if customerByPhone != nil && customerByPhone.Nama != "" &&
		customerByPhone.Nomor != "" && customerByPhone.Nomor != "-" &&
		strings.TrimSpace(customerByPhone.Nomor) != "" {
		c := *customerByPhone
		customer = &c
	} else if vip := vipStore.FindVipCustomer(phoneNumber); vip != nil {
		customer = &Customer{
			Nama:    vip.Nama,
			Nomor:   phoneNumber,
			Wilayah: vip.Wilayah,
			Source:  "vip_only",
			IsVip:   true,
			VipInfo: vip,
		}
	}

	if customer == nil {
		return nil
	}

	if customer.Nama == "" {
		if customer.ID != nil {
			customer.Nama = fmt.Sprintf("%d", *customer.ID)
		} else {
			customer.Nama = "Unknown Customer"
		}
	}

	if _, ok := PricingRules[customer.Pulau]; customer.Pulau == "" || !ok {
		if customer.BillingAddressProvinsi != "" {
			customer.Pulau = GetIslandFromProvince(customer.BillingAddressProvinsi)
		} else {
			customer.Pulau = "Pulau Kalimantan"
		}
	}

	return customer
}

// jsString returns the first non-empty string value found among the keys,
// mirroring JS's `a || b || c || ""` truthy fallback for strings.
func jsString(obj map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if s, ok := obj[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// jsNumber mirrors `(raw && raw[key]) || obj[key] || 0`.
func jsNumber(raw, obj map[string]interface{}, key string) float64 {
	if raw != nil {
		if v, ok := toFloat(raw[key]); ok && v != 0 {
			return v
		}
	}
	if obj != nil {
		if v, ok := toFloat(obj[key]); ok && v != 0 {
			return v
		}
	}
	return 0
}

// jsStringRaw mirrors `(raw && raw[key]) || obj[key] || ""`.
func jsStringRaw(raw, obj map[string]interface{}, key string) string {
	if raw != nil {
		if s, ok := raw[key].(string); ok && s != "" {
			return s
		}
	}
	if obj != nil {
		if s, ok := obj[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func toFloat(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	}
	return 0, false
}

// jsID mirrors `customer.id || customer.person_id`.
func jsID(obj map[string]interface{}, keys ...string) *int64 {
	for _, k := range keys {
		v, ok := obj[k]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case float64:
			if t != 0 {
				iv := int64(t)
				return &iv
			}
		case string:
			if t != "" {
				if iv, err := strconv.ParseInt(t, 10, 64); err == nil {
					return &iv
				}
			}
		}
	}
	return nil
}

// transformRawCustomers - ported verbatim from the `.map()` in
// local-data-manager.saveCustomers (the field-filtering step).
func transformRawCustomers(raw []map[string]interface{}) []Customer {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	result := make([]Customer, 0, len(raw))

	for _, c := range raw {
		rawObj, _ := c["raw"].(map[string]interface{})

		active := true
		if rawObj != nil {
			if isArchived, ok := rawObj["is_archived"].(bool); ok {
				active = !isArchived
			}
		}

		result = append(result, Customer{
			ID:                     jsID(c, "id", "person_id"),
			Nomor:                  jsString(c, "nomor", "mobile", "phone"),
			Nama:                   jsString(c, "nama", "display_name", "name"),
			Email:                  jsString(c, "email"),
			Address:                jsString(c, "address"),
			Company:                jsString(c, "company", "associate_company"),
			Marketing:              strings.ToLower(jsString(c, "marketing")),
			Active:                 active,
			Balance:                jsNumber(rawObj, c, "balance"),
			ReceivablesBalance:     jsNumber(rawObj, c, "receivables_balance"),
			PayablesBalance:        jsNumber(rawObj, c, "payables_balance"),
			TaxNo:                  jsStringRaw(rawObj, c, "tax_no"),
			Fax:                    jsStringRaw(rawObj, c, "fax"),
			BillingAddressProvinsi: "",
			Pulau:                  "",
			LastUpdated:            now,
		})
	}

	return result
}

// Sync - ported from local-sync-system.syncCustomers: fetch all customers
// from Jurnal.id, merge with existing snapshot to preserve previously
// resolved province/island, enrich customers still missing province, apply
// the hardcoded special case for customer 36619280, validate, then persist.
func (s *Store) Sync(ctx context.Context) error {
	existing := s.All()

	rawCustomers, err := s.jurnal.FetchAllCustomers(ctx)
	if err != nil {
		return fmt.Errorf("fetchAllCustomers: %w", err)
	}

	newCustomers := transformRawCustomers(rawCustomers)

	// Safety checks - mirror saveCustomers' guards.
	if len(newCustomers) == 0 {
		return fmt.Errorf("empty API response - refusing to overwrite existing data")
	}
	if len(existing) > 100 && len(newCustomers) < len(existing)/2 {
		return fmt.Errorf("new data (%d) is significantly smaller than existing (%d), skipping save", len(newCustomers), len(existing))
	}

	// Merge: preserve existing province/island for matched customer IDs.
	existingByID := make(map[int64]*Customer, len(existing))
	for i := range existing {
		if existing[i].ID != nil {
			existingByID[*existing[i].ID] = &existing[i]
		}
	}
	for i := range newCustomers {
		if newCustomers[i].ID == nil {
			continue
		}
		if ec, ok := existingByID[*newCustomers[i].ID]; ok && ec.BillingAddressProvinsi != "" {
			newCustomers[i].BillingAddressProvinsi = ec.BillingAddressProvinsi
			newCustomers[i].Pulau = ec.Pulau
		}
	}

	// Enrich customers still missing province via Jurnal profile API.
	s.updateCustomersWithProvinceData(ctx, newCustomers)

	// Hardcoded special case - ported verbatim from syncCustomers.
	for i := range newCustomers {
		if newCustomers[i].ID != nil && *newCustomers[i].ID == 36619280 {
			newCustomers[i].BillingAddressProvinsi = "Bandar Lampung"
			newCustomers[i].Pulau = "Sumatera"
		}
	}

	if err := s.saveSnapshot(newCustomers); err != nil {
		return fmt.Errorf("saveSnapshot: %w", err)
	}

	s.mu.Lock()
	s.customers = newCustomers
	s.mu.Unlock()

	return nil
}

// updateCustomersWithProvinceData enriches customers whose
// billing_address_provinsi is still empty by fetching their profile from
// Jurnal.id (rate limited ~20/min, trying API_KEY_CUST1 then API_KEY_CUST2).
func (s *Store) updateCustomersWithProvinceData(ctx context.Context, customers []Customer) {
	toProcess := 0
	for i := range customers {
		if customers[i].BillingAddressProvinsi == "" && customers[i].ID != nil {
			toProcess++
		}
	}
	if toProcess == 0 {
		return
	}
	log.Printf("updating province data for %d customers...", toProcess)

	successCount := 0
	for i := range customers {
		if customers[i].BillingAddressProvinsi != "" || customers[i].ID == nil {
			continue
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		profile, err := s.jurnal.FetchCustomerProfile(ctx, *customers[i].ID)
		if err != nil || profile == nil {
			continue
		}

		provinsi, _ := profile["billing_address_provinsi"].(string)
		customers[i].BillingAddressProvinsi = provinsi
		customers[i].Pulau = GetIslandFromProvince(provinsi)
		successCount++
	}

	log.Printf("province data update complete: %d/%d customers updated", successCount, toProcess)
}
