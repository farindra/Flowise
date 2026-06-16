package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProductSearchClient_Search(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "6205" {
			t.Errorf("q = %q, want 6205", got)
		}
		if got := r.URL.Query().Get("limit"); got != "10" {
			t.Errorf("limit = %q, want 10", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"products":[{"id":1,"kode":"6205","nama":"Bearing 6205","stok":5,"brand":"SKF","hargaNormal":"100.000","hargaCustomer":"90.000","hargaNonCustomer":"95.000","hargaCash":"85.000","_hargaNum":{"normal":100000,"customer":90000,"nonCustomer":95000,"cash":85000},"active":true,"available":true,"lastUpdated":"2026-01-01"}]}`))
	}))
	defer srv.Close()

	c := NewProductSearchClient(srv.URL)
	products, err := c.Search(context.Background(), "6205", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(products) != 1 {
		t.Fatalf("len(products) = %d, want 1", len(products))
	}
	if products[0].Kode != "6205" {
		t.Errorf("Kode = %q, want 6205", products[0].Kode)
	}
	if products[0].HargaNum.Customer != 90000 {
		t.Errorf("HargaNum.Customer = %v, want 90000", products[0].HargaNum.Customer)
	}
}

func TestProductSearchClient_SearchEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"products":[]}`))
	}))
	defer srv.Close()

	c := NewProductSearchClient(srv.URL)
	products, err := c.Search(context.Background(), "nonexistent", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(products) != 0 {
		t.Errorf("len(products) = %d, want 0", len(products))
	}
}

func TestProductSearchClient_SearchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewProductSearchClient(srv.URL)
	if _, err := c.Search(context.Background(), "6205", 10); err == nil {
		t.Error("Search() error = nil, want error")
	}
}
