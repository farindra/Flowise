// Package transform ports the merge logic from syncProducts and the field
// mapping from saveProducts in local-sync-system.js / local-data-manager.js.
package transform

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"sync-indexer/internal/codevariations"
)

// flexString unmarshals a JSON string, number, or bool into a string. The
// oceanbearings.co.id product API stringifies all fields, but this guards
// against any field that comes through as a JSON number/bool instead.
type flexString string

func (f *flexString) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = flexString(s)
		return nil
	}

	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch v := raw.(type) {
	case float64:
		*f = flexString(strconv.FormatFloat(v, 'f', -1, 64))
	case bool:
		*f = flexString(strconv.FormatBool(v))
	default:
		*f = ""
	}
	return nil
}

// RawProduct is one element of the JSON array returned by
// PRODUK_API_URL/PRODUK_API_URL2. Only the fields used by saveProducts'
// mapping (and the syncProducts merge step) are declared - unknown fields are
// ignored by encoding/json, keeping per-product memory low even for the
// ~37000-product response.
type RawProduct struct {
	IDProduct         flexString `json:"id_product"`
	Kode              flexString `json:"kode"`
	Name              flexString `json:"name"`
	Description       flexString `json:"description"`
	Quantity          flexString `json:"quantity"`
	Active            flexString `json:"active"`
	AvailableForOrder flexString `json:"available_for_order"`
	Price255          flexString `json:"price_255"`
	Price355          flexString `json:"price_355"`
	PriceTC           flexString `json:"price_tc"`
	Price255HR        flexString `json:"price_255_hr"`
	Price355HR        flexString `json:"price_355_hr"`
	PriceTCHR         flexString `json:"price_tc_hr"`
}

// ProductDoc is the Meilisearch document shape. Mirrors the fields produced
// by saveProducts, with _hargaNum flattened (harga_*_num) for
// filtering/sorting and code_variations added (from buildSearchIndexes) for
// product-search-service's exact-match phase.
//
// Note: saveProducts does not produce a "keterangan" field (that only exists
// in the historical CSV-imported products.json, never re-populated by a
// regular API sync), so buildSearchIndexes' keterangan_words index is always
// empty for synced products - it is intentionally not ported here.
type ProductDoc struct {
	ID    int64  `json:"id"`
	Kode  string `json:"kode"`
	Nama  string `json:"nama"`
	Stok  int64  `json:"stok"`
	Brand string `json:"brand"`

	HargaNormal      string `json:"harga_normal"`
	HargaCustomer    string `json:"harga_customer"`
	HargaNonCustomer string `json:"harga_noncustomer"`
	HargaCash        string `json:"harga_cash"`

	HargaNormalNum      float64 `json:"harga_normal_num"`
	HargaCustomerNum    float64 `json:"harga_customer_num"`
	HargaNonCustomerNum float64 `json:"harga_noncustomer_num"`
	HargaCashNum        float64 `json:"harga_cash_num"`

	Active      bool   `json:"active"`
	Available   bool   `json:"available"`
	LastUpdated string `json:"lastUpdated"`

	CodeVariations []string `json:"code_variations"`
}

var specialBrands = []string{"NTN", "KOYO", "FAG"}

// MergeProducts - ported verbatim from the productMap merge logic in
// syncProducts: products are keyed by kode||name, and quantity/stok are
// summed for duplicates found in the secondary list.
func MergeProducts(primary, secondary []RawProduct) []RawProduct {
	productMap := make(map[string]RawProduct, len(primary)+len(secondary))
	order := make([]string, 0, len(primary)+len(secondary))

	for _, p := range primary {
		code := productCode(p)
		if code == "" {
			continue
		}
		if _, exists := productMap[code]; !exists {
			order = append(order, code)
		}
		productMap[code] = p
	}

	for _, p := range secondary {
		code := productCode(p)
		if code == "" {
			continue
		}

		if existing, ok := productMap[code]; ok {
			sum := parseIntLike(string(existing.Quantity)) + parseIntLike(string(p.Quantity))
			existing.Quantity = flexString(strconv.FormatInt(sum, 10))
			productMap[code] = existing
			continue
		}

		productMap[code] = p
		order = append(order, code)
	}

	result := make([]RawProduct, 0, len(order))
	for _, code := range order {
		result = append(result, productMap[code])
	}
	return result
}

func productCode(p RawProduct) string {
	if p.Kode != "" {
		return string(p.Kode)
	}
	return string(p.Name)
}

// ToDoc - ported verbatim from the field mapping in saveProducts (including
// the special-brand-then-dot-suffix brand extraction).
func ToDoc(p RawProduct) ProductDoc {
	name := string(p.Name)
	description := string(p.Description)

	brand := ""
	isSpecialBrand := false
	for _, sb := range specialBrands {
		if strings.Contains(name, sb) || strings.Contains(description, sb) {
			brand = sb
			isSpecialBrand = true
			break
		}
	}
	if !isSpecialBrand && strings.Contains(name, ".") {
		parts := strings.Split(name, ".")
		brand = parts[len(parts)-1]
	}

	nama := description
	if nama == "" {
		nama = name
	}

	return ProductDoc{
		ID:    toInt64(string(p.IDProduct)),
		Kode:  name,
		Nama:  nama,
		Stok:  toInt64(string(p.Quantity)),
		Brand: brand,

		HargaNormal:      string(p.Price255HR),
		HargaCustomer:    string(p.Price355HR),
		HargaNonCustomer: string(p.Price255HR),
		HargaCash:        string(p.PriceTCHR),

		HargaNormalNum:      toFloat(string(p.Price255)),
		HargaCustomerNum:    toFloat(string(p.Price355)),
		HargaNonCustomerNum: toFloat(string(p.Price255)),
		HargaCashNum:        toFloat(string(p.PriceTC)),

		Active:      string(p.Active) == "1",
		Available:   string(p.AvailableForOrder) == "1",
		LastUpdated: time.Now().UTC().Format(time.RFC3339Nano),

		CodeVariations: codevariations.CreateCodeVariations(strings.ToLower(name)),
	}
}

func parseIntLike(s string) int64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return int64(f)
}

func toInt64(s string) int64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f)
}

func toFloat(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}
