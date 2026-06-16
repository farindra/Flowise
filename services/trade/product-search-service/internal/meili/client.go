// Package meili wraps the Meilisearch SDK for read-only search against the
// "products" index populated by sync-indexer.
package meili

import (
	"context"
	"fmt"
	"strings"

	meilisearch "github.com/meilisearch/meilisearch-go"

	"product-search-service/internal/model"
)

const IndexUID = "products"

type Client struct {
	sm meilisearch.ServiceManager
}

func New(url, apiKey string) *Client {
	return &Client{sm: meilisearch.New(url, meilisearch.WithAPIKey(apiKey))}
}

// SearchExact looks up documents whose code_variations field matches any of
// the given variations (Phase A: exact code match), restricted to stok > 0.
// All variations are checked in a single request via an OR'd filter.
func (c *Client) SearchExact(ctx context.Context, variations []string, limit int64) ([]model.Doc, error) {
	if len(variations) == 0 {
		return nil, nil
	}

	clauses := make([]string, 0, len(variations))
	for _, v := range variations {
		clauses = append(clauses, fmt.Sprintf("code_variations = %s", quoteFilterValue(v)))
	}
	filter := fmt.Sprintf("(%s) AND stok > 0", strings.Join(clauses, " OR "))

	return c.search(ctx, "", filter, limit)
}

// SearchFuzzy runs a normal full-text search restricted to stok > 0 (Phase B
// fallback for partial/typo queries).
func (c *Client) SearchFuzzy(ctx context.Context, query string, limit int64) ([]model.Doc, error) {
	return c.search(ctx, query, "stok > 0", limit)
}

func (c *Client) search(ctx context.Context, query, filter string, limit int64) ([]model.Doc, error) {
	resp, err := c.sm.Index(IndexUID).SearchWithContext(ctx, query, &meilisearch.SearchRequest{
		Filter: filter,
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}

	var docs []model.Doc
	if err := resp.Hits.DecodeInto(&docs); err != nil {
		return nil, fmt.Errorf("decode search hits: %w", err)
	}
	return docs, nil
}

// quoteFilterValue quotes a string for use in a Meilisearch filter
// expression, escaping backslashes and double quotes.
func quoteFilterValue(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
