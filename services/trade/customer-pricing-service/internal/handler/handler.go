package handler

import (
	"encoding/json"
	"net/http"

	"customer-pricing-service/internal/pricing"
)

type Handler struct {
	store    *pricing.Store
	vipStore *pricing.VipStore
	name     string
}

func New(store *pricing.Store, vipStore *pricing.VipStore, serviceName string) *Handler {
	return &Handler{store: store, vipStore: vipStore, name: serviceName}
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"service":       h.name,
		"status":        "ok",
		"customerCount": h.store.Count(),
	})
}

// Customer - GET /customer?phone=...
func (h *Handler) Customer(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing phone query parameter"})
		return
	}

	customer := h.store.GetCustomerDetailWithIsland(phone, h.vipStore)
	if customer == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "customer not found"})
		return
	}

	writeJSON(w, http.StatusOK, customer)
}

// CustomerByCompany - GET /customer/by-company?name=...
func (h *Handler) CustomerByCompany(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing name query parameter"})
		return
	}

	customer := h.store.FindCustomerByCompanyName(name)
	if customer == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "customer not found"})
		return
	}

	writeJSON(w, http.StatusOK, customer)
}

// CustomerVip - GET /customer/vip?phone=...
func (h *Handler) CustomerVip(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing phone query parameter"})
		return
	}

	vip := h.vipStore.FindVipCustomer(phone)
	if vip == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "vip customer not found"})
		return
	}

	writeJSON(w, http.StatusOK, vip)
}

// Price - POST /price
func (h *Handler) Price(w http.ResponseWriter, r *http.Request) {
	var req pricing.PriceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.PhoneNumber == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing phoneNumber"})
		return
	}

	resp := pricing.GetCustomerPrice(h.store, h.vipStore, req)
	writeJSON(w, http.StatusOK, resp)
}
