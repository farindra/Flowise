package shared

import "strings"

// IsGreeting ports messageHandler.isGreeting (line 2017).
// Returns false immediately if message starts with '^' (product-search prefix).
func IsGreeting(message string) bool {
	if strings.HasPrefix(message, "^") {
		return false
	}
	lower := strings.ToLower(message)
	for _, g := range []string{"hai", "halo", "hello", "hi", "selamat", "pagi", "siang", "sore", "malam"} {
		if strings.Contains(lower, g) {
			return true
		}
	}
	return false
}

// IsLocationQuestion ports messageHandler.isLocationQuestion (line 2028).
func IsLocationQuestion(message string) bool {
	lower := strings.ToLower(message)
	for _, kw := range []string{"alamat", "dimana", "lokasi", "toko", "kantor", "tempat"} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// IsMarketingQuestion ports messageHandler.isMarketingQuestion (line 2034).
func IsMarketingQuestion(message string) bool {
	lower := strings.ToLower(message)
	for _, kw := range []string{
		"marketing", "sales", "penjualan", "hubungkan", "hubungi", "kontak",
		"nomor marketing", "telepon marketing", "wa marketing", "whatsapp marketing",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// IsBotIdentityQuestion ports messageHandler.isBotIdentityQuestion (line 2040).
func IsBotIdentityQuestion(message string) bool {
	lower := strings.ToLower(message)
	for _, kw := range []string{
		"siapa", "kamu siapa", "bot", "asisten", "namamu", "nama kamu",
		"dengan siapa", "bicara dengan siapa", "ini siapa", "berbicara dengan siapa",
		"saya bicara dengan siapa", "kamu ini siapa", "kamu ini apa",
		"apakah kamu bot", "apakah kamu manusia", "apakah saya bicara dengan bot",
		"apakah saya bicara dengan manusia", "kamu robot", "kamu ai",
		"kamu artificial intelligence", "kamu program", "kamu aplikasi",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// IsPriceQuestion ports messageHandler.isPriceQuestion (line 2072).
func IsPriceQuestion(message string) bool {
	lower := strings.ToLower(message)
	for _, kw := range []string{"harga", "berapa", "diskon", "promo"} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// IsStockQuestion ports messageHandler.isStockQuestion (line 2078).
func IsStockQuestion(message string) bool {
	lower := strings.ToLower(message)
	for _, kw := range []string{"stok", "stock", "ready", "tersedia", "ada barang"} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// IsPPNQuestion ports messageHandler.isPPNQuestion (line 2084).
func IsPPNQuestion(message string) bool {
	lower := strings.ToLower(message)
	for _, kw := range []string{
		"ppn", "pajak", "tax", "sudah ppn", "belum ppn", "termasuk ppn", "plus ppn",
		"dengan ppn", "tanpa ppn", "harga ppn", "ppn berapa", "kena ppn", "ada ppn",
		"include ppn", "exclude ppn", "before tax", "after tax", "pre tax", "post tax",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
