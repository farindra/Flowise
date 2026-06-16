package shared

import (
	"testing"
)

func TestFormatCurrency(t *testing.T) {
	tests := []struct {
		amount float64
		want   string
	}{
		{0, "Rp 0"},
		{500, "Rp 500"},
		{1234567, "Rp 1.234.567"},
		{1000000000, "Rp 1.000.000.000"},
		{-500, "-Rp 500"},
		{234567, "Rp 234.567"},
	}
	for _, tt := range tests {
		got := FormatCurrency(tt.amount)
		if got != tt.want {
			t.Errorf("FormatCurrency(%v) = %q, want %q", tt.amount, got, tt.want)
		}
	}
}

func TestFormatPrice(t *testing.T) {
	tests := []struct {
		price float64
		want  string
	}{
		{0, "Rp 0"},
		{1234567, "Rp 1.234.567"},
		{-500, "Rp -500"},
		{99000, "Rp 99.000"},
	}
	for _, tt := range tests {
		got := FormatPrice(tt.price)
		if got != tt.want {
			t.Errorf("FormatPrice(%v) = %q, want %q", tt.price, got, tt.want)
		}
	}
}

func TestExtractNumericPrice(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"1,234,567", 1234567},
		{"Rp 1.234.567", 1}, // period-format: only leading digit matched
		{"567", 567},
		{"", 0},
		{"abc", 0},
		{"1,000", 1000},
	}
	for _, tt := range tests {
		got := ExtractNumericPrice(tt.input)
		if got != tt.want {
			t.Errorf("ExtractNumericPrice(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestCalculateTotalPrice(t *testing.T) {
	sub := 50000.0
	items := []CartItem{
		{Kode: "A", Harga: 10000, Quantity: 2},    // 20000
		{Kode: "B", Harga: 15000, Quantity: 0},    // qty=0 → qty=1 → 15000
		{Kode: "C", Harga: 99999, Subtotal: &sub}, // uses subtotal 50000
	}
	got := CalculateTotalPrice(items)
	if got != 85000 {
		t.Errorf("CalculateTotalPrice() = %v, want 85000", got)
	}
}

func TestCalculateGrandTotal(t *testing.T) {
	got := CalculateGrandTotal(100000)
	if got != 111000 {
		t.Errorf("CalculateGrandTotal(100000) = %v, want 111000", got)
	}
}
