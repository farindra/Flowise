package shared

import "testing"

func TestIsGreeting(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"halo", true},
		{"Halo bos", true},
		{"selamat pagi", true},
		{"^6205", false}, // starts with ^ → product search, not greeting
		{"mau beli bearing", false},
		{"hi ada stok?", true},
	}
	for _, tt := range tests {
		got := IsGreeting(tt.msg)
		if got != tt.want {
			t.Errorf("IsGreeting(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func TestIsLocationQuestion(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"dimana kantornya?", true},
		{"alamat toko", true},
		{"berapa harga 6205?", false},
		{"lokasi gudang mana", true},
	}
	for _, tt := range tests {
		got := IsLocationQuestion(tt.msg)
		if got != tt.want {
			t.Errorf("IsLocationQuestion(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func TestIsMarketingQuestion(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"hubungi marketing", true},
		{"nomor marketing berapa?", true},
		{"berapa harga bearing?", false},
		{"kontak sales", true},
	}
	for _, tt := range tests {
		got := IsMarketingQuestion(tt.msg)
		if got != tt.want {
			t.Errorf("IsMarketingQuestion(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func TestIsBotIdentityQuestion(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"kamu siapa?", true},
		{"apakah kamu bot?", true},
		{"kamu robot bukan?", true},
		{"berapa harga 6205?", false},
		{"siapa yang jual bearing?", true},
	}
	for _, tt := range tests {
		got := IsBotIdentityQuestion(tt.msg)
		if got != tt.want {
			t.Errorf("IsBotIdentityQuestion(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func TestIsPriceQuestion(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"harga 6205 berapa?", true},
		{"berapa harga bearing ini?", true},
		{"ada diskon?", true},
		{"stok masih ada?", false},
	}
	for _, tt := range tests {
		got := IsPriceQuestion(tt.msg)
		if got != tt.want {
			t.Errorf("IsPriceQuestion(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func TestIsStockQuestion(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"stok 6205 masih ada?", true},
		{"ready stock?", true},
		{"ada barang gak?", true},
		{"berapa harga?", false},
	}
	for _, tt := range tests {
		got := IsStockQuestion(tt.msg)
		if got != tt.want {
			t.Errorf("IsStockQuestion(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func TestIsPPNQuestion(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"harganya sudah termasuk ppn?", true},
		{"apakah ada pajak?", true},
		{"before tax atau after tax?", true},
		{"harga bearing berapa?", false},
	}
	for _, tt := range tests {
		got := IsPPNQuestion(tt.msg)
		if got != tt.want {
			t.Errorf("IsPPNQuestion(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}
