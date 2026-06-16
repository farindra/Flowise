package pricing

import "math"

// HargaNum mirrors the subset of product._hargaNum used for pricing.
type HargaNum struct {
	Customer    float64 `json:"customer"`
	NonCustomer float64 `json:"nonCustomer"`
}

type PriceRequest struct {
	PhoneNumber string   `json:"phoneNumber"`
	HargaNum    HargaNum `json:"hargaNum"`
}

type PriceResponse struct {
	Price        float64     `json:"price"`
	IsVip        bool        `json:"isVip"`
	IsRegistered bool        `json:"isRegistered,omitempty"`
	Pulau        string      `json:"pulau,omitempty"`
	Discount     float64     `json:"discount,omitempty"`
	Customer     interface{} `json:"customer,omitempty"`
}

// GetCustomerPrice - ported verbatim from messageHandler.getCustomerPrice.
func GetCustomerPrice(store *Store, vipStore *VipStore, req PriceRequest) PriceResponse {
	vipPricing := vipStore.CalculateVipPrice(req.HargaNum.Customer, req.PhoneNumber)
	if vipPricing.IsVip {
		return PriceResponse{
			Price:    vipPricing.Price,
			IsVip:    true,
			Discount: vipPricing.Discount,
			Customer: vipPricing.VipCustomer,
		}
	}

	customer := store.GetCustomerDetailWithIsland(req.PhoneNumber, vipStore)
	isRegistered := customer != nil && customer.Nama != "" && customer.ID != nil

	var pulau string
	var pricingFactor float64
	if isRegistered {
		pulau = customer.Pulau
		if pulau == "" {
			pulau = "Pulau Kalimantan"
		}
		if pf, ok := PricingRules[pulau]; ok {
			pricingFactor = pf
		} else {
			pricingFactor = PricingRules["Pulau Kalimantan"]
		}
	} else {
		pulau = "nonCustomer"
		pricingFactor = PricingRules["nonCustomer"]
	}

	var basePrice float64
	if isRegistered {
		basePrice = req.HargaNum.Customer
	} else {
		basePrice = req.HargaNum.NonCustomer
	}

	price := math.Round(basePrice * pricingFactor)

	return PriceResponse{
		Price:        price,
		IsVip:        false,
		IsRegistered: isRegistered,
		Pulau:        pulau,
		Customer:     customer,
	}
}
