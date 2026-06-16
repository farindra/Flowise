package shared

import (
	"slices"
	"testing"
)

func TestKnownBrands_Loaded(t *testing.T) {
	if len(KnownBrands) == 0 {
		t.Fatal("KnownBrands is empty")
	}
	// File has 299 lines, all non-empty.
	if len(KnownBrands) < 200 {
		t.Errorf("KnownBrands len = %d, want >= 200", len(KnownBrands))
	}
}

func TestKnownBrands_Lowercase(t *testing.T) {
	for _, b := range KnownBrands {
		if b == "" {
			t.Error("found empty brand in KnownBrands")
		}
	}
	// Spot-check known brands are lowercase.
	for _, want := range []string{"skf", "fag", "nsk", "ntn", "timken", "koyo"} {
		if !slices.Contains(KnownBrands, want) {
			t.Errorf("KnownBrands missing %q", want)
		}
	}
}
