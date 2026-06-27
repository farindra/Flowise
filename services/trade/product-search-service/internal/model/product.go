// Package model defines the Meilisearch document shape (Doc, mirroring
// sync-indexer's transform.ProductDoc) and the API response shape (Product,
// reconstructing an entry of local-data-manager.js's in-memory
// `this.products` array).
package model

// Doc is the "products" index document shape, as written by sync-indexer.
type Doc struct {
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
	Keterangan  string `json:"keterangan"`

	CodeVariations []string `json:"code_variations"`
}

// HargaNum mirrors the "_hargaNum" object produced by saveProducts
// (local-data-manager.js ~line 361).
type HargaNum struct {
	Normal      float64 `json:"normal"`
	Customer    float64 `json:"customer"`
	NonCustomer float64 `json:"nonCustomer"`
	Cash        float64 `json:"cash"`
}

// Product reconstructs the shape of an entry in local-data-manager.js's
// in-memory `this.products` array (the shape searchProducts returns to
// callers).
type Product struct {
	ID    int64  `json:"id"`
	Kode  string `json:"kode"`
	Nama  string `json:"nama"`
	Stok  int64  `json:"stok"`
	Brand string `json:"brand"`

	HargaNormal      string `json:"hargaNormal"`
	HargaCustomer    string `json:"hargaCustomer"`
	HargaNonCustomer string `json:"hargaNonCustomer"`
	HargaCash        string `json:"hargaCash"`

	HargaNum HargaNum `json:"_hargaNum"`

	Active      bool   `json:"active"`
	Available   bool   `json:"available"`
	LastUpdated string `json:"lastUpdated"`
	Keterangan  string `json:"keterangan"`
}

// ToProduct reconstructs the original this.products entry shape from a
// Meilisearch document (un-flattening harga_*_num back into _hargaNum).
func (d Doc) ToProduct() Product {
	return Product{
		ID:    d.ID,
		Kode:  d.Kode,
		Nama:  d.Nama,
		Stok:  d.Stok,
		Brand: d.Brand,

		HargaNormal:      d.HargaNormal,
		HargaCustomer:    d.HargaCustomer,
		HargaNonCustomer: d.HargaNonCustomer,
		HargaCash:        d.HargaCash,

		HargaNum: HargaNum{
			Normal:      d.HargaNormalNum,
			Customer:    d.HargaCustomerNum,
			NonCustomer: d.HargaNonCustomerNum,
			Cash:        d.HargaCashNum,
		},

		Active:      d.Active,
		Available:   d.Available,
		LastUpdated: d.LastUpdated,
		Keterangan:  d.Keterangan,
	}
}
