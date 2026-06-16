// Package client provides typed HTTP clients for the Phase 2 services
// (product-search-service, customer-pricing-service, ai-vision-service).
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// HargaNum mirrors the "_hargaNum" object produced by product-search-service.
type HargaNum struct {
	Normal      float64 `json:"normal"`
	Customer    float64 `json:"customer"`
	NonCustomer float64 `json:"nonCustomer"`
	Cash        float64 `json:"cash"`
}

// Product mirrors product-search-service's GET /search response entries
// (product-search-service/internal/model/product.go).
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
}

// ProductSearchClient talks to product-search-service.
type ProductSearchClient struct {
	baseURL string
	http    *http.Client
}

func NewProductSearchClient(baseURL string) *ProductSearchClient {
	return &ProductSearchClient{baseURL: strings.TrimSuffix(baseURL, "/"), http: &http.Client{}}
}

type searchResponse struct {
	Products []Product `json:"products"`
}

// Search calls GET /search?q=...&limit=..., the port of
// productService.searchProducts (backed by Meilisearch).
func (c *ProductSearchClient) Search(ctx context.Context, query string, limit int) ([]Product, error) {
	u := fmt.Sprintf("%s/search?q=%s&limit=%d", c.baseURL, url.QueryEscape(query), limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("product-search: GET /search status %d", resp.StatusCode)
	}

	var body searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	if body.Products == nil {
		body.Products = []Product{}
	}
	return body.Products, nil
}
