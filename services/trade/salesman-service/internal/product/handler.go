package product

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Handler struct {
	searchURL string
	http      *http.Client
}

func NewHandler(searchURL string) *Handler {
	return &Handler{
		searchURL: strings.TrimSuffix(searchURL, "/"),
		http:      &http.Client{Timeout: 10 * time.Second},
	}
}

// HandleProducts — GET /products?q=...&limit=...
func (h *Handler) HandleProducts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	q := r.URL.Query().Get("q")
	if q == "" {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "parameter q wajib diisi"})
		return
	}
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "10"
	}

	url := fmt.Sprintf("%s/search?q=%s&limit=%s", h.searchURL, q, limit)
	resp, err := h.http.Get(url)
	if err != nil {
		w.WriteHeader(502)
		json.NewEncoder(w).Encode(map[string]string{"error": "product-search-service tidak tersedia"})
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		w.WriteHeader(502)
		json.NewEncoder(w).Encode(map[string]string{"error": "parse error"})
		return
	}

	products, _ := data["products"].([]any)
	results := make([]map[string]any, 0, len(products))
	for _, p := range products {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		hargaNum, _ := m["_hargaNum"].(map[string]any)
		harga, _ := hargaNum["normal"].(float64)
		results = append(results, map[string]any{
			"kode":        m["kode"],
			"nama":        m["nama"],
			"brand":       m["brand"],
			"stok":        m["stok"],
			"harga":       harga,
			"harga_fmt":   m["hargaNormal"],
			"keterangan":  m["keterangan"],
		})
	}

	json.NewEncoder(w).Encode(map[string]any{
		"query":   q,
		"total":   len(results),
		"results": results,
	})
}
