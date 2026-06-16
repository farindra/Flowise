package shared

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// CartItem is the shape stored under the "cart" key in the state store,
// matching the object pushed by handleAddToCart (line 2625).
type CartItem struct {
	Kode     string   `json:"kode"`
	Nama     string   `json:"nama"`
	Harga    float64  `json:"harga"`
	Quantity int      `json:"quantity"`
	Stok     int64    `json:"stok"`
	Subtotal *float64 `json:"subtotal,omitempty"`
}

// FormatCurrency ports messageHandler.formatCurrency (line 2693):
// Intl.NumberFormat('id-ID', {style:'currency',currency:'IDR',...}).format(amount).
// Output: "Rp 1.234.567" for positive, "-Rp 500" for negative.
func FormatCurrency(amount float64) string {
	n := math.Round(amount)
	if n < 0 {
		return "-Rp " + formatIDR(int64(-n))
	}
	return "Rp " + formatIDR(int64(n))
}

// FormatPrice ports messageHandler.formatPrice (line 4992):
// `Rp ${price.toLocaleString('id-ID')}`.
// Output: "Rp 1.234.567" for positive, "Rp -500" for negative.
func FormatPrice(price float64) string {
	n := math.Round(price)
	if n < 0 {
		return "Rp -" + formatIDR(int64(-n))
	}
	return "Rp " + formatIDR(int64(n))
}

// formatIDR formats n with "." as thousands separator (id-ID locale).
func formatIDR(n int64) string {
	s := strconv.FormatInt(n, 10)
	mod := len(s) % 3
	var b strings.Builder
	b.WriteString(s[:mod])
	for i := mod; i < len(s); i += 3 {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

var numericPriceRe = regexp.MustCompile(`[\d,]+`)

// ExtractNumericPrice ports messageHandler.extractNumericPrice (line 4983):
// finds the first sequence of digits and commas in priceString, strips commas,
// and parses as an integer. Used for prices formatted with comma thousands
// separators (e.g. Jurnal's _hr format "1,234,567" → 1234567).
func ExtractNumericPrice(priceString string) int {
	m := numericPriceRe.FindString(priceString)
	if m == "" {
		return 0
	}
	n, _ := strconv.Atoi(strings.ReplaceAll(m, ",", ""))
	return n
}

// CalculateTotalPrice ports messageHandler.calculateTotalPrice (line 5000):
// sums cart items, using Subtotal when set, otherwise Harga * Quantity.
func CalculateTotalPrice(items []CartItem) float64 {
	total := 0.0
	for _, item := range items {
		if item.Subtotal != nil {
			total += *item.Subtotal
		} else {
			qty := item.Quantity
			if qty == 0 {
				qty = 1
			}
			total += item.Harga * float64(qty)
		}
	}
	return total
}

// CalculateGrandTotal ports messageHandler.calculateGrandTotal (line 5014):
// adds PPN 11% to totalPrice.
func CalculateGrandTotal(totalPrice float64) float64 {
	return totalPrice + totalPrice*0.11
}
