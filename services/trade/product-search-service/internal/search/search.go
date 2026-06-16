// Package search orchestrates the two-phase product lookup used by
// GET /search: an exact code_variations match (Phase A) followed by a
// Meilisearch full-text fallback (Phase B), merged and deduped by kode.
package search

import (
	"context"
	"strings"

	"product-search-service/internal/codevariations"
	"product-search-service/internal/meili"
	"product-search-service/internal/model"
)

const DefaultLimit = 10

type Service struct {
	meili *meili.Client
}

func New(m *meili.Client) *Service {
	return &Service{meili: m}
}

// Search returns up to limit products matching q. Exact code_variations
// matches are returned first, followed by fuzzy full-text matches to fill
// any remaining slots. Both phases are restricted to stok > 0.
func (s *Service) Search(ctx context.Context, q string, limit int) ([]model.Product, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}

	queryLower := strings.ToLower(strings.TrimSpace(q))
	if queryLower == "" {
		return []model.Product{}, nil
	}

	variations := codevariations.CreateCodeVariations(queryLower)

	exactDocs, err := s.meili.SearchExact(ctx, variations, int64(limit))
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, limit)
	results := make([]model.Product, 0, limit)
	for _, d := range exactDocs {
		if seen[d.Kode] {
			continue
		}
		seen[d.Kode] = true
		results = append(results, d.ToProduct())
	}

	if len(results) < limit {
		fuzzyDocs, err := s.meili.SearchFuzzy(ctx, queryLower, int64(limit*2))
		if err != nil {
			return nil, err
		}
		for _, d := range fuzzyDocs {
			if len(results) >= limit {
				break
			}
			if seen[d.Kode] {
				continue
			}
			seen[d.Kode] = true
			results = append(results, d.ToProduct())
		}
	}

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}
