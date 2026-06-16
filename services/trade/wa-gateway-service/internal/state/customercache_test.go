package state

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"wa-gateway-service/internal/client"
)

// mockPricingServer builds an httptest.Server backing
// client.NewCustomerPricingClient, with per-test handler overrides.
type mockPricingServer struct {
	customerByPhone   map[string]client.Customer
	customerByCompany map[string]client.Customer
	vipByPhone        map[string]client.VipCustomer
}

func newMockPricingServer(t *testing.T) (*httptest.Server, *mockPricingServer) {
	t.Helper()
	m := &mockPricingServer{
		customerByPhone:   map[string]client.Customer{},
		customerByCompany: map[string]client.Customer{},
		vipByPhone:        map[string]client.VipCustomer{},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/customer":
			phone := r.URL.Query().Get("phone")
			if c, ok := m.customerByPhone[phone]; ok {
				json.NewEncoder(w).Encode(c)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "customer not found"})
		case "/customer/by-company":
			name := r.URL.Query().Get("name")
			if c, ok := m.customerByCompany[name]; ok {
				json.NewEncoder(w).Encode(c)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "customer not found"})
		case "/customer/vip":
			phone := r.URL.Query().Get("phone")
			if v, ok := m.vipByPhone[phone]; ok {
				json.NewEncoder(w).Encode(v)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "vip customer not found"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, m
}

func TestCustomerCache_PhoneLookup(t *testing.T) {
	srv, mock := newMockPricingServer(t)
	mock.customerByPhone["6281234567890"] = client.Customer{Nomor: "6281234567890", Nama: "PT Test", Pulau: "Pulau Jawa"}

	store := newTestStore(t)
	cc := NewCustomerCache(store, client.NewCustomerPricingClient(srv.URL))

	customer, err := cc.GetCustomerInfoEnhanced(context.Background(), "6281234567890")
	if err != nil {
		t.Fatalf("GetCustomerInfoEnhanced() error = %v", err)
	}
	if customer == nil {
		t.Fatal("GetCustomerInfoEnhanced() = nil, want customer")
	}
	if customer.Nama != "PT Test" {
		t.Errorf("Nama = %q, want PT Test", customer.Nama)
	}
	if customer.Source != "phone_lookup" {
		t.Errorf("Source = %q, want phone_lookup", customer.Source)
	}
	if customer.IsVip {
		t.Error("IsVip = true, want false")
	}

	var isRegistered bool
	found, err := store.Get("6281234567890", "isRegistered", &isRegistered)
	if err != nil || !found || !isRegistered {
		t.Errorf("isRegistered = (%v, %v, %v), want (true, true, nil)", found, isRegistered, err)
	}
}

func TestCustomerCache_PhoneLookupWithVip(t *testing.T) {
	srv, mock := newMockPricingServer(t)
	mock.customerByPhone["6281234567890"] = client.Customer{Nomor: "6281234567890", Nama: "PT Test", Pulau: "Pulau Jawa"}
	mock.vipByPhone["6281234567890"] = client.VipCustomer{ID: 1, Nama: "PT Test", Grade: "gold", DiscountPercentage: 5}

	store := newTestStore(t)
	cc := NewCustomerCache(store, client.NewCustomerPricingClient(srv.URL))

	customer, err := cc.GetCustomerInfoEnhanced(context.Background(), "6281234567890")
	if err != nil {
		t.Fatalf("GetCustomerInfoEnhanced() error = %v", err)
	}
	if !customer.IsVip {
		t.Error("IsVip = false, want true")
	}
	if customer.VipInfo == nil || customer.VipInfo.Grade != "gold" {
		t.Errorf("VipInfo = %+v, want Grade=gold", customer.VipInfo)
	}
}

func TestCustomerCache_MemoryCacheHit(t *testing.T) {
	srv, mock := newMockPricingServer(t)
	mock.customerByPhone["6281234567890"] = client.Customer{Nomor: "6281234567890", Nama: "PT Test", Pulau: "Pulau Jawa"}

	store := newTestStore(t)
	cc := NewCustomerCache(store, client.NewCustomerPricingClient(srv.URL))

	first, err := cc.GetCustomerInfoEnhanced(context.Background(), "6281234567890")
	if err != nil {
		t.Fatalf("first call error = %v", err)
	}

	// Remove the customer from the mock backend; a cached result should still
	// be returned within the 30s TTL without hitting the backend again.
	delete(mock.customerByPhone, "6281234567890")

	second, err := cc.GetCustomerInfoEnhanced(context.Background(), "6281234567890")
	if err != nil {
		t.Fatalf("second call error = %v", err)
	}
	if second == nil {
		t.Fatal("second call = nil, want cached customer")
	}
	if second.Nama != first.Nama {
		t.Errorf("Nama = %q, want %q", second.Nama, first.Nama)
	}
	if second.CacheSource != "memory" {
		t.Errorf("CacheSource = %q, want memory", second.CacheSource)
	}
	if second.LastCached == 0 {
		t.Error("LastCached = 0, want nonzero")
	}
}

func TestCustomerCache_VipOnlyFallback(t *testing.T) {
	srv, mock := newMockPricingServer(t)
	// No phone match, but customer-pricing-service's /customer?phone= already
	// resolves the vip_only synthetic customer server-side.
	mock.customerByPhone["6281234567890"] = client.Customer{
		Nomor: "6281234567890", Nama: "VIP Orang", Wilayah: "Jawa Barat",
		Pulau: "Pulau Jawa", Source: "vip_only", IsVip: true,
		VipInfo: &client.VipCustomer{ID: 2, Nama: "VIP Orang", Grade: "platinum"},
	}

	store := newTestStore(t)
	cc := NewCustomerCache(store, client.NewCustomerPricingClient(srv.URL))

	customer, err := cc.GetCustomerInfoEnhanced(context.Background(), "6281234567890")
	if err != nil {
		t.Fatalf("GetCustomerInfoEnhanced() error = %v", err)
	}
	if customer == nil {
		t.Fatal("GetCustomerInfoEnhanced() = nil, want vip_only customer")
	}
	if customer.Source != "vip_only" {
		t.Errorf("Source = %q, want vip_only", customer.Source)
	}
	if customer.CacheSource != "vip_database" {
		t.Errorf("CacheSource = %q, want vip_database", customer.CacheSource)
	}
	if !customer.IsVip || customer.VipInfo == nil || customer.VipInfo.Grade != "platinum" {
		t.Errorf("VipInfo = %+v", customer.VipInfo)
	}
}

func TestCustomerCache_CompanyLookupPrecedesVipOnly(t *testing.T) {
	srv, mock := newMockPricingServer(t)
	// /customer?phone= for the user's own number resolves to a vip_only
	// fallback...
	mock.customerByPhone["6281234567890"] = client.Customer{
		Nomor: "6281234567890", Nama: "VIP Orang", Source: "vip_only", IsVip: true,
		VipInfo: &client.VipCustomer{ID: 2, Nama: "VIP Orang", Grade: "platinum"},
	}
	// ...but the user also has a company set, which should take precedence.
	mock.customerByCompany["PT Perusahaan"] = client.Customer{Nomor: "6289999999999", Nama: "PT Perusahaan", Company: "PT Perusahaan"}
	// Re-fetch by the matched customer's own phone for island defaulting.
	mock.customerByPhone["6289999999999"] = client.Customer{Nomor: "6289999999999", Nama: "PT Perusahaan", Company: "PT Perusahaan", Pulau: "Pulau Jawa"}

	store := newTestStore(t)
	if err := store.Set("6281234567890", "company", "PT Perusahaan"); err != nil {
		t.Fatalf("Set(company) error = %v", err)
	}

	cc := NewCustomerCache(store, client.NewCustomerPricingClient(srv.URL))
	customer, err := cc.GetCustomerInfoEnhanced(context.Background(), "6281234567890")
	if err != nil {
		t.Fatalf("GetCustomerInfoEnhanced() error = %v", err)
	}
	if customer == nil {
		t.Fatal("GetCustomerInfoEnhanced() = nil, want company-matched customer")
	}
	if customer.Source != "company_lookup" {
		t.Errorf("Source = %q, want company_lookup", customer.Source)
	}
	if customer.Nama != "PT Perusahaan" {
		t.Errorf("Nama = %q, want PT Perusahaan", customer.Nama)
	}
}

func TestCustomerCache_NotFound(t *testing.T) {
	srv, _ := newMockPricingServer(t)

	store := newTestStore(t)
	cc := NewCustomerCache(store, client.NewCustomerPricingClient(srv.URL))

	customer, err := cc.GetCustomerInfoEnhanced(context.Background(), "6280000000000")
	if err != nil {
		t.Fatalf("GetCustomerInfoEnhanced() error = %v", err)
	}
	if customer != nil {
		t.Errorf("GetCustomerInfoEnhanced() = %+v, want nil", customer)
	}

	var notFound bool
	found, err := store.Get("6280000000000", "customerNotFound", &notFound)
	if err != nil || !found || !notFound {
		t.Errorf("customerNotFound = (%v, %v, %v), want (true, true, nil)", found, notFound, err)
	}

	var notFoundTime int64
	found, err = store.Get("6280000000000", "customerNotFoundTime", &notFoundTime)
	if err != nil || !found || notFoundTime == 0 {
		t.Errorf("customerNotFoundTime = (%v, %v, %v), want (true, nonzero, nil)", found, notFoundTime, err)
	}
}

func TestCustomerCache_IsRegisteredCustomer(t *testing.T) {
	srv, mock := newMockPricingServer(t)
	id := int64(42)
	mock.customerByPhone["6281234567890"] = client.Customer{ID: &id, Nomor: "6281234567890", Nama: "PT Test"}

	store := newTestStore(t)
	cc := NewCustomerCache(store, client.NewCustomerPricingClient(srv.URL))

	registered, err := cc.IsRegisteredCustomer(context.Background(), "6281234567890")
	if err != nil {
		t.Fatalf("IsRegisteredCustomer() error = %v", err)
	}
	if !registered {
		t.Error("IsRegisteredCustomer() = false, want true")
	}

	registered, err = cc.IsRegisteredCustomer(context.Background(), "6280000000000")
	if err != nil {
		t.Fatalf("IsRegisteredCustomer() error = %v", err)
	}
	if registered {
		t.Error("IsRegisteredCustomer() = true, want false")
	}
}

func TestCustomerCache_GetCustomerPrice(t *testing.T) {
	pricingSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/price" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req client.PriceRequest
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(client.PriceResponse{Price: req.HargaNum.Customer * 1.05, IsRegistered: true})
	}))
	defer pricingSrv.Close()

	store := newTestStore(t)
	cc := NewCustomerCache(store, client.NewCustomerPricingClient(pricingSrv.URL))

	price, err := cc.GetCustomerPrice(context.Background(), client.Product{
		HargaNum: client.HargaNum{Customer: 100000, NonCustomer: 110000},
	}, "6281234567890")
	if err != nil {
		t.Fatalf("GetCustomerPrice() error = %v", err)
	}
	if price != 105000 {
		t.Errorf("GetCustomerPrice() = %v, want 105000", price)
	}
}
