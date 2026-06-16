// Package shared provides pure helper functions used across multiple router
// handlers, ported from messageHandler.js utility methods.
package shared

import (
	_ "embed"
	"strings"
)

//go:embed all_brands.txt
var allBrandsTxt string

// KnownBrands ports messageHandler.loadKnownBrands (line 61-76): the list of
// known bearing brand names loaded from all_brands.txt, lowercased and trimmed.
// Bundled into the binary via go:embed so no runtime file access is needed.
var KnownBrands = loadKnownBrands()

func loadKnownBrands() []string {
	lines := strings.Split(allBrandsTxt, "\n")
	brands := make([]string, 0, len(lines))
	for _, line := range lines {
		if b := strings.ToLower(strings.TrimSpace(line)); b != "" {
			brands = append(brands, b)
		}
	}
	return brands
}
