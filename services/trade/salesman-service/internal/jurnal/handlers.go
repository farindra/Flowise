package jurnal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// MeiliSearcher is the subset of meilisearch.Syncer we need.
type MeiliSearcher interface {
	SearchCustomers(ctx context.Context, q string, limit int) ([]map[string]any, error)
}

type Handler struct {
	client *Client
	meili  MeiliSearcher // may be nil (falls back to Jurnal API)
}

func NewHandler(client *Client, meili MeiliSearcher) *Handler {
	return &Handler{client: client, meili: meili}
}

// Customer represents a Jurnal customer in our format.
type Customer struct {
	ID           int    `json:"id"`
	Nama         string `json:"nama"`
	Email        string `json:"email"`
	Telepon      string `json:"telepon"`
	Alamat       string `json:"alamat"`
	Tipe         string `json:"tipe"`
	NPWP         string `json:"npwp,omitempty"`
}

func formatCustomer(c map[string]any) Customer {
	id, _ := c["id"].(float64)
	name := strVal(c, "display_name")
	if name == "" {
		name = strVal(c, "name")
	}
	return Customer{
		ID:      int(id),
		Nama:    name,
		Email:   strVal(c, "email"),
		Telepon: strVal(c, "phone"),
		Alamat:  strVal(c, "address"),
		Tipe:    strVal(c, "customer_type"),
		NPWP:    strVal(c, "tax_no"),
	}
}

// HandleCustomers — GET /customers?q=...&limit=...&id=...
func (h *Handler) HandleCustomers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	// Lookup by ID
	if idStr := r.URL.Query().Get("id"); idStr != "" {
		resp, err := h.client.get(ctx, "customers/"+idStr, nil)
		if err != nil {
			jsonError(w, "Customer tidak ditemukan: "+err.Error(), 404)
			return
		}
		c := resp
		if inner, ok := resp["customer"].(map[string]any); ok {
			c = inner
		}
		json.NewEncoder(w).Encode(map[string]any{"customer": formatCustomer(c)})
		return
	}

	// Search by name — via Meilisearch (preferred) or Jurnal API fallback
	q := r.URL.Query().Get("q")
	limitStr := r.URL.Query().Get("limit")
	limitN := 20
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil {
			limitN = n
		}
	}

	source := "jurnal_api"
	var results []Customer

	if h.meili != nil {
		hits, err := h.meili.SearchCustomers(ctx, q, limitN)
		if err == nil {
			source = "meilisearch"
			for _, m := range hits {
				id, _ := m["id"].(float64)
				name := strVal(m, "display_name")
				if name == "" {
					name = strVal(m, "name")
				}
				results = append(results, Customer{
					ID:      int(id),
					Nama:    name,
					Email:   strVal(m, "email"),
					Telepon: strVal(m, "phone"),
					Alamat:  strVal(m, "address"),
					Tipe:    strVal(m, "customer_type"),
					NPWP:    strVal(m, "npwp"),
				})
			}
		}
	}

	if source == "jurnal_api" {
		params := map[string]string{"per_page": strconv.Itoa(limitN)}
		if q != "" {
			params["name"] = q
		}
		resp, err := h.client.get(ctx, "customers", params)
		if err != nil {
			jsonError(w, "Gagal ambil customer: "+err.Error(), 502)
			return
		}
		raw, _ := resp["customers"].([]any)
		for _, item := range raw {
			if m, ok := item.(map[string]any); ok {
				results = append(results, formatCustomer(m))
			}
		}
	}

	if results == nil {
		results = []Customer{}
	}
	json.NewEncoder(w).Encode(map[string]any{
		"query":   q,
		"total":   len(results),
		"results": results,
		"source":  source,
		"hint":    "Gunakan field 'id' sebagai customer_id di /quotation atau /sales-order",
	})
}

// QuotationRequest is the body for POST /quotation.
type QuotationRequest struct {
	CustomerID   int        `json:"customer_id"`
	CustomerName string     `json:"customer_name"`
	DueDate      string     `json:"due_date"`
	Memo         string     `json:"memo"`
	Items        []LineItem `json:"items"`
}

type LineItem struct {
	ProductName     string  `json:"product_name"`
	Quantity        float64 `json:"quantity"`
	UnitPrice       float64 `json:"unit_price"`
	Unit            string  `json:"unit"`
	DiscountPercent float64 `json:"discount_percent"`
}

// HandleQuotation — POST /quotation
func (h *Handler) HandleQuotation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}

	var req QuotationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Body JSON tidak valid: "+err.Error(), 400)
		return
	}
	if req.CustomerID == 0 && req.CustomerName == "" {
		jsonError(w, "customer_id atau customer_name wajib diisi", 400)
		return
	}
	if len(req.Items) == 0 {
		jsonError(w, "items wajib diisi", 400)
		return
	}

	dueDate := req.DueDate
	if dueDate == "" {
		dueDate = time.Now().AddDate(0, 0, 30).Format("2006-01-02")
	}

	lines := buildLines(req.Items)
	payload := map[string]any{
		"quotation": map[string]any{
			"transaction_date":              time.Now().Format("2006-01-02"),
			"due_date":                      dueDate,
			"memo":                          req.Memo,
			"person_id":                     nullIfZero(req.CustomerID),
			"person_name":                   req.CustomerName,
			"transaction_lines_attributes":  lines,
		},
	}

	resp, err := h.client.post(ctx, "quotations", payload)
	if err != nil {
		jsonError(w, "Gagal buat quotation: "+err.Error(), 502)
		return
	}

	q := resp
	if inner, ok := resp["quotation"].(map[string]any); ok {
		q = inner
	}
	id, _ := q["id"].(float64)
	json.NewEncoder(w).Encode(map[string]any{
		"success":      true,
		"quotation_id": int(id),
		"nomor":        strVal(q, "transaction_no"),
		"status":       strVal(q, "status"),
		"total":        q["amount"],
		"due_date":     dueDate,
		"url_jurnal":   fmt.Sprintf("https://jurnal.id/quotations/%d", int(id)),
	})
}

// HandleSalesOrder — POST /sales-order
func (h *Handler) HandleSalesOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}

	var req struct {
		QuotationRequest
		QuotationID int `json:"quotation_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Body JSON tidak valid: "+err.Error(), 400)
		return
	}
	if req.CustomerID == 0 && req.CustomerName == "" {
		jsonError(w, "customer_id atau customer_name wajib diisi", 400)
		return
	}
	if len(req.Items) == 0 {
		jsonError(w, "items wajib diisi", 400)
		return
	}

	dueDate := req.DueDate
	if dueDate == "" {
		dueDate = time.Now().AddDate(0, 0, 14).Format("2006-01-02")
	}

	srcDoc := ""
	if req.QuotationID > 0 {
		srcDoc = "quotation:" + strconv.Itoa(req.QuotationID)
	}

	payload := map[string]any{
		"sales_order": map[string]any{
			"transaction_date":             time.Now().Format("2006-01-02"),
			"due_date":                     dueDate,
			"memo":                         req.Memo,
			"person_id":                    nullIfZero(req.CustomerID),
			"person_name":                  req.CustomerName,
			"source_document":              srcDoc,
			"transaction_lines_attributes": buildLines(req.Items),
		},
	}

	resp, err := h.client.post(ctx, "sales_orders", payload)
	if err != nil {
		jsonError(w, "Gagal buat sales order: "+err.Error(), 502)
		return
	}

	so := resp
	if inner, ok := resp["sales_order"].(map[string]any); ok {
		so = inner
	}
	id, _ := so["id"].(float64)
	json.NewEncoder(w).Encode(map[string]any{
		"success":        true,
		"sales_order_id": int(id),
		"nomor":          strVal(so, "transaction_no"),
		"status":         strVal(so, "status"),
		"total":          so["amount"],
		"due_date":       dueDate,
		"url_jurnal":     fmt.Sprintf("https://jurnal.id/sales_orders/%d", int(id)),
	})
}

// ── helpers ──────────────────────────────────────────────────────────────────

func buildLines(items []LineItem) []map[string]any {
	lines := make([]map[string]any, len(items))
	for i, it := range items {
		unit := it.Unit
		if unit == "" {
			unit = "pcs"
		}
		lines[i] = map[string]any{
			"product_name":     it.ProductName,
			"quantity":         it.Quantity,
			"unit_price":       it.UnitPrice,
			"unit":             unit,
			"discount_percent": it.DiscountPercent,
		}
	}
	return lines
}

func strVal(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func nullIfZero(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
