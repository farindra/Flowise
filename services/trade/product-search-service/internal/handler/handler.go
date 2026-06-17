package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"product-search-service/internal/search"
)

type Handler struct {
	search *search.Service
	name   string
}

func New(s *search.Service, serviceName string) *Handler {
	return &Handler{search: s, name: serviceName}
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": h.name,
		"status":  "ok",
	})
}

// Search - GET /search?q=...&limit=10 (also accepts a JSON POST body
// {"q": "...", "limit": 10} for consistency with the other services).
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := search.DefaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	if r.Method == http.MethodPost {
		var body struct {
			Q     string `json:"q"`
			Limit int    `json:"limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			if body.Q != "" {
				q = body.Q
			}
			if body.Limit > 0 {
				limit = body.Limit
			}
		}
	}

	if q == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing q parameter"})
		return
	}

	results, err := h.search.Search(r.Context(), q, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"products": results,
	})
}

// ExportCSV - GET /export/csv
// Streams the full product catalog as a UTF-8 CSV file (BOM-prefixed for Excel).
func (h *Handler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	docs, err := h.search.AllProducts(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("katalog-ob-trade-%s.csv", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	// UTF-8 BOM so Excel opens it correctly.
	w.Write([]byte("\xEF\xBB\xBF")) //nolint:errcheck

	cw := csv.NewWriter(w)
	cw.Write([]string{"Kode", "Nama", "Brand", "Stok", "Harga Normal", "Harga Customer", "Harga Non-Customer", "Harga Cash", "Tersedia", "Last Updated"}) //nolint:errcheck

	for _, d := range docs {
		tersedia := "Ya"
		if !d.Available {
			tersedia = "Tidak"
		}
		cw.Write([]string{ //nolint:errcheck
			d.Kode,
			d.Nama,
			d.Brand,
			strconv.FormatInt(d.Stok, 10),
			d.HargaNormal,
			d.HargaCustomer,
			d.HargaNonCustomer,
			d.HargaCash,
			tersedia,
			d.LastUpdated,
		})
	}
	cw.Flush()
}
