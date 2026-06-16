package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCustomerPricingClient_GetCustomer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("phone") {
		case "6281234567890":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Customer{Nomor: "6281234567890", Nama: "PT Test", Pulau: "Pulau Jawa"})
		default:
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "customer not found"})
		}
	}))
	defer srv.Close()

	c := NewCustomerPricingClient(srv.URL)

	customer, err := c.GetCustomer(context.Background(), "6281234567890")
	if err != nil {
		t.Fatalf("GetCustomer() error = %v", err)
	}
	if customer == nil || customer.Nama != "PT Test" {
		t.Errorf("GetCustomer() = %+v, want Nama=PT Test", customer)
	}

	notFound, err := c.GetCustomer(context.Background(), "60000000000")
	if err != nil {
		t.Fatalf("GetCustomer() error = %v", err)
	}
	if notFound != nil {
		t.Errorf("GetCustomer() = %+v, want nil", notFound)
	}
}

func TestCustomerPricingClient_GetCustomerByCompany(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/customer/by-company" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("name"); got != "PT Test" {
			t.Errorf("name = %q, want PT Test", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Customer{Nomor: "6281234567890", Nama: "PT Test", Company: "PT Test"})
	}))
	defer srv.Close()

	c := NewCustomerPricingClient(srv.URL)
	customer, err := c.GetCustomerByCompany(context.Background(), "PT Test")
	if err != nil {
		t.Fatalf("GetCustomerByCompany() error = %v", err)
	}
	if customer == nil || customer.Company != "PT Test" {
		t.Errorf("GetCustomerByCompany() = %+v", customer)
	}
}

func TestCustomerPricingClient_GetCustomerVip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/customer/vip" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "vip customer not found"})
	}))
	defer srv.Close()

	c := NewCustomerPricingClient(srv.URL)
	vip, err := c.GetCustomerVip(context.Background(), "6281234567890")
	if err != nil {
		t.Fatalf("GetCustomerVip() error = %v", err)
	}
	if vip != nil {
		t.Errorf("GetCustomerVip() = %+v, want nil", vip)
	}
}

func TestCustomerPricingClient_GetPrice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/price" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}

		var req PriceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.PhoneNumber != "6281234567890" {
			t.Errorf("PhoneNumber = %q, want 6281234567890", req.PhoneNumber)
		}
		if req.HargaNum.Customer != 90000 {
			t.Errorf("HargaNum.Customer = %v, want 90000", req.HargaNum.Customer)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PriceResponse{Price: 99000, IsRegistered: true, Pulau: "Pulau Jawa"})
	}))
	defer srv.Close()

	c := NewCustomerPricingClient(srv.URL)
	resp, err := c.GetPrice(context.Background(), PriceRequest{
		PhoneNumber: "6281234567890",
		HargaNum:    PriceHargaNum{Customer: 90000, NonCustomer: 95000},
	})
	if err != nil {
		t.Fatalf("GetPrice() error = %v", err)
	}
	if resp.Price != 99000 {
		t.Errorf("Price = %v, want 99000", resp.Price)
	}
	if !resp.IsRegistered {
		t.Error("IsRegistered = false, want true")
	}
}
