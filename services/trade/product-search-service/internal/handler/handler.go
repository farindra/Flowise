package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

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
