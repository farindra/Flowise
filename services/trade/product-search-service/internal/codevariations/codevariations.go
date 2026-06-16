// Package codevariations is a verbatim port of normalizeProductCode and
// createCodeVariations from local-data-manager.js. Duplicated from
// sync-indexer's internal/codevariations package - both are independent
// microservices and need the exact same variation logic (sync-indexer to
// index code_variations, product-search-service to generate query variations
// for exact-match lookups).
package codevariations

import (
	"regexp"
	"strings"
)

var (
	bearingPattern  = regexp.MustCompile(`^(\d{4,6})([a-z]{1,3})$`)
	digitLetterRe   = regexp.MustCompile(`(?i)(\d)([a-z])`)
	combinedNumRe   = regexp.MustCompile(`(?i)(\d+)\s+(\d+)([a-z]+)`)
	mergedSpecialRe = regexp.MustCompile(`[-./\\_\s]`)
	splitSpecialRe  = regexp.MustCompile(`[-./\\_]`)
	multiSpaceRe    = regexp.MustCompile(`\s+`)
)

// NormalizeProductCode - ported verbatim from normalizeProductCode:
// handles bearing code patterns like "63022rs" -> "6302 2rs".
func NormalizeProductCode(code string) string {
	if code == "" {
		return ""
	}

	normalized := strings.ToLower(strings.TrimSpace(code))

	if m := bearingPattern.FindStringSubmatch(normalized); m != nil {
		numbers, letters := m[1], m[2]
		if len(numbers) >= 5 {
			base := numbers[:len(numbers)-1]
			last := numbers[len(numbers)-1:]
			return base + " " + last + letters
		}
	}

	return normalized
}

// CreateCodeVariations - ported verbatim from createCodeVariations: 7
// variation strategies collected into a de-duplicated, order-preserving list.
func CreateCodeVariations(code string) []string {
	seen := make(map[string]bool)
	var variations []string
	add := func(v string) {
		if !seen[v] {
			seen[v] = true
			variations = append(variations, v)
		}
	}

	original := strings.ToLower(strings.TrimSpace(code))
	add(original)

	normalized := NormalizeProductCode(original)
	if normalized != original {
		add(normalized)
	}

	noSpaces := multiSpaceRe.ReplaceAllString(original, "")
	add(noSpaces)

	withSpaces := digitLetterRe.ReplaceAllString(original, "$1 $2")
	add(withSpaces)

	combinedNumbers := combinedNumRe.ReplaceAllString(original, "$1$2$3")
	add(combinedNumbers)

	mergedSpecial := mergedSpecialRe.ReplaceAllString(original, "")
	add(mergedSpecial)

	splitSpecial := splitSpecialRe.ReplaceAllString(original, " ")
	splitSpecial = multiSpaceRe.ReplaceAllString(splitSpecial, " ")
	splitSpecial = strings.TrimSpace(splitSpecial)
	add(splitSpecial)

	return variations
}
