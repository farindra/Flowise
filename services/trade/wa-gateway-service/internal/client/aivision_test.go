package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAIVisionClient_AnalyzeImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/analyze-image" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var req analyzeImageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Image != "base64data" {
			t.Errorf("Image = %q, want base64data", req.Image)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AnalyzeResult{
			Products:    []string{"6205"},
			Codes:       []string{"6205"},
			Confidence:  0.9,
			Description: "deteksi bearing 6205",
		})
	}))
	defer srv.Close()

	c := NewAIVisionClient(srv.URL)
	result, err := c.AnalyzeImage(context.Background(), "base64data", "image/jpeg", "6281234567890")
	if err != nil {
		t.Fatalf("AnalyzeImage() error = %v", err)
	}
	if len(result.Products) != 1 || result.Products[0] != "6205" {
		t.Errorf("Products = %v, want [6205]", result.Products)
	}
}

func TestAIVisionClient_ParseMultiProduct(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/parse-multi-product" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var req textPhoneRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Text != "6203, 6204 dan 6205" {
			t.Errorf("Text = %q", req.Text)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(MultiProductResult{
			IsMultiProduct: true,
			Products:       []string{"6203", "6204", "6205"},
			Confidence:     0.95,
			Method:         "ai_parsing",
		})
	}))
	defer srv.Close()

	c := NewAIVisionClient(srv.URL)
	result, err := c.ParseMultiProduct(context.Background(), "6203, 6204 dan 6205", "6281234567890")
	if err != nil {
		t.Fatalf("ParseMultiProduct() error = %v", err)
	}
	if !result.IsMultiProduct {
		t.Error("IsMultiProduct = false, want true")
	}
	if len(result.Products) != 3 {
		t.Errorf("len(Products) = %d, want 3", len(result.Products))
	}
}

func TestAIVisionClient_AnalyzeMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/analyze-message" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var req textPhoneRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Text != "harga 6205 berapa?" {
			t.Errorf("Text = %q", req.Text)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(MessageAnalysis{
			Keywords:        []string{"6205", "harga"},
			Intent:          "price_check",
			Products:        []string{"6205"},
			Quantity:        1,
			EnhancedQuery:   "6205",
			OriginalMessage: "harga 6205 berapa?",
		})
	}))
	defer srv.Close()

	c := NewAIVisionClient(srv.URL)
	result, err := c.AnalyzeMessage(context.Background(), "harga 6205 berapa?", "6281234567890")
	if err != nil {
		t.Fatalf("AnalyzeMessage() error = %v", err)
	}
	if result.Intent != "price_check" {
		t.Errorf("Intent = %q, want price_check", result.Intent)
	}
}

func TestAIVisionClient_PostJSONError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
	}))
	defer srv.Close()

	c := NewAIVisionClient(srv.URL)
	if _, err := c.AnalyzeMessage(context.Background(), "halo", "6281234567890"); err == nil {
		t.Error("AnalyzeMessage() error = nil, want error")
	}
}
