package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// VipCustomer mirrors customer-pricing-service/internal/pricing/vip.go's
// VipCustomer (one entry of customer_vip.json's vip_customers[]).
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

// Customer mirrors customer-pricing-service/internal/pricing/customer.go's
// Customer.
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

// PriceHargaNum mirrors the subset of product._hargaNum used for pricing
// (customer-pricing-service/internal/pricing/price.go's HargaNum).
type PriceHargaNum struct {
	Customer    float64 `json:"customer"`
	NonCustomer float64 `json:"nonCustomer"`
}

type PriceRequest struct {
	PhoneNumber string        `json:"phoneNumber"`
	HargaNum    PriceHargaNum `json:"hargaNum"`
}

type PriceResponse struct {
	Price        float64     `json:"price"`
	IsVip        bool        `json:"isVip"`
	IsRegistered bool        `json:"isRegistered,omitempty"`
	Pulau        string      `json:"pulau,omitempty"`
	Discount     float64     `json:"discount,omitempty"`
	Customer     interface{} `json:"customer,omitempty"`
}

// CustomerPricingClient talks to customer-pricing-service.
type CustomerPricingClient struct {
	baseURL string
	http    *http.Client
}

func NewCustomerPricingClient(baseURL string) *CustomerPricingClient {
	return &CustomerPricingClient{baseURL: strings.TrimSuffix(baseURL, "/"), http: &http.Client{}}
}

// errorBody mirrors the {"error": "..."} bodies written by writeJSON on
// non-2xx responses.
type errorBody struct {
	Error string `json:"error"`
}

// getJSON performs a GET request and decodes a 200 response into out. A 404
// response returns found=false with no error, matching the "not found"
// semantics of the underlying endpoints.
func (c *CustomerPricingClient) getJSON(ctx context.Context, path string, out interface{}) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return false, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return false, err
		}
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		var eb errorBody
		_ = json.NewDecoder(resp.Body).Decode(&eb)
		return false, fmt.Errorf("customer-pricing: GET %s status %d: %s", path, resp.StatusCode, eb.Error)
	}
}

// GetCustomer calls GET /customer?phone=..., the port of
// customerService.verifyCustomer + the VIP-only fallback (already combined
// server-side by GetCustomerDetailWithIsland). Returns nil, nil if not found.
func (c *CustomerPricingClient) GetCustomer(ctx context.Context, phone string) (*Customer, error) {
	var customer Customer
	found, err := c.getJSON(ctx, "/customer?phone="+url.QueryEscape(phone), &customer)
	if err != nil || !found {
		return nil, err
	}
	return &customer, nil
}

// GetCustomerByCompany calls GET /customer/by-company?name=..., the port of
// customerService.findCustomerByCompanyName. Returns nil, nil if not found.
func (c *CustomerPricingClient) GetCustomerByCompany(ctx context.Context, name string) (*Customer, error) {
	var customer Customer
	found, err := c.getJSON(ctx, "/customer/by-company?name="+url.QueryEscape(name), &customer)
	if err != nil || !found {
		return nil, err
	}
	return &customer, nil
}

// GetCustomerVip calls GET /customer/vip?phone=..., the port of
// localDataManager.findVipCustomer. Returns nil, nil if not found.
func (c *CustomerPricingClient) GetCustomerVip(ctx context.Context, phone string) (*VipCustomer, error) {
	var vip VipCustomer
	found, err := c.getJSON(ctx, "/customer/vip?phone="+url.QueryEscape(phone), &vip)
	if err != nil || !found {
		return nil, err
	}
	return &vip, nil
}

// GetPrice calls POST /price, the port of messageHandler.getCustomerPrice
// (VIP discount, registration check, island pricing factor, base price
// selection).
func (c *CustomerPricingClient) GetPrice(ctx context.Context, req PriceRequest) (*PriceResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/price", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var eb errorBody
		_ = json.NewDecoder(resp.Body).Decode(&eb)
		return nil, fmt.Errorf("customer-pricing: POST /price status %d: %s", resp.StatusCode, eb.Error)
	}

	var priceResp PriceResponse
	if err := json.NewDecoder(resp.Body).Decode(&priceResp); err != nil {
		return nil, err
	}
	return &priceResp, nil
}
