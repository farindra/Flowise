package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// ── Meilisearch ───────────────────────────────────────────────────────────────

type meiliClient struct {
	url           string
	key           string
	http          *http.Client
	productIndex  string
	customerIndex string
}

func newMeili(url, key string) *meiliClient {
	return &meiliClient{url: url, key: key, http: &http.Client{Timeout: 30 * time.Second}, productIndex: "jurnal_products", customerIndex: "customers"}
}

func (m *meiliClient) req(ctx context.Context, method, path string, body any) error {
	var br io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		br = bytes.NewReader(b)
	}
	req, _ := http.NewRequestWithContext(ctx, method, m.url+path, br)
	req.Header.Set("Authorization", "Bearer "+m.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("meili %s %s → %d: %s", method, path, resp.StatusCode, raw)
	}
	return nil
}

func (m *meiliClient) upsert(ctx context.Context, index string, docs any) error {
	return m.req(ctx, http.MethodPost, "/indexes/"+index+"/documents?primaryKey=id", docs)
}

func (m *meiliClient) ensureSearchable(ctx context.Context, index string, attrs []string) {
	_ = m.req(ctx, http.MethodPut, "/indexes/"+index+"/settings/searchable-attributes", attrs)
}

func (m *meiliClient) ensureFilterable(ctx context.Context, index string, attrs []string) {
	_ = m.req(ctx, http.MethodPut, "/indexes/"+index+"/settings/filterable-attributes", attrs)
}

// ── Jurnal HTTP helper ────────────────────────────────────────────────────────

type jurnalClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func newJurnal(baseURL, token string) *jurnalClient {
	return &jurnalClient{baseURL: baseURL, token: token, http: &http.Client{Timeout: 30 * time.Second}}
}

func (j *jurnalClient) getPage(ctx context.Context, endpoint string, page, perPage int) (map[string]any, error) {
	url := fmt.Sprintf("%s%s?page=%d&per_page=%d", j.baseURL, endpoint, page, perPage)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+j.token)
	resp, err := j.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("json decode: %w", err)
	}
	return result, nil
}

// ── Product sync ─────────────────────────────────────────────────────────────

type JurnalProduct struct {
	ID                int     `json:"id"`
	Name              string  `json:"name"`
	Code              string  `json:"code"`
	Description       string  `json:"description"`
	Unit              string  `json:"unit"`
	Category          string  `json:"category"`
	SellPrice         float64 `json:"sell_price"`
	LastBuyPrice      float64 `json:"last_buy_price"`
	AveragePrice      float64 `json:"average_price"`
	Quantity          float64 `json:"quantity"`
	QuantityAvailable float64 `json:"quantity_available"`
	Active            bool    `json:"active"`
	Archive           bool    `json:"archive"`
	SyncedAt          string  `json:"synced_at"`
}

func strVal(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func floatVal(m map[string]any, key string) float64 {
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0
}

func boolVal(m map[string]any, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func mapProduct(m map[string]any, now string) JurnalProduct {
	p := JurnalProduct{
		ID:                int(floatVal(m, "id")),
		Name:              strVal(m, "name"),
		Code:              strVal(m, "product_code"),
		Description:       strVal(m, "description"),
		SellPrice:         floatVal(m, "sell_price_per_unit"),
		LastBuyPrice:      floatVal(m, "last_buy_price"),
		AveragePrice:      floatVal(m, "average_price"),
		Quantity:          floatVal(m, "quantity"),
		QuantityAvailable: floatVal(m, "quantity_available"),
		Active:            boolVal(m, "active"),
		Archive:           boolVal(m, "archive"),
		SyncedAt:          now,
	}
	if unit, ok := m["unit"].(map[string]any); ok {
		p.Unit = strVal(unit, "name")
	}
	p.Category = strVal(m, "product_categories_string")
	return p
}

func (m *meiliClient) ensureSortable(ctx context.Context, index string, attrs []string) {
	_ = m.req(ctx, http.MethodPut, "/indexes/"+index+"/settings/sortable-attributes", attrs)
}

func syncProducts(ctx context.Context, j *jurnalClient, meili *meiliClient, status *syncStatus) (int, error) {
	idx := meili.productIndex
	log.Printf("[product-sync] starting (index: %s)...", idx)
	meili.ensureSearchable(ctx, idx, []string{"name", "code", "description", "category"})
	meili.ensureFilterable(ctx, idx, []string{"active", "archive", "quantity_available"})
	meili.ensureSortable(ctx, idx, []string{"name", "quantity", "quantity_available"})

	page := 1
	total := 0
	now := time.Now().Format("2006-01-02 15:04:05")

	for {
		data, err := j.getPage(ctx, "products", page, 100)
		if err != nil {
			return total, fmt.Errorf("page %d: %w", page, err)
		}
		items, _ := data["products"].([]any)
		if len(items) == 0 {
			break
		}

		docs := make([]JurnalProduct, 0, len(items))
		for _, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			p := mapProduct(m, now)
			if p.ID == 0 {
				continue
			}
			docs = append(docs, p)
		}

		if err := meili.upsert(ctx, idx, docs); err != nil {
			return total, fmt.Errorf("meili upsert page %d: %w", page, err)
		}

		total += len(docs)
		status.addProducts(len(docs))
		totalPages, _ := data["total_pages"].(float64)
		log.Printf("[product-sync] page %d/%d — %d docs", page, int(totalPages), total)

		if page >= int(totalPages) {
			break
		}
		page++
		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("[product-sync] done — %d products indexed", total)
	return total, nil
}

// ── Customer sync ─────────────────────────────────────────────────────────────

type JurnalCustomer struct {
	ID           int    `json:"id"`
	DisplayName  string `json:"display_name"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	Address      string `json:"address"`
	CustomerType string `json:"customer_type"`
	NPWP         string `json:"npwp"`
	SyncedAt     string `json:"synced_at"`
}

func mapCustomer(m map[string]any, now string) JurnalCustomer {
	name := strVal(m, "display_name")
	if name == "" {
		name = strVal(m, "name")
	}
	return JurnalCustomer{
		ID:           int(floatVal(m, "id")),
		DisplayName:  name,
		Name:         strVal(m, "name"),
		Email:        strVal(m, "email"),
		Phone:        strVal(m, "phone"),
		Address:      strVal(m, "address"),
		CustomerType: strVal(m, "customer_type"),
		NPWP:         strVal(m, "tax_no"),
		SyncedAt:     now,
	}
}

func syncCustomers(ctx context.Context, j *jurnalClient, meili *meiliClient, status *syncStatus) (int, error) {
	idx := meili.customerIndex
	log.Printf("[customer-sync] starting (index: %s)...", idx)
	meili.ensureSearchable(ctx, idx, []string{"display_name", "name", "phone", "address", "npwp"})

	page := 1
	total := 0
	now := time.Now().Format("2006-01-02 15:04:05")

	for {
		data, err := j.getPage(ctx, "customers", page, 100)
		if err != nil {
			return total, fmt.Errorf("page %d: %w", page, err)
		}
		items, _ := data["customers"].([]any)
		if len(items) == 0 {
			break
		}

		docs := make([]JurnalCustomer, 0, len(items))
		for _, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			c := mapCustomer(m, now)
			if c.ID == 0 {
				continue
			}
			docs = append(docs, c)
		}

		if err := meili.upsert(ctx, idx, docs); err != nil {
			return total, fmt.Errorf("meili upsert page %d: %w", page, err)
		}

		total += len(docs)
		status.addCustomers(len(docs))
		totalPages, _ := data["total_pages"].(float64)
		log.Printf("[customer-sync] page %d/%d — %d docs", page, int(totalPages), total)

		if page >= int(totalPages) {
			break
		}
		page++
		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("[customer-sync] done — %d customers indexed", total)
	return total, nil
}

// ── Full sync (products + customers) ─────────────────────────────────────────

func runFullSync(ctx context.Context, j *jurnalClient, meili *meiliClient, status *syncStatus) {
	status.setRunning(true)
	defer status.setRunning(false)

	start := time.Now()
	var errs []string
	var counts syncCounts

	nProducts, err := syncProducts(ctx, j, meili, status)
	counts.Products = nProducts
	if err != nil {
		log.Printf("[sync] product error: %v", err)
		errs = append(errs, "products: "+err.Error())
	}

	nCustomers, err := syncCustomers(ctx, j, meili, status)
	counts.Customers = nCustomers
	if err != nil {
		log.Printf("[sync] customer error: %v", err)
		errs = append(errs, "customers: "+err.Error())
	}

	status.record(time.Since(start), counts, errs)
}
